package ssh

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kevinburke/ssh_config"
	"github.com/pkg/sftp"
	cryptoSSH "golang.org/x/crypto/ssh"
)

const (
	BitriseHostPattern   = "BitriseRunningVM"
	SSHKeyName           = "id_bitrise_remote_access"
	localReadmeFilePath  = "assets/README_REMOTE_ACCESS.md"
	remoteReadmeFileName = "README_REMOTE_ACCESS.md"
	sourceDirEnvVar      = "BITRISE_SOURCE_DIR"
	revisionEnvVar       = "BITRISE_OSX_STACK_REV_ID"
	osTypeEnvVar         = "OSTYPE"
)

type ConfigEntry struct {
	Host     string
	HostName string
	User     string
	Port     string
	Password string
}

func EnsureSSHConfig(configEntry ConfigEntry) {
	bitriseEntryHostName := ssh_config.Get(BitriseHostPattern, "HostName")
	entryExists := strings.Contains(bitriseEntryHostName, "ngrok.io")
	if entryExists {
		log.Println("Updating SSH config entry")
		if err := updateSSHConfig(configEntry); err != nil {
			log.Printf("Failed to update SSH config: %s", err)
		} else {
			log.Println("SSH config entry updated")
		}
	} else {
		log.Println("Inserting SSH config entry")
		if err := insertSSHConfig(configEntry); err != nil {
			log.Printf("Failed to insert SSH config: %s", err)
		} else {
			log.Println("SSH config entry inserted")
		}
	}
}

func updateSSHConfig(configEntry ConfigEntry) error {
	f, _ := os.Open(sshConfigPath())
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return fmt.Errorf("decode SSH config: %w", err)
	}
	f.Close()

	f, _ = os.Create(sshConfigPath())
	defer f.Close()

	newHost := makeSSHConfigHost(configEntry)
	for i, host := range cfg.Hosts {
		if host.Patterns[0].String() == BitriseHostPattern {
			cfg.Hosts[i] = &newHost
		}
	}

	f.WriteString(cfg.String())

	return nil
}

func insertSSHConfig(configEntry ConfigEntry) error {
	f, _ := os.OpenFile(sshConfigPath(), os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	defer f.Close()

	newHost := makeSSHConfigHost(configEntry)
	f.WriteString("\n")
	f.WriteString(newHost.String())
	return nil
}

func ParseBitriseSSHSnippet(sshSnippet string, password string) (ConfigEntry, error) {
	snippetPattern := `ssh .* (.*)@(.*) -p (\d+)`
	re := regexp.MustCompile(snippetPattern)
	matches := re.FindStringSubmatch(sshSnippet)
	if len(matches) < 4 {
		return ConfigEntry{}, fmt.Errorf("invalid SSH snippet: %s", sshSnippet)
	}

	return ConfigEntry{
		Host:     BitriseHostPattern,
		HostName: matches[2],
		User:     matches[1],
		Port:     matches[3],
		Password: password,
	}, nil
}

func makeSSHConfigHost(config ConfigEntry) ssh_config.Host {
	pattern, _ := ssh_config.NewPattern(config.Host)
	return ssh_config.Host{
		Patterns: []*ssh_config.Pattern{
			pattern,
		},
		EOLComment: " Bitrise CI VM",
		Nodes: []ssh_config.Node{
			&ssh_config.KV{
				Key:   "  HostName",
				Value: config.HostName,
			},
			&ssh_config.KV{
				Key:   "  User",
				Value: config.User,
			},
			&ssh_config.KV{
				Key:   "  Port",
				Value: config.Port,
			},
			&ssh_config.KV{
				Key:   "  StrictHostKeyChecking",
				Value: "no", // Don't prompt for adding the host to known_hosts
			},
			&ssh_config.KV{
				Key:   "  CheckHostIP",
				Value: "no", // https://serverfault.com/questions/1040512/how-does-the-ssh-option-checkhostip-yes-really-help-me
			},
			&ssh_config.KV{
				Key:   "  IdentityFile",
				Value: "~/.ssh/" + SSHKeyName, // Use the generated SSH key for authentication
			},
			&ssh_config.KV{
				Key:   "  IdentitiesOnly",
				Value: "yes", // Only use the specified identity file
			},
		},
	}
}

func sshConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".ssh", "config")
}

func EnsureSSHKey(configEntry ConfigEntry) error {
	keyPath := filepath.Join(os.Getenv("HOME"), ".ssh", SSHKeyName)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-C", "Bitrise remote access key", "-N", "")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("generate SSH key: %w", err)
		}
	}

	command := strings.Join([]string{"ssh-copy-id", "-i", keyPath, "-p", configEntry.Port, "-o", "StrictHostKeyChecking=no", fmt.Sprintf("%s@%s", configEntry.User, configEntry.HostName)}, " ")

	expectScript := fmt.Sprintf(`
	spawn %s
	expect {
		"continue connecting (yes/no*" { send "yes\r"; exp_continue }
		"*password:*" { send "%s\r"; exp_continue }
		eof
	}
	`, command, configEntry.Password)

	cmd := exec.Command("expect", "-c", expectScript)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		fmt.Println("\n--------- Copy SSH key ---------")
		fmt.Print(out.String())
		fmt.Print("--------------------------------\n\n")
		return fmt.Errorf("copy SSH key to remote host: %w", err)
	}

	return nil
}

func connectSSHClient(configEntry ConfigEntry) (*cryptoSSH.Client, error) {
	sshConfig := &cryptoSSH.ClientConfig{
		User: configEntry.User,
		Auth: []cryptoSSH.AuthMethod{
			cryptoSSH.Password(configEntry.Password),
		},
		HostKeyCallback: cryptoSSH.InsecureIgnoreHostKey(),
	}

	client, err := cryptoSSH.Dial("tcp", fmt.Sprintf("%s:%s", configEntry.HostName, configEntry.Port), sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %s", err)
	}

	return client, nil
}

func createSSHSession(client *cryptoSSH.Client) (*cryptoSSH.Session, error) {
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create session: %s", err)
	}

	return session, nil
}

func retrieveEnvVar(client *cryptoSSH.Client, envVar string, command string) (string, error) {
	session, err := createSSHSession(client)
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	fullCmd := strings.ReplaceAll(command, "VAR", envVar)

	err = session.Run(fullCmd)

	if err != nil {
		return "", fmt.Errorf("failed to retrieve %s with command '%s': %s", envVar, fullCmd, err)
	}

	return strings.TrimSpace(stdoutBuf.String()), nil
}

func getRemoteEnvVars(client *cryptoSSH.Client, envVars []string) (map[string]string, error) {
	envMap := make(map[string]string)

	for _, envVar := range envVars {
		value, err := retrieveEnvVar(client, envVar, "bash -lc 'echo $VAR'")
		if err != nil || value == "" {
			log.Print(err.Error())
			continue
		}
		envMap[envVar] = value
	}
	return envMap, nil
}

func copyFileToRemote(client *cryptoSSH.Client, localFilePath, remoteFilePath string, replace map[string]string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %s", err)
	}
	defer sftpClient.Close()

	if _, err := sftpClient.Stat(remoteFilePath); err == nil {
		return fmt.Errorf("remote file already exists: %s", remoteFilePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if remote file exists: %s", err)
	}

	dstFile, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	content, err := os.ReadFile(localFilePath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %s", err)
	}

	modifiedContent := string(content)
	for key, value := range replace {
		modifiedContent = strings.ReplaceAll(modifiedContent, key, value)
	}

	if _, err := dstFile.Write([]byte(modifiedContent)); err != nil {
		return fmt.Errorf("failed to write to destination file: %s", err)
	}

	return nil
}

func SetupRemote(configEntry ConfigEntry) (string, error) {
	log.Println("Setting up remote environment...")
	client, err := connectSSHClient(configEntry)
	if err != nil {
		return "", err
	}
	defer client.Close()

	envMap, err := getRemoteEnvVars(client, []string{sourceDirEnvVar, osTypeEnvVar, revisionEnvVar})
	if err != nil {
		return "", err
	}

	sourceDir := envMap[sourceDirEnvVar]

	if isMacOS(envMap[osTypeEnvVar]) {
		log.Println("Ensuring SSH key is available...")
		if err := EnsureSSHKey(configEntry); err != nil {
			log.Printf("Failed to ensure SSH key: %s", err)
		} else {
			log.Println("SSH key ensured")
		}

		remotePath := filepath.Join(sourceDir, remoteReadmeFileName)
		replaceInFile := map[string]string{
			sourceDirEnvVar: sourceDir,
			revisionEnvVar:  envMap[revisionEnvVar],
		}

		log.Printf("Copying README file to remote...")
		if err := copyFileToRemote(client, localReadmeFilePath, remotePath, replaceInFile); err != nil {
			log.Printf("Failed to copy README file to remote: %s", err)
		} else {
			log.Println("README file copied")
		}
	} else {
		log.Println("Skipping SSH key and README file setup for non-macOS stack")
	}
	return sourceDir, nil
}

func isMacOS(osType string) bool {
	return strings.Contains(osType, "darwin")
}
