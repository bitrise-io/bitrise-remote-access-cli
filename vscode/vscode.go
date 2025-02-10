package vscode

import (
	"bufio"
	"fmt"
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
		fmt.Println("Ending session.")
		return fmt.Errorf("%s CLI not found in $PATH", ideIdentifier)
	}

	if !prepareSSHExtension() {
		fmt.Println("Ending session.")
		return fmt.Errorf("%s does not have the necessary extensions installed", ideName)
	}

	fmt.Println("Opening...")

	cmd := exec.Command(codePath, fmt.Sprintf("--folder-uri=vscode-remote://ssh-remote+%s/%s", hostPattern, folderPath))

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
		fmt.Printf("%s does not have the necessary \"%s\" extension installed\n", ideName, sshExtensionName)
		fmt.Print("Would you like to install it? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if response == "y\n" {
			cmd := exec.Command("code", "--install-extension", sshExtensionIdentifier)
			out, _ := cmd.Output()
			fmt.Print(string(out))
			return isSSHExtensionInstalled()
		} else {
			return false
		}
	}
}
