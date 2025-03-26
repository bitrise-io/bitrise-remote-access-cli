package ssh

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	cryptoSSH "golang.org/x/crypto/ssh"
)

type copyItem struct {
	Content     string
	RemotePath  string
	Replace     *map[string]string
	Append      bool
	NoDuplicate bool
}

var ErrRemoteFileExists = errors.New("remote file already exists")

func copyItemSFTP(client *cryptoSSH.Client, item *copyItem) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	if err := sftpClient.MkdirAll(filepath.Dir(item.RemotePath)); err != nil {
		return fmt.Errorf("create remote directories: %w", err)
	}

	flags := os.O_RDWR | os.O_CREATE
	if item.Append {
		flags |= os.O_APPEND
	}

	dstFile, err := sftpClient.OpenFile(item.RemotePath, flags)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer dstFile.Close()

	// Replace placeholders in content
	modifiedContent := item.Content
	if item.Replace != nil {
		for key, value := range *item.Replace {
			modifiedContent = strings.ReplaceAll(modifiedContent, key, value)
		}
	}

	if item.NoDuplicate {
		content, err := io.ReadAll(dstFile)
		if err != nil {
			return fmt.Errorf("read destination file: %w", err)
		}
		existingContent := string(content)

		// Check for duplicates
		if strings.Contains(existingContent, modifiedContent) {
			return ErrRemoteFileExists
		}
	}

	if _, err := dstFile.Write([]byte(modifiedContent)); err != nil {
		return fmt.Errorf("write destination file: %w", err)
	}

	return nil
}

func copyItemSSH(client *cryptoSSH.Client, item *copyItem) error {
	// check if file exists
	var exists bool
	cmd := fmt.Sprintf("if [ -f %q ]; then echo exists; else echo missing; fi", item.RemotePath)
	existsResult, err := runWithPty(client, &[]string{cmd}, "", true)
	if err != nil {
		return fmt.Errorf("check file existence: %w", err)
	}
	exists = strings.Contains(existsResult[cmd], "exists")

	// Create remote directories
	cmd = fmt.Sprintf("mkdir -p %q", filepath.Dir(item.RemotePath))
	if _, err := runWithPty(client, &[]string{cmd}, "", false); err != nil {
		return fmt.Errorf("create remote directories: %w", err)
	}

	// Replace placeholders in content
	modifiedContent := item.Content
	if item.Replace != nil {
		for key, value := range *item.Replace {
			modifiedContent = strings.ReplaceAll(modifiedContent, key, value)
		}
	}

	if item.NoDuplicate && exists {
		cmd := fmt.Sprintf(`cat %q | tr '\n' ' '`, item.RemotePath)
		contentResult, err := runWithPty(client, &[]string{cmd}, "", false)
		if err != nil {
			return fmt.Errorf("read remote file: %w", err)
		}

		existingContent := contentResult[cmd]
		if strings.Contains(existingContent, strings.ReplaceAll(modifiedContent, "\n", " ")) {
			return ErrRemoteFileExists
		}
	}

	// Content will be written to the file in lines
	appending := exists && item.Append
	lines := strings.Split(modifiedContent, "\n")
	var cmds []string
	for _, line := range lines {
		operator := " >> "
		if !appending {
			operator = " > "
			appending = true
		}
		cmds = append(cmds, "echo '"+line+"'"+operator+item.RemotePath)
	}

	if _, err := runWithPty(client, &cmds, "", false); err != nil {
		return fmt.Errorf("write to remote file: %w", err)
	}

	return nil
}
