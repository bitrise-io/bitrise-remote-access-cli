package ssh

import (
	"bytes"
	"fmt"
	"strings"

	cryptoSSH "golang.org/x/crypto/ssh"
)

// runWithPty runs the given commands on the remote server using a pseudo terminal.
// It takes an SSH client, a slice of commands, a command prefix, and a result map to store the output.
// The function returns an error if any step fails.
func runWithPty(client *cryptoSSH.Client, commands *[]string, commandPrefix string, resultMap *map[string]string) error {
	session, err := createSSHSession(client)
	if err != nil {
		return err
	}
	defer session.Close()

	// Request a pseudo terminal
	if err := session.RequestPty("xterm", 80, 40, cryptoSSH.TerminalModes{}); err != nil {
		return fmt.Errorf("request pty: %w", err)
	}

	// Save pipe for commands later
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("get stdin pipe: %w", err)
	}

	// Start remote shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	// Commands will be given in a single string, separated by carriage return
	var jointCommands string
	for i, command := range *commands {
		// Format the command to be able to extract the output later
		// Output will be in the format (prefix not included): [command=output]
		var formattedCommand string
		if resultMap != nil {
			formattedCommand = fmt.Sprintf("%s | awk '{print \"[result%d=\"$0\"]\"}'\r", command, i)
		} else {
			formattedCommand = fmt.Sprintf("%s\r", command)
		}
		jointCommands = fmt.Sprintf("%s%s%s", jointCommands, commandPrefix, formattedCommand)
	}

	// Session woould wait for the last command to finish, so we need to exit the shell
	if _, err := fmt.Fprintf(stdin, "%sexit\r", jointCommands); err != nil {
		return fmt.Errorf("send command: %w", err)
	}

	// Wait till exit
	if err := session.Wait(); err != nil {
		return fmt.Errorf("wait for session: %w", err)
	}

	// Check for errors
	if stderrBuf.Len() > 0 {
		return fmt.Errorf("stderr: %s", stderrBuf.String())
	}

	if resultMap == nil {
		return nil
	}

	if *resultMap == nil {
		*resultMap = make(map[string]string)
	}

	// Extract the output
	output := stdoutBuf.String()
	for i, command := range *commands {
		prefix := fmt.Sprintf("[result%d=", i)
		startIndex := strings.LastIndex(output, prefix)
		if startIndex != -1 {
			startIndex += len(prefix)
			endIndex := strings.Index(output[startIndex:], "]")
			if endIndex != -1 {
				result := output[startIndex : startIndex+endIndex]
				(*resultMap)[command] = result
			}
		}
	}

	return nil
}
