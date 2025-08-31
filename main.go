package main

import (
	_ "embed"
	"io"
	"log"
	"strings"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"

	gossh "golang.org/x/crypto/ssh"
)

//go:embed keys/hostkey
var hostkey []byte

func main() {

	publicKeyOption := ssh.PublicKeyHandler(func(ctx ssh.Context, key ssh.PublicKey) bool {
		return true
	})

	s := &ssh.Server{
		Addr:             ":2222",
		PublicKeyHandler: publicKeyOption,
		Handler:          sessionHandler,
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": SftpHandler,
		},
	}

	signer, err := gossh.ParsePrivateKey(hostkey)
	if err != nil {
		log.Fatalf("Failed to parse host key: %v", err)
	}

	s.AddHostKey(signer)

	log.Fatal(s.ListenAndServe())
}

func sessionHandler(s ssh.Session) {
	master, slave, err := pty.Open()
	if err != nil {
		io.WriteString(s, "failed to open pty: "+err.Error()+"\n")
		return
	}
	defer master.Close()
	defer slave.Close()
	io.WriteString(s, "Git Good - v.0.0.1\nEnter command (or 'exit' to quit)\n> ")

	go func() {
		buf := make([]byte, 8192)
		for {

			n, err := slave.Read(buf)
			if n > 0 {
				command := string(buf[:n-1])
				_, _ = slave.Write([]byte(executeCommand(command, s)))
				buf = make([]byte, 8192)
				io.WriteString(slave, "\n> ")
			}
			if err != nil {
				return
			}
		}
	}()

	// Pipe between SSH session and PTY master
	// session -> master (what user types goes to master (and then to slave))
	go func() { _, _ = io.Copy(master, s) }()
	// master -> session (what was written to slave and echoed appears here)
	_, _ = io.Copy(s, master)

}

func SftpHandler(sess ssh.Session) {
	serverOptions := []sftp.ServerOption{
		sftp.WithServerWorkingDirectory("repos"),
	}
	server, err := sftp.NewServer(
		sess,
		serverOptions...,
	)
	if err != nil {
		log.Printf("sftp server init error: %s\n", err)
		return
	}
	if err := server.Serve(); err == io.EOF {
		server.Close()
	}
}

func executeCommand(rawcommand string, s ssh.Session) string {

	command := strings.Split(rawcommand, " ")

	switch command[0] {

	case "create":

		if len(command) != 2 {
			return "Usage:\ncreate (USERNAME/)REPONAME"
		}

		// check if it contains invaild chars
		var repoUser string
		var repoName string

		split := strings.Split(command[1], "/")
		if len(split) == 2 {
			repoUser = split[0]
			repoName = split[1]
		} else {
			repoUser = s.User()
			repoName = command[1]
		}

		if err := createRepo(repoUser, repoName); err != nil {
			return "error while creating repo"
		}

		return "Created repo " + repoUser + "/" + repoName + "."
	case "exit":
		s.Close()
	}
	return "Unkown command use 'help'"

}
