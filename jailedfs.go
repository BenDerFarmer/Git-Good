package main

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

type jailedFS struct {
	root string
}

func (j *jailedFS) safePath(p string) (string, error) {
	// SFTP paths are posix. Clean with path (not filepath), then map to host.
	clean := path.Clean("/" + p) // ensure absolute for cleaning
	clean = strings.TrimPrefix(clean, "/")
	full := filepath.Join(j.root, filepath.FromSlash(clean))

	// Ensure the result is still within the jail.
	jroot, _ := filepath.Abs(j.root)
	jfull, _ := filepath.Abs(full)
	if !strings.HasPrefix(jfull+string(os.PathSeparator), jroot+string(os.PathSeparator)) &&
		jfull != jroot {
		return "", errors.New("path escapes jail")
	}
	return full, nil
}

func (j *jailedFS) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	p, err := j.safePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.Open(p) // *os.File implements io.ReaderAt (and will be closed by server)
}

func (j *jailedFS) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	// Handles Put and (fallback) Open. We map SFTP flags to os flags.
	p, err := j.safePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	flags := j.toOSFlags(r.Pflags())
	// Avoid O_APPEND because WriteAt and O_APPEND conflict over offsets.
	flags &^= os.O_APPEND

	mode := os.FileMode(0o644)
	if r.AttrFlags().Permissions {
		mode = r.Attributes().FileMode()
	}
	f, err := os.OpenFile(p, flags, mode)
	if err != nil {
		return nil, err
	}
	return f, nil // *os.File implements io.WriterAt
}

// OpenFile allows read/write on the same handle (used by some clients).
func (j *jailedFS) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	p, err := j.safePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	flags := j.toOSFlags(r.Pflags())
	flags &^= os.O_APPEND
	mode := os.FileMode(0o644)
	if r.AttrFlags().Permissions {
		mode = r.Attributes().FileMode()
	}
	return os.OpenFile(p, flags, mode)
}

func (j *jailedFS) toOSFlags(f sftp.FileOpenFlags) int {
	// Map SFTP flags to os flags.
	var flags int
	switch {
	case f.Read && f.Write:
		flags |= os.O_RDWR
	case f.Read:
		flags |= os.O_RDONLY
	case f.Write:
		flags |= os.O_WRONLY
	}
	if f.Creat {
		flags |= os.O_CREATE
	}
	if f.Trunc {
		flags |= os.O_TRUNC
	}
	if f.Excl {
		flags |= os.O_EXCL
	}
	return flags
}

func (j *jailedFS) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	p, err := j.safePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case "List":
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(entries))
		for _, de := range entries {
			fi, err := de.Info()
			if err != nil {
				return nil, err
			}
			infos = append(infos, fi)
		}
		return &listAt{infos: infos}, nil
	case "Stat", "Lstat":
		fi, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		return &listAt{infos: []os.FileInfo{fi}}, nil
	case "Readlink":
		// Fallback behavior: return the target’s FileInfo (best-effort)
		target, err := os.Readlink(p)
		if err != nil {
			return nil, err
		}
		tp, err := j.safePath(path.Join(path.Dir(r.Filepath), target))
		if err != nil {
			return nil, err
		}
		fi, err := os.Lstat(tp)
		if err != nil {
			return nil, err
		}
		return &listAt{infos: []os.FileInfo{fi}}, nil
	default:
		return nil, errors.New("unsupported list method")
	}
}

type listAt struct {
	infos []os.FileInfo
	pos   int64
}

func (l *listAt) ListAt(dst []os.FileInfo, off int64) (int, error) {
	if off != l.pos {
		l.pos = off
	}
	if off >= int64(len(l.infos)) {
		return 0, io.EOF
	}
	n := copy(dst, l.infos[off:])
	l.pos += int64(n)
	if int(l.pos) >= len(l.infos) {
		return n, io.EOF
	}
	return n, nil
}

func (j *jailedFS) Filecmd(r *sftp.Request) error {
	p, err := j.safePath(r.Filepath)
	if err != nil {
		return err
	}
	switch r.Method {
	case "Mkdir":
		return os.Mkdir(p, 0o755)
	case "Rmdir":
		return os.Remove(p)
	case "Remove":
		return os.Remove(p)
	case "Rename", "PosixRename":
		if r.Target == "" {
			return errors.New("missing target")
		}
		tp, err := j.safePath(r.Target)
		if err != nil {
			return err
		}
		return os.Rename(p, tp)
	case "Symlink":
		if r.Target == "" {
			return errors.New("missing target")
		}
		tp, err := j.safePath(r.Target)
		if err != nil {
			return err
		}
		return os.Symlink(tp, p)
	case "Link":
		if r.Target == "" {
			return errors.New("missing target")
		}
		tp, err := j.safePath(r.Target)
		if err != nil {
			return err
		}
		return os.Link(p, tp)
	case "Setstat":
		// Best-effort: apply chmod/chown/truncate/times when provided.
		attrs := r.Attributes()
		flags := r.AttrFlags()
		if flags.Permissions {
			if err := os.Chmod(p, attrs.FileMode()); err != nil {
				return err
			}
		}
		if flags.Size {
			if err := os.Truncate(p, int64(attrs.Size)); err != nil {
				return err
			}
		}
		if flags.Acmodtime {
			at := attrs.AccessTime()
			mt := attrs.ModTime()
			if !at.IsZero() || !mt.IsZero() {
				if err := os.Chtimes(p, at, mt); err != nil {
					return err
				}
			}
		}
		// uid/gid intentionally ignored unless you plumb platform-specific logic.
		return nil
	default:
		return errors.New("unsupported cmd")
	}
}
