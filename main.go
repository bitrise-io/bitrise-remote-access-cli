package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/bitrise-io/bitrise-remote-access-cli/ide"
	"github.com/bitrise-io/bitrise-remote-access-cli/ssh"
	"github.com/bitrise-io/bitrise-remote-access-cli/vscode"
	"github.com/urfave/cli/v2"
)

var supportedIDEs = []ide.IDE{
	vscode.IdeData}

func main() {
	commands := []*cli.Command{
		{
			Name:        "auto",
			Usage:       "Automatically detect the IDE and open the project",
			Action:      openWithAutoIDE,
			HelpName:    "auto command",
			Description: "You need to add SSH arguments to connect to the remote server",
		}}

	for _, ide := range supportedIDEs {
		commands = append(commands, &cli.Command{
			Name:        ide.Identifier,
			Usage:       fmt.Sprintf("Debug the build with %s", ide.Name),
			Action:      func(ctx *cli.Context) error { return openWithIDE(ctx, &ide) },
			Aliases:     ide.Aliases,
			HelpName:    fmt.Sprintf("%s command", ide.Identifier),
			Description: "You need to add SSH arguments to connect to the remote server",
		})
	}

	app := &cli.App{
		Name:     "remote-access",
		Usage:    "Instantly connect to a running Bitrise CI build and debug it with an IDE",
		Commands: commands,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func openWithAutoIDE(ctx *cli.Context) error {
	termProgram := os.Getenv("TERM_PROGRAM")

	if termProgram != "" {
		for _, ide := range supportedIDEs {
			if termProgram == ide.Identifier {
				log.Printf("%s IDE detected automatically\n", ide.Name)
				return openWithIDE(ctx, &ide)
			}
		}
	}

	for _, ide := range supportedIDEs {
		_, installed := ide.OnTestPath()
		if installed {
			log.Printf("%s IDE found in PATH\n", ide.Name)
			return openWithIDE(ctx, &ide)
		}
	}

	return fmt.Errorf("IDE could not be detected automatically, please specify the IDE explicitly instead of using the 'auto' subcommand")
}

func setupSSH(ctx *cli.Context) (string, error) {
	sshSnippet := ctx.Args().Get(0)
	sshConfigEntry, err := ssh.ParseBitriseSSHSnippet(sshSnippet, ctx.Args().Get(1))
	if err != nil {
		return "", fmt.Errorf("parse SSH snippet: %w", err)
	}

	isMacOs, folder, err := ssh.SetupRemote(sshConfigEntry)
	if err != nil {
		log.Print(err)
	}

	if err := ssh.EnsureSSHConfig(sshConfigEntry, isMacOs); err != nil {
		return "", err
	} else {
		log.Println("Bitrise SSH config inclusion ensured")
	}

	return folder, nil
}

func openWithIDE(ctx *cli.Context, ide *ide.IDE) error {
	if ctx.Args().Len() == 0 {
		return cli.ShowAppHelp(ctx)
	}

	folder, err := setupSSH(ctx)
	if err != nil {
		return err
	}

	if folder == "" {
		fmt.Print("Source code location is unknown.\nWould you like to use the root directory and proceed? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		clearLines(3)

		if response == "y\n" {
			log.Println("Using root directory")
		} else {
			log.Println("Ending session...")
			return fmt.Errorf("source code location could not be determined")
		}
	} else {
		log.Printf("Source code location: %s\n", folder)
	}

	return ide.OnOpen(ssh.BitriseHostPattern, folder)
}

func clearLines(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[1A")
		fmt.Print("\033[2K")
	}
}
