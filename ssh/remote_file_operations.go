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

func fileExistsSSH(client *cryptoSSH.Client, item *copyItem) (bool, error) {
	var result map[string]string
	cmd := fmt.Sprintf("test -f %q && echo exists || echo missing", item.RemotePath)
	if err := runWithPty(client, &[]string{cmd}, "", &result); err != nil {
		return false, fmt.Errorf("check file existence: %w", err)
	}

	if strings.TrimSpace(result[cmd]) == "exists" {
		return true, ErrRemoteFileExists
	}

	return false, nil
}

func copyItemSSH(client *cryptoSSH.Client, item *copyItem) error {
	exists, err := fileExistsSSH(client, item)
	if err != nil {
		return fmt.Errorf("check remote file: %w", err)
	}

	cmd := fmt.Sprintf("mkdir -p %q", filepath.Dir(item.RemotePath))
	if err := runWithPty(client, &[]string{cmd}, "", nil); err != nil {
		return fmt.Errorf("create remote directories: %w", err)
	}

	// Replace placeholders in content
	modifiedContent := item.Content
	if item.Replace != nil {
		for key, value := range *item.Replace {
			modifiedContent = strings.ReplaceAll(modifiedContent, key, value)
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
		}
		cmds = append(cmds, "echo '"+line+"'"+operator+item.RemotePath)
		appending = true
	}

	if err := runWithPty(client, &cmds, "", nil); err != nil {
		return fmt.Errorf("write to remote file: %w", err)
	}

	return nil
}
