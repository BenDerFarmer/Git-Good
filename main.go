package main

import (
	_ "embed"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"

	gossh "golang.org/x/crypto/ssh"
)

//go:embed keys/hostkey
var hostKey []byte

const (
	serverName    = "Git Good"
	serverVersion = "v.0.0.1"
	reposDir      = "./repos/"
)

func main() {

	publicKeyOption := ssh.PublicKeyHandler(func(ctx ssh.Context, key ssh.PublicKey) bool {
		return true
	})

	s := &ssh.Server{
		Addr:             ":22",
		Version:          serverName + " " + serverVersion,
		PublicKeyHandler: publicKeyOption,
		Handler:          sessionHandler,
	}

	signer, err := gossh.ParsePrivateKey(hostKey)
	if err != nil {
		log.Fatalf("Failed to parse host key: %v", err)
	}

	s.AddHostKey(signer)

	log.Println(serverName + " - " + serverVersion)
	log.Println("listen on " + s.Addr)

	log.Fatal(s.ListenAndServe())

}

func sessionHandler(s ssh.Session) {
	if len(s.Command()) == 2 {
		err := gitHandler(s)
		if err != nil {
			log.Println(err)
		}
	} else {
		cliHandler(s)
	}
}

func cliHandler(s ssh.Session) {
	master, slave, err := pty.Open()
	if err != nil {
		io.WriteString(s, "failed to open pty: "+err.Error()+"\n")
		return
	}
	defer master.Close()
	defer slave.Close()
	io.WriteString(s, serverName+" - "+serverVersion+"\nEnter command (or 'exit' to quit)\n> ")

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

func gitHandler(s ssh.Session) error {
	command := s.Command()[0]
	repoPath := s.Command()[1]

	if !isVaildCommand(command) {
		return fmt.Errorf("Invaild command")
	}

	var err error
	repoPath, err = sanitizeRepoArg(repoPath)
	if err != nil {
		return err
	}

	if !isVaildRepoPath(repoPath) {
		return fmt.Errorf("Invaild Repo User or Name")
	}

	args := []string{repoPath}

	c := exec.Command(command, args...)
	c.Dir = reposDir
	c.Stdin = s
	c.Stdout = s
	c.Stderr = s
	c.Env = []string{}

	fmt.Printf("command: %v\n", command)
	fmt.Printf("path: %v\n", repoPath)

	if err := c.Run(); err != nil {
		return fmt.Errorf("git command failed: %w", err)
	}

	return nil
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
