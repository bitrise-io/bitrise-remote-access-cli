package vscode

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bitrise-io/bitrise-remote-access-cli/ide"
	"github.com/bitrise-io/bitrise-remote-access-cli/logger"
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

func openInVSCode(hostPattern, folderPath, additionalInfo string) error {
	codePath, installed := isVSCodeInstalled()
	if !installed {
		logger.Infof(`
		
%s is either not installed or it is not added to $PATH
Please visit the following sites for more info:
- installing: %s
- adding to path: %s

		`, ideName, urlInstallVSCode, urlAddVSCodeToPath)
		return fmt.Errorf("%s CLI not found in $PATH", ideIdentifier)
	}

	if !prepareSSHExtension() {
		logger.Info("Ending session...")
		return fmt.Errorf("%s does not have the necessary extensions installed", ideName)
	}

	if additionalInfo != "" {
		header := fmt.Sprintf("Opening %s", ideName)
		logger.PrintFormattedOutput(header, fmt.Sprintf("Source code location:\n\n%s\n\n%s", folderPath, additionalInfo))
	} else {
		logger.Infof("Opening %s...", folderPath)
	}

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
		confirm, err := logger.Confirm(
			fmt.Sprintf("%s does not have the necessary \"%s\" extension installed\nWould you like to install it?", ideName, sshExtensionName),
			"Installing extensions...",
			"Ending session...")
		if err != nil || !confirm {
			return false
		}

		cmd := exec.Command("code", "--install-extension", sshExtensionIdentifier)

		if out, err := cmd.Output(); err != nil {
			logger.PrintFormattedOutput("Install extensions", fmt.Sprintf("install %s extension\nreason: %s\n\noutput:\n%s\n", sshExtensionIdentifier, err, out))
			return false
		}
		return isSSHExtensionInstalled()
	}
}
