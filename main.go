package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/bitrise-io/bitrise-remote-access-cli/ssh"
	"github.com/bitrise-io/bitrise-remote-access-cli/vscode"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "remote-access",
		Usage: "Instantly connect to a running Bitrise CI build and debug it with an IDE",
		Commands: []*cli.Command{
			{
				Name:    vscode.IDEIdentifier,
				Usage:   fmt.Sprintf("Debug the build with %s", vscode.IDEName),
				Action:  func(ctx *cli.Context) error { return openWithIDE(ctx, vscode.IDEIdentifier) },
				Aliases: []string{"code"},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func setupSSH(ctx *cli.Context) error {
	sshSnippet := ctx.Args().Get(0)
	sshConfigEntry, err := ssh.ParseBitriseSSHSnippet(sshSnippet)
	if err != nil {
		return fmt.Errorf("parse SSH snippet: %w", err)
	}

	ssh.EnsureSSHConfig(sshConfigEntry)

	return nil
}

func openWithIDE(ctx *cli.Context, ide string) error {
	if ctx.Args().Len() == 0 {
		return cli.ShowAppHelp(ctx)
	}

	err := setupSSH(ctx)
	if err != nil {
		return err
	}

	var folder = os.Getenv("BITRISE_SOURCE_DIR")
	if folder == "" {
		fmt.Println("BITRISE_SOURCE_DIR environment variable is not set, source code location is unknown.")
		fmt.Print("Would you like to use the root directory and proceed? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if response == "y\n" {
			// Code to open the root directory and proceed
			fmt.Println("Using root directory and proceeding...")
		} else {
			fmt.Println("Ending session.")
			return fmt.Errorf("source code location could not be determined")
		}
	}

	switch ide {
	case vscode.IDEIdentifier:
		return vscode.OpenInVSCode(ssh.BitriseHostPattern, folder)
	}
	return nil
}
