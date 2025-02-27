package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/bitrise-io/bitrise-remote-access-cli/ide"
	"github.com/bitrise-io/bitrise-remote-access-cli/ssh"
	"github.com/bitrise-io/bitrise-remote-access-cli/vscode"
	"github.com/urfave/cli/v3"
)

const (
	cliName        = "remote-access"
	autoCommand    = "auto"
	sshSnippetFlag = "ssh"
	passwordFlag   = "password"
)

var supportedIDEs = []ide.IDE{
	vscode.IdeData}

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    sshSnippetFlag,
		Usage:   "SSH Snippet to connect to the remote server",
		Aliases: []string{"s"},
	},
	&cli.StringFlag{
		Name:    passwordFlag,
		Usage:   "Password for SSH connection",
		Aliases: []string{"p"},
	},
}

func main() {
	commands := []*cli.Command{
		{
			Name:            autoCommand,
			Usage:           "Automatically detect the IDE and open the project",
			UsageText:       usageTextForCommand(autoCommand),
			Action:          entry,
			Description:     "You need to add SSH arguments and password to connect to the remote server",
			Flags:           flags,
			SkipFlagParsing: true,
		}}

	for _, ide := range supportedIDEs {
		commands = append(commands, &cli.Command{
			Name:            ide.Identifier,
			Usage:           fmt.Sprintf("Debug the build with %s", ide.Name),
			UsageText:       usageTextForCommand(ide.Identifier),
			Action:          entry,
			Aliases:         ide.Aliases,
			Description:     "You need to add SSH arguments to connect to the remote server",
			Flags:           flags,
			SkipFlagParsing: true,
		})
	}

	app := &cli.Command{
		Name:     cliName,
		Usage:    "Instantly connect to a running Bitrise CI build and debug it with an IDE",
		Commands: commands,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func entry(ctx context.Context, cliCmd *cli.Command) error {
	command := cliCmd.Name
	args := cliCmd.Args().Slice()
	if len(args) == 0 {
		return cli.ShowSubcommandHelp(cliCmd)
	}

	var ide ide.IDE

	if command == autoCommand {
		autoIDE, err := autoChooseIDE()
		if err != nil {
			return err
		}
		ide = autoIDE
	} else {
		for _, supportedIDE := range supportedIDEs {
			if command == supportedIDE.Identifier {
				ide = supportedIDE
			}
		}
	}
	if ide.Identifier == "" {
		return fmt.Errorf("unknown command: %s", command)
	}

	parsedArgs := parseArgs(args, flags)

	sshSnippet := parsedArgs[sshSnippetFlag]
	password := parsedArgs[passwordFlag]

	if sshSnippet == "" || password == "" {
		log.Println("SSH snippet and password are required, see the usage for more details")
		return cli.ShowSubcommandHelp(cliCmd)
	}

	return openWithIDE(sshSnippet, password, &ide)
}

func usageTextForCommand(command string) string {
	return fmt.Sprintf("%s %s --%s \"<SSH_SNIPPET>\" --%s <PASSWORD>", cliName, command, sshSnippetFlag, passwordFlag)
}

// built in flag parsing cannot ignore unknown flags AND set the required ones
// at the same time, so we need to parse the args manually
func parseArgs(args []string, flags []cli.Flag) map[string]string {
	parsed := make(map[string]string)
	validFlags := make(map[string]bool)
	flagAliases := make(map[string]string)

	for _, flag := range flags {
		switch f := flag.(type) {
		case *cli.StringFlag:
			validFlags[f.Name] = true
			for _, alias := range f.Aliases {
				validFlags[alias] = true
				flagAliases[alias] = f.Name
			}
		}
	}

	ignoredFlags := []string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			key := strings.TrimLeft(arg, "-")
			if alias, exists := flagAliases[key]; exists {
				key = alias
			}
			if validFlags[key] {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") && !strings.HasPrefix(args[i+1], "-") {
					parsed[key] = args[i+1]
					i++ // next will be value
				}
			} else {
				ignoredFlags = append(ignoredFlags, key)
			}
		}
	}

	if len(ignoredFlags) > 0 {
		log.Printf("Ignored unknown flags: %v\n", ignoredFlags)
	}

	return parsed
}

func autoChooseIDE() (ide.IDE, error) {
	termProgram := os.Getenv("TERM_PROGRAM")

	if termProgram != "" {
		for _, ide := range supportedIDEs {
			if termProgram == ide.Identifier {
				log.Printf("%s IDE detected automatically\n", ide.Name)
				return ide, nil
			}
		}
	}

	for _, ide := range supportedIDEs {
		_, installed := ide.OnTestPath()
		if installed {
			log.Printf("%s IDE found in PATH\n", ide.Name)
			return ide, nil
		}
	}

	return ide.IDE{}, fmt.Errorf("IDE could not be detected automatically, please specify the IDE explicitly instead of using the '%s' subcommand", autoCommand)
}

func setupSSH(sshSnippet, password string) (string, error) {
	sshConfigEntry, err := ssh.ParseBitriseSSHSnippet(sshSnippet, password)
	if err != nil {
		return "", fmt.Errorf("parse SSH snippet: %w", err)
	}

	isMacOs, folder, err := ssh.SetupRemote(sshConfigEntry)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return "", fmt.Errorf("dial remote host: please check the SSH arguments and make sure the remote host is reachable")
		}
		log.Print(err)
	}

	if err := ssh.EnsureSSHConfig(sshConfigEntry, isMacOs); err != nil {
		return "", err
	} else {
		log.Println("Bitrise SSH config inclusion ensured")
	}

	return folder, nil
}

func openWithIDE(sshSnippet, sshPassword string, ide *ide.IDE) error {
	folder, err := setupSSH(sshSnippet, sshPassword)
	if err != nil {
		return err
	}

	if folder == "" {
		fmt.Print("Source code location is unknown.\nWould you like to use the root directory and proceed? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		clearLines(3)

		if strings.TrimSpace(response) == "y" {
			log.Println("Using root directory")
		} else {
			log.Println("Ending session...")
			return fmt.Errorf("source code location could not be determined")
		}
	}

	return ide.OnOpen(ssh.BitriseHostPattern, folder)
}

func clearLines(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[1A")
		fmt.Print("\033[2K")
	}
}
