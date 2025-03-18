package ssh

import (
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

func fileExistsSFTP(client *sftp.Client, item *copyItem) (bool, error) {
	stat, err := client.Stat(item.RemotePath)
	if err == nil {
		// File exists
		if !item.Append {
			return true, fmt.Errorf("remote file already exists: %s", item.RemotePath)
		}
		if item.NoDuplicate {
			file, err := client.Open(item.RemotePath)
			if err != nil {
				return true, fmt.Errorf("open remote file: %w", err)
			}
			defer file.Close()

			fileContent, err := io.ReadAll(file)
			if err != nil {
				return true, fmt.Errorf("read remote file content: %w", err)
			}

			if strings.Contains(string(fileContent), item.Content) {
				return true, fmt.Errorf("remote file already contains the content")
			}
		}
		return true, nil
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("check file existence: %w", err)
	}
	return stat != nil, nil
}

func copyItemSFTP(client *cryptoSSH.Client, item *copyItem) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	exists, err := fileExistsSFTP(sftpClient, item)
	if err != nil {
		return fmt.Errorf("copy to remote: %w", err)
	}

	if err := sftpClient.MkdirAll(filepath.Dir(item.RemotePath)); err != nil {
		return fmt.Errorf("create remote directories: %w", err)
	}

	var dstFile *sftp.File
	if exists && item.Append {
		dstFile, err = sftpClient.OpenFile(item.RemotePath, os.O_APPEND|os.O_WRONLY)
		if err != nil {
			return fmt.Errorf("open file for appending: %w", err)
		}
	} else {
		dstFile, err = sftpClient.Create(item.RemotePath)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}
	}
	defer dstFile.Close()

	// Replace placeholders in content
	modifiedContent := item.Content
	if item.Replace != nil {
		for key, value := range *item.Replace {
			modifiedContent = strings.ReplaceAll(modifiedContent, key, value)
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
	if err := runWithPty(client, &[]string{"cd ~", cmd}, "", &result); err != nil {
		return false, fmt.Errorf("check file existence: %w", err)
	}

	exists := strings.TrimSpace(result[cmd]) == "exists"
	return exists, nil
}

func copyItemSSH(client *cryptoSSH.Client, item *copyItem) error {
	exists, err := fileExistsSSH(client, item)
	if err != nil {
		return fmt.Errorf("check remote file: %w", err)
	}

	cmd := fmt.Sprintf("mkdir -p %q", filepath.Dir(item.RemotePath))
	if err := runWithPty(client, &[]string{"cd ~", cmd}, "", nil); err != nil {
		return fmt.Errorf("create remote directories: %w", err)
	}

	var cmdWrite string
	if exists && item.Append {
		cmdWrite = fmt.Sprintf("echo %q >> %q", item.Content, item.RemotePath)
	} else {
		cmdWrite = fmt.Sprintf("echo %q > %q", item.Content, item.RemotePath)
	}

	if err := runWithPty(client, &[]string{"cd ~", cmdWrite}, "", nil); err != nil {
		return fmt.Errorf("write to remote file: %w", err)
	}

	return nil
}
