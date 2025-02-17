package vscode

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/bitrise-io/bitrise-remote-access-cli/ide"
)

const (
	ideIdentifier          = "vscode"
	ideName                = "Visual Studio Code"
	sshExtensionIdentifier = "ms-vscode-remote.remote-ssh"
	sshExtensionName       = "Remote - SSH"
	codePathMac            = "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"
	urlInstallVSCode       = "https://code.visualstudio.com/docs/setup/setup-overview"
	urlAddVSCodeToPath     = "https://code.visualstudio.com/docs/setup/mac#_launch-vs-code-from-the-command-line"
)

var IdeData = ide.IDE{
	Identifier: ideIdentifier,
	Name:       ideName,
	Aliases:    []string{"code"},
	OnOpen:     openInVSCode,
	OnTestPath: isVSCodeInstalled}

func openInVSCode(hostPattern, folderPath string) error {
	codePath, installed := isVSCodeInstalled()
	if !installed {
		log.Printf(`
		
%s is either not installed or it is not added to $PATH
Please visit the following sites for more info:
- installing: %s
- adding to path: %s

		`, ideName, urlInstallVSCode, urlAddVSCodeToPath)
		return fmt.Errorf("%s CLI not found in $PATH", ideIdentifier)
	}

	if !prepareSSHExtension() {
		fmt.Println("Ending session...")
		return fmt.Errorf("%s does not have the necessary extensions installed", ideName)
	}

	log.Printf("Opening %s...", folderPath)

	openPath := fmt.Sprintf("--folder-uri=vscode-remote://ssh-remote+%s%s/", hostPattern, folderPath)

	cmd := exec.Command(codePath, openPath)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("open %s window: %w", ideName, err)
	}

	return nil
}

func isVSCodeInstalled() (string, bool) {
	codePath, err := exec.LookPath("code")
	if err == nil {
		return codePath, true
	}

	_, err = os.Stat(codePathMac)
	return codePathMac, err == nil
}

func isSSHExtensionInstalled() bool {
	cmd := exec.Command("code", "--list-extensions")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	if strings.Contains(string(out), sshExtensionIdentifier) {
		return true
	}

	return false
}

func prepareSSHExtension() bool {
	if isSSHExtensionInstalled() {
		return true
	} else {
		log.Printf("%s does not have the necessary \"%s\" extension installed\n", ideName, sshExtensionName)
		log.Print("Would you like to install it? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')

		clearLines(3)

		if response == "y\n" {
			log.Println("Installing extensions...")

			cmd := exec.Command("code", "--install-extension", sshExtensionIdentifier)

			if out, err := cmd.Output(); err != nil {
				fmt.Println("\n------ Install extensions ------")
				log.Printf("Failed to install %s extension\nreason: %s\n\noutput:\n%s\n", sshExtensionIdentifier, err, out)
				fmt.Print("\n--------------------------------\n\n")
				return false
			}
			return isSSHExtensionInstalled()
		} else {
			return false
		}
	}
}

func clearLines(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[1A")
		fmt.Print("\033[2K")
	}
}
