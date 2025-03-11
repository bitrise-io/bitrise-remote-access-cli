package ssh

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
	"github.com/pkg/sftp"
	cryptoSSH "golang.org/x/crypto/ssh"
)

const (
	BitriseHostPattern   = "BitriseRunningVM"
	SSHKeyName           = "id_bitrise_remote_access"
	remoteReadmeFileName = "README_REMOTE_ACCESS.md"
	sourceDirEnvVar      = "BITRISE_SOURCE_DIR"
	revisionEnvVar       = "BITRISE_OSX_STACK_REV_ID"
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
	log.Println("Ensuring Bitrise SSH config inclusion...")
	if err := ensureBitriseClientConfigIncluded(); err != nil {
		return fmt.Errorf("ensure Bitrise SSH config inclusion: %w", err)
	} else {
		log.Println("Bitrise SSH config inclusion ensured")
	}

	log.Println("Updating SSH config entry...")
	if err := writeSSHClientConfig(configEntry, addIdentityKey); err != nil {
		return fmt.Errorf("update SSH config: %w", err)
	} else {
		log.Println("SSH config entry updated")
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

func EnsureClientKeyOnRemote(configEntry *ConfigEntry) error {
	var password string

	if configEntry.Password != nil {
		password = *configEntry.Password
	} else {
		return fmt.Errorf("no password provided")
	}

	keyPath := filepath.Join(getHomeDir(), ".ssh", SSHKeyName)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-C", "Bitrise remote access key", "-N", "")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("generate SSH key: %w", err)
		}
	}

	command := strings.Join([]string{
		"ssh-copy-id",
		"-i", fmt.Sprintf("\"%s\"", keyPath),
		"-p", configEntry.Port,
		"-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", configEntry.User, configEntry.HostName),
	}, " ")
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		powershellScript := fmt.Sprintf(`
		$command = = @'
%s
'@
		$password = "%s"
		$securePassword = ConvertTo-SecureString $password -AsPlainText -Force
		$credential = New-Object System.Management.Automation.PSCredential ("dummy", $securePassword)
		Invoke-Command -ScriptBlock {
			param ($command, $credential)
			Start-Process -FilePath "powershell.exe" -ArgumentList "-Command", $command -Credential $credential -NoNewWindow -Wait
		} -ArgumentList $command, $credential
		`, command, password)

		cmd = exec.Command("powershell", "-Command", powershellScript)
	} else {
		expectScript := fmt.Sprintf(`
		spawn %s
		expect {
			"continue connecting (yes/no*" { send "yes\r"; exp_continue }
			"*password:*" { send "%s\r"; exp_continue }
			eof
		}
		exit
		`, command, password)

		cmd = exec.Command("expect", "-c", expectScript)
	}

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

func retrieveEnvVar(client *cryptoSSH.Client, envVar string) (string, error) {
	session, err := createSSHSession(client)
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	fullCmd := fmt.Sprintf("bash -lc 'echo $%s'", envVar)

	err = session.Run(fullCmd)
	if err != nil {
		return "", fmt.Errorf("retrieve $%s: %w", envVar, err)
	}

	output := stdoutBuf.String()
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line, nil
		}
	}

	return "", fmt.Errorf("retrieve '%s' environment variable: no valid output", envVar)
}

func getRemoteEnvVars(client *cryptoSSH.Client, envVars []string) (map[string]string, error) {
	envMap := make(map[string]string)

	for _, envVar := range envVars {
		value, err := retrieveEnvVar(client, envVar)
		if err != nil || value == "" {
			log.Printf("retrieve %s: %s", envVar, err)
			continue
		}
		envMap[envVar] = value
	}
	return envMap, nil
}

func copyReadmeToRemote(client *cryptoSSH.Client, remoteFilePath string, replace map[string]string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	if _, err := sftpClient.Stat(remoteFilePath); err == nil {
		return fmt.Errorf("remote file already exists: %s", remoteFilePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check file existence: %w", err)
	}

	dstFile, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	modifiedContent := readmeFile
	for key, value := range replace {
		modifiedContent = strings.ReplaceAll(modifiedContent, key, value)
	}

	if _, err := dstFile.Write([]byte(modifiedContent)); err != nil {
		return fmt.Errorf("write destination file: %w", err)
	}

	return nil
}

func removeHostKey(configEntry *ConfigEntry) error {
	hostname := fmt.Sprintf("[%s]:%s", configEntry.HostName, configEntry.Port)
	cmd := exec.Command("ssh-keygen", "-R", hostname)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		fmt.Println("\n--------- Remove Host Key ---------")
		fmt.Print(out.String())
		fmt.Print("-----------------------------------\n\n")
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
	log.Println("Setting up SSH config of remote host...")

	log.Println("Removing old host key...")
	if err := removeHostKey(configEntry); err != nil {
		return false, "", err
	} else {
		log.Println("No old host keys remaining")
	}

	if configEntry.Password == nil {
		return false, "", nil
	}

	isMacOs := false
	log.Println("Connecting to remote host...")
	client, err := connectSSHClient(configEntry)
	if err != nil {
		return isMacOs, "", err
	}
	log.Println("Connected to remote host")
	defer client.Close()

	log.Println("Detecting remote environment...")
	envMap, err := getRemoteEnvVars(client, []string{sourceDirEnvVar, osTypeEnvVar, revisionEnvVar})
	if err != nil {
		return isMacOs, "", err
	}

	sourceDir := envMap[sourceDirEnvVar]

	isMacOs = isMacOS(envMap[osTypeEnvVar])
	if isMacOs {
		log.Println("Ensuring SSH key is available...")
		if err := EnsureClientKeyOnRemote(configEntry); err != nil {
			return isMacOs, sourceDir, fmt.Errorf("ensure SSH key available on remote: %w", err)
		} else {
			log.Println("SSH key ensured")
		}

		remotePath := filepath.Join(sourceDir, remoteReadmeFileName)
		replaceInFile := map[string]string{
			sourceDirEnvVar: sourceDir,
			revisionEnvVar:  envMap[revisionEnvVar],
		}

		log.Printf("Copying README file to remote...")
		if err := copyReadmeToRemote(client, remotePath, replaceInFile); err != nil {
			log.Printf("copy README file to remote: %s", err)
		} else {
			log.Println("README file copied")
		}

		// Linux stacks' sshd_config is located at /etc/ssh/sshd_config and it should be updated, because
		// PrintMotd is set to 'no', but before that can be changed the ssh key availability should be ensured on Linux
		// stacks too.
		log.Println("Adding message of the day to shell configs...")
		if err := setupShellConfigs(client, []string{"~/.zshrc", "~/.bashrc"}); err != nil {
			log.Printf("modifying shell config: %s", err)
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
