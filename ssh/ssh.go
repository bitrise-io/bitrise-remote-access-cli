package ssh

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kevinburke/ssh_config"
)

const BitriseHostPattern = "BitriseRunningVM"

type ConfigEntry struct {
	Host     string
	HostName string
	User     string
	Port     string
}

func EnsureSSHConfig(configEntry ConfigEntry) {

	bitriseEntryHostName := ssh_config.Get(BitriseHostPattern, "HostName")
	entryExists := strings.Contains(bitriseEntryHostName, "ngrok.io")
	if entryExists {
		log.Println("Updating SSH config entry")
		updateSSHConfig(configEntry)
	} else {
		log.Println("Inserting SSH config entry")
		insertSSHConfig(configEntry)
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

func ParseBitriseSSHSnippet(sshSnippet string) (ConfigEntry, error) {
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
		},
	}
}

func sshConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".ssh", "config")
}
