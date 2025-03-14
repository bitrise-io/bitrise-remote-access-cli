package ssh

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"time"

	cryptoSSH "golang.org/x/crypto/ssh"
)

type ReusableSession struct {
	session *cryptoSSH.Session
	stdin   io.WriteCloser
	// TODO: WaitGroup
}

func StartNewReusableSession(client *cryptoSSH.Client) (ReusableSession, error) {
	session, err := client.NewSession()
	if err != nil {
		return ReusableSession{}, fmt.Errorf("start new session: %w", err)
	}

	// Get a pipe that will be used to write commands to the remote shell
	stdin, err := session.StdinPipe()
	if err != nil {
		return ReusableSession{}, fmt.Errorf("connect stdin: %w", err)
	}

	// Start a login shell on the remote for processing each command interactively
	session.Shell()

	return ReusableSession{session, stdin}, nil
}

func (s *ReusableSession) Run(command string) (stdout bytes.Buffer, stderr bytes.Buffer, err error) {
	s.session.Stdout = &stdout
	s.session.Stderr = &stderr

	rawInput := []byte(fmt.Sprintf("%s\n", command))
	_, err = s.stdin.Write(rawInput)
	if err != nil {
		return stdout, stderr, fmt.Errorf("run command: %w", err)
	}

	time.Sleep(2 * time.Second)
	log.Printf(stdout.String())
	log.Printf(stderr.String())

	// TODO: this hangs and probably incorrect, it waits for the shell to exit
	err = s.session.Wait()
	if err != nil {
		return stdout, stderr, err
	}

	return stdout, stderr, nil
}

func (s *ReusableSession) Close() {
	s.stdin.Close()
	s.session.Close()
}
