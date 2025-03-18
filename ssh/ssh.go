package ssh

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/bitrise-io/bitrise-remote-access-cli/logger"
	"github.com/kevinburke/ssh_config"
	cryptoSSH "golang.org/x/crypto/ssh"
)

const (
	BitriseHostPattern   = "BitriseRunningVM"
	SSHKeyName           = "id_bitrise_remote_access"
	remoteReadmeFileName = "README_REMOTE_ACCESS.md"
	sourceDirEnvVar      = "BITRISE_SOURCE_DIR"
	revisionEnvVar       = "BITRISE_OSX_STACK_REV_ID"
	revisionEnvVarUbuntu = "BITRISE_STACK_REV_ID"
	osTypeEnvVar         = "OSTYPE"
)

//go:embed README_REMOTE_ACCESS.md
var readmeFile string

type ConfigEntry struct {
	Host     string
	HostName string
	User     string
	Port     string
	Password *string
}

func SetupClientConfig(configEntry *ConfigEntry, addIdentityKey bool) error {
	logger.Info("Ensuring Bitrise SSH config inclusion...")
	if err := ensureBitriseClientConfigIncluded(); err != nil {
		return fmt.Errorf("ensure Bitrise SSH config inclusion: %w", err)
	} else {
		logger.Success("Bitrise SSH config inclusion ensured")
	}

	logger.Info("Updating SSH config entry...")
	if err := writeSSHClientConfig(configEntry, addIdentityKey); err != nil {
		return fmt.Errorf("update SSH config: %w", err)
	} else {
		logger.Success("SSH config entry updated")
	}

	return nil
}

func ensureBitriseClientConfigIncluded() error {
	sshConfigPath := sshConfigPath()
	includeLine := fmt.Sprintf("Include %s", bitriseConfigPath())

	f, err := os.Open(sshConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(sshConfigPath), 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
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

func writeSSHClientConfig(configEntry *ConfigEntry, addIdentityKey bool) error {
	newHost := makeSSHConfigHost(configEntry, addIdentityKey)
	trimmedHost := strings.TrimSpace(newHost.String())
	content := "# --- Bitrise Generated ---\n" + trimmedHost + "\n# -------------------------\n"

	configDir := bitriseConfigPath()

	parentDir := filepath.Dir(configDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	file, err := os.OpenFile(configDir, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(content)

	return err
}

func CreateClientConfig(host, port, user string, password *string) (*ConfigEntry, error) {
	switch "" {
	case host:
		return nil, fmt.Errorf("host cannot be empty")
	case port:
		return nil, fmt.Errorf("port cannot be empty")
	case user:
		return nil, fmt.Errorf("user cannot be empty")
	}

	if net.ParseIP(host) == nil {
		if _, err := net.LookupHost(host); err != nil {
			return nil, fmt.Errorf("invalid host: %s", host)
		}
	}

	if p, err := strconv.Atoi(port); err != nil || p <= 0 || p > 65535 {
		return nil, fmt.Errorf("invalid port: %s", port)
	}

	configEntry := &ConfigEntry{
		Host:     BitriseHostPattern,
		HostName: host,
		User:     user,
		Port:     port,
		Password: password,
	}

	return configEntry, nil
}

func makeSSHConfigHost(config *ConfigEntry, useIdentityOnly bool) ssh_config.Host {
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

	if useIdentityOnly {
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

func EnsureClientKeyOnRemote(client *cryptoSSH.Client, configEntry *ConfigEntry, ubuntu bool) error {
	keyPath := filepath.Join(getHomeDir(), ".ssh", SSHKeyName)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-C", "Bitrise remote access key", "-N", "")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("generate SSH key: %w", err)
		}
	}

	pubKeyPath := keyPath + ".pub"
	pubKey, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("read public key: %w", err)
	}

	remotePath := ".ssh/authorized_keys"

	item := &copyItem{
		Content:     string(pubKey),
		RemotePath:  remotePath,
		Append:      true,
		NoDuplicate: true,
	}

	if err := copyItemSFTP(client, item); err != nil {
		return fmt.Errorf("append public key to remote authorized_keys: %w", err)
	}

	return nil
}

func connectSSHClient(configEntry *ConfigEntry) (*cryptoSSH.Client, error) {
	password := configEntry.Password

	if password == nil {
		return nil, fmt.Errorf("trying to connect without password")
	}

	sshConfig := &cryptoSSH.ClientConfig{
		User: configEntry.User,
		Auth: []cryptoSSH.AuthMethod{
			cryptoSSH.Password(*password),
		},
		HostKeyCallback: cryptoSSH.InsecureIgnoreHostKey(),
	}

	client, err := cryptoSSH.Dial("tcp", fmt.Sprintf("%s:%s", configEntry.HostName, configEntry.Port), sshConfig)
	if err != nil {
		if opErr, ok := err.(*net.OpError); ok {
			return nil, opErr
		}
		return nil, fmt.Errorf("start client connection: %w, %T", err, err)
	}

	return client, nil
}

func createSSHSession(client *cryptoSSH.Client) (*cryptoSSH.Session, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return session, nil
}

func removeHostKey(configEntry *ConfigEntry) error {
	hostname := fmt.Sprintf("[%s]:%s", configEntry.HostName, configEntry.Port)
	cmd := exec.Command("ssh-keygen", "-R", hostname)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		logger.PrintFormattedOutput("Remove Host Key", out.String())
		return fmt.Errorf("remove host key for %s: %w", hostname, err)
	}

	return nil

}

func addMotdToShellConfig(client *cryptoSSH.Client, shellConfig string) error {
	cmd := fmt.Sprintf(`grep -qxF "cat /etc/motd" %s || echo -e "\ncat /etc/motd\n" >> %s`, shellConfig, shellConfig)
	session, err := createSSHSession(client)
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	if err = session.Run(cmd); err != nil {
		return fmt.Errorf("edit remote shell config '%s': %w", shellConfig, err)
	}
	return nil
}

func setupShellConfigs(client *cryptoSSH.Client, shellConfigs []string) error {
	for _, config := range shellConfigs {
		if err := addMotdToShellConfig(client, config); err != nil {
			return err
		}
	}
	return nil
}

func SetupRemoteConfig(configEntry *ConfigEntry) (bool, string, error) {
	logger.Info("Setting up SSH config of remote host...")

	logger.Info("Removing old host key...")
	if err := removeHostKey(configEntry); err != nil {
		return false, "", err
	} else {
		logger.Success("No old host keys remaining")
	}

	if configEntry.Password == nil {
		return false, "", nil
	}

	isMacOs := false
	logger.Info("Connecting to remote host...")
	client, err := connectSSHClient(configEntry)
	if err != nil {
		return isMacOs, "", err
	}
	defer client.Close()

	logger.Info("Detecting remote environment...")
	envMap := make(map[string]string)
	err = runWithPty(client, &[]string{sourceDirEnvVar, osTypeEnvVar, revisionEnvVar, revisionEnvVarUbuntu}, "echo $", &envMap)
	if err != nil {
		return isMacOs, "", err
	}

	sourceDir := envMap[sourceDirEnvVar]

	isMacOs = isMacOS(envMap[osTypeEnvVar])
	if isMacOs {
		logger.Info("Ensuring SSH key is available...")
		if err := EnsureClientKeyOnRemote(client, configEntry, !isMacOs); err != nil {
			return isMacOs, sourceDir, fmt.Errorf("ensure SSH key available on remote: %w", err)
		} else {
			logger.Success("SSH key ensured")
		}

		revision := envMap[revisionEnvVar]
		if revision == "" {
			// Ubuntu stack stores the revision in a different environment variable
			revision = envMap[revisionEnvVarUbuntu]
		}
		remotePath := filepath.Join(sourceDir, remoteReadmeFileName)
		replaceInFile := map[string]string{
			sourceDirEnvVar: sourceDir,
			revisionEnvVar:  revision,
		}

		logger.Info("Copying README file to remote...")
		item := &copyItem{
			Content:    string(readmeFile),
			RemotePath: remotePath,
			Replace:    &replaceInFile,
		}
		if err := copyItemSFTP(client, item); err != nil {
			logger.Warnf("copy README file to remote: %s", err)
		} else {
			logger.Success("README file copied")
		}

		// Linux stacks' sshd_config is located at /etc/ssh/sshd_config and it should be updated, because
		// PrintMotd is set to 'no', but before that can be changed the ssh key availability should be ensured on Linux
		// stacks too.
		logger.Info("Adding message of the day to shell configs...")
		if err := setupShellConfigs(client, []string{"~/.zshrc", "~/.bashrc"}); err != nil {
			logger.Infof("modifying shell config: %s", err)
		} else {
			logger.Success("MOTD added to shell configs")
		}
	} else {
		// Skipping SSH key and README file setup for non-macOS stack because we encountered issues with ssh-copy-id and it's probably caused by our Linux stack setup where the VM runs a Docker container and remote access connects the two with `docker exec`.
		// The error message is "bash: line 1: ssh-ed25519: command not found"
		if sourceDir == "" {
			sourceDir = "/bitrise/src"
		}
	}
	return isMacOs, sourceDir, nil
}

func isMacOS(osType string) bool {
	return strings.Contains(osType, "darwin")
}
