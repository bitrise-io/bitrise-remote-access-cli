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
	cryptoSSH "golang.org/x/crypto/ssh"
)

const (
	BitriseHostPattern = "BitriseRunningVM"
	SSHKeyName         = "bitrise_remote_access_id_rsa"
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

	log.Println("Ensuring SSH key is available")
	if err := ensureSSHKey(configEntry); err != nil {
		log.Printf("Failed to ensure SSH key: %s", err)
	} else {
		log.Println("SSH key ensured")
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
		},
	}
}

func sshConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".ssh", "config")
}

func ensureSSHKey(configEntry ConfigEntry) error {
	keyPath := filepath.Join(os.Getenv("HOME"), ".ssh", SSHKeyName)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		cmd := exec.Command("ssh-keygen", "-t", "rsa", "-b", "4096", "-f", keyPath, "-N", "")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("generate SSH key: %w", err)
		}
		log.Println("SSH key generated")
	}

	expectScript := fmt.Sprintf(`
	spawn ssh-copy-id -i %s -p %s %s@%s
	expect {
		"continue connecting (yes/no*" { send "yes\r"; exp_continue }
		"*password:*" { send "%s\r"; exp_continue }
		eof
	}
	`, keyPath, configEntry.Port, configEntry.User, configEntry.HostName, configEntry.Password)

	fmt.Println("\n--------- Copy SSH key ---------")

	cmd := exec.Command("expect", "-c", expectScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	fmt.Print("--------------------------------\n\n")
	if err != nil {
		return fmt.Errorf("copy SSH key to remote host: %w", err)
	}

	return nil
}

func RetrieveRemoteEnvVars(configEntry ConfigEntry, envVars []string) (map[string]string, error) {
	sshConfig := &cryptoSSH.ClientConfig{
		User: configEntry.User,
		Auth: []cryptoSSH.AuthMethod{
			cryptoSSH.Password(configEntry.Password),
		},
		HostKeyCallback: cryptoSSH.InsecureIgnoreHostKey(),
	}

	client, err := cryptoSSH.Dial("tcp", fmt.Sprintf("%s:%s", configEntry.HostName, configEntry.Port), sshConfig)
	if err != nil {
		return nil, fmt.Errorf("getting remote environment variables: failed to dial: %s", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("getting remote environment variables: failed to create session: %s", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer

	session.Stdout = &stdoutBuf
	envMap := make(map[string]string)

	for _, envVar := range envVars {
		cmd := "bash -lc 'printenv " + envVar + "'"
		if err := session.Run(cmd); err != nil {
			log.Printf("getting remote environment variables: failed to retrieve %s: %s", envVar, err)
			continue
		}
		envMap[envVar] = stdoutBuf.String()
	}

	return envMap, nil
}
