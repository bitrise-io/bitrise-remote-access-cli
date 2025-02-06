package vscode

import (
	"fmt"
	"os/exec"
	"strings"
)

func OpenWindowVSCode(hostPattern, folderPath string) error {
	codePath, err := exec.LookPath("code")
	if err != nil {
		return fmt.Errorf("VSCode CLI not found in $PATH")
	}

	// TODO: implement `--goto` flag
	cmd := exec.Command(codePath, fmt.Sprintf("--folder-uri=vscode-remote://ssh-remote+%s/%s", hostPattern, folderPath))

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("open VSCode window: %w", err)
	}

	return nil
}

func isVSCodeInstalled() bool {
	_, err := exec.LookPath("code")
	return err == nil
}

func isSSHExtensionInstalled() bool {
	cmd := exec.Command("code", "--list-extensions")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(out), "ms-vscode-remote.remote-ssh")
}
