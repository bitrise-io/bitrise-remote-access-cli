package ssh

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

func EnsureSSHConfig(configEntry ConfigEntry, addIdentityKey bool) error {
	log.Println("Ensuring Bitrise SSH config inclusion...")
	if err := ensureBitriseConfigIncluded(); err != nil {
		return fmt.Errorf("failed to ensure Bitrise SSH config inclusion: %s", err)
	} else {
		log.Println("Bitrise SSH config inclusion ensured")
	}

	log.Println("Updating SSH config entry...")
	if err := writeSSHConfig(configEntry, addIdentityKey); err != nil {
		return fmt.Errorf("failed to update SSH config: %s", err)
	} else {
		log.Println("SSH config entry updated")
	}

	return nil
}

func ensureBitriseConfigIncluded() error {
	sshConfigPath := sshConfigPath()
	includeLine := fmt.Sprintf("Include %s", bitriseConfigPath())

	f, err := os.Open(sshConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(sshConfigPath, []byte(includeLine+"\n"), 0644)
		}
		return err
	}
	defer f.Close()

	lines := make([]string, 0)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == includeLine {
			return nil
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	description := "# Added by Bitrise\n# This will be added again if you remove it."

	lines = append([]string{description, includeLine}, lines...)

	newContent := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(sshConfigPath, []byte(newContent), 0644)
}

func writeSSHConfig(configEntry ConfigEntry, addIdentityKey bool) error {
	newHost := makeSSHConfigHost(configEntry, addIdentityKey)
	trimmedHost := strings.TrimSpace(newHost.String())
	content := "# --- Bitrise Generated ---\n" + trimmedHost + "\n# -------------------------\n"

	configDir := bitriseConfigPath()

	parentDir := filepath.Dir(configDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.OpenFile(configDir, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("error opening file: %s", err)
	}
	defer file.Close()

	_, err = file.WriteString(content)

	return err
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

func makeSSHConfigHost(config ConfigEntry, addIdentityKey bool) ssh_config.Host {

	// Space after hostname but before comment is important but there is no other way
	// so we have to add it to the pattern. The built in methods will trim hostnames and
	// add spaces after them based on the pattern.
	pattern, _ := ssh_config.NewPattern(fmt.Sprintf("%s ", config.Host))

	nodes := []ssh_config.Node{
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
	}

	if addIdentityKey {
		nodes = append(nodes, &ssh_config.KV{
			Key:   "  IdentityFile",
			Value: "~/.ssh/" + SSHKeyName, // Use the generated SSH key for authentication
		})
		nodes = append(nodes, &ssh_config.KV{
			Key:   "  IdentitiesOnly",
			Value: "yes", // Only use the specified identity file
		})
	}

	return ssh_config.Host{
		Patterns: []*ssh_config.Pattern{
			pattern,
		},
		EOLComment: "Bitrise CI VM",
		Nodes:      nodes,
	}
}

func getHomeDir() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("USERPROFILE")
	}
	return os.Getenv("HOME")
}

func sshConfigPath() string {
	return filepath.Join(getHomeDir(), ".ssh", "config")
}

func bitriseConfigPath() string {
	return filepath.Join(getHomeDir(), ".bitrise", "remote-access", "ssh_config")
}

func EnsureSSHKey(configEntry ConfigEntry) error {
	keyPath := filepath.Join(getHomeDir(), ".ssh", SSHKeyName)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-C", "Bitrise remote access key", "-N", "")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("generate SSH key: %w", err)
		}
	}

	command := strings.Join([]string{"ssh-copy-id", "-i", fmt.Sprintf("\"%s\"", keyPath), "-p", configEntry.Port, "-o", "StrictHostKeyChecking=no", fmt.Sprintf("%s@%s", configEntry.User, configEntry.HostName)}, " ")

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

func removeHostKey(configEntry ConfigEntry) error {
	hostname := fmt.Sprintf("[%s]:%s", configEntry.HostName, configEntry.Port)
	cmd := exec.Command("ssh-keygen", "-R", hostname)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		fmt.Println("\n--------- Remove Host Key ---------")
		fmt.Print(out.String())
		fmt.Print("-----------------------------------\n\n")
		return fmt.Errorf("failed to remove host key for %s: %w", hostname, err)
	}

	return nil

}

func addMotdToShellConfig(client *cryptoSSH.Client, shellConfigs []string) error {
	for _, config := range shellConfigs {
		cmd := fmt.Sprintf(`grep -qxF "cat /etc/motd" %s || echo "cat /etc/motd" >> %s`, config, config)
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create SSH session: %w", err)
		}
		defer session.Close()

		if err := session.Run(cmd); err != nil {
			return fmt.Errorf("failed to modify remote shell config %s: %w", config, err)
		}
	}
	return nil
}

func SetupRemote(configEntry ConfigEntry) (bool, string, error) {
	log.Println("Setting up remote environment...")

	log.Println("Removing old host key...")
	if err := removeHostKey(configEntry); err != nil {
		return false, "", err
	} else {
		log.Println("Old host key removed successfully or was not present")
	}

	isMacOs := false
	client, err := connectSSHClient(configEntry)
	if err != nil {
		return isMacOs, "", err
	}
	defer client.Close()

	envMap, err := getRemoteEnvVars(client, []string{sourceDirEnvVar, osTypeEnvVar, revisionEnvVar})
	if err != nil {
		return isMacOs, "", err
	}

	sourceDir := envMap[sourceDirEnvVar]

	if isMacOS(envMap[osTypeEnvVar]) {
		isMacOs = true
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

		// Linux stacks' sshd_config is located at /etc/ssh/sshd_config and it should be updated, because
		// PrintMotd is set to 'no', but before that can be changed the ssh key availability should be ensured on Linux
		// stacks too.
		log.Println("Adding message of the day to shell configs...")
		if err := addMotdToShellConfig(client, []string{"~/.zshrc", "~/.bashrc"}); err != nil {
			log.Println("Error modifying shell config:", err)
		} else {
			log.Println("MOTD added to shell configs")
		}
	} else {
		// Skipping SSH key and README file setup for non-macOS stack because we encountered issues with ssh-copy-id and it's probably caused by our Linux stack setup where the VM runs a Docker container and remote access connects the two with `docker exec`.
		// The error message is "bash: line 1: ssh-ed25519: command not found"
		sourceDir = "/bitrise/src"
	}
	return isMacOs, sourceDir, nil
}

func isMacOS(osType string) bool {
	return strings.Contains(osType, "darwin")
}
