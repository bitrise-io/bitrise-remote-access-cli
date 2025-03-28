package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bitrise-io/bitrise-remote-access-cli/ide"
	"github.com/bitrise-io/bitrise-remote-access-cli/logger"
	"github.com/bitrise-io/bitrise-remote-access-cli/ssh"
	"github.com/bitrise-io/bitrise-remote-access-cli/vscode"
	"github.com/urfave/cli/v3"
)

const (
	cliName         = ":remote"
	autoCommand     = "auto"
	sshHostFlag     = "host"
	sshPortFlag     = "port"
	sshUserFlag     = "user"
	sshPasswordFlag = "password"
)

var supportedIDEs = []ide.IDE{
	vscode.IdeData}

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    sshHostFlag,
		Usage:   "SSH Hostname",
		Aliases: []string{"H"},
	},
	&cli.StringFlag{
		Name:    sshPortFlag,
		Usage:   "SSH Port number",
		Aliases: []string{"P"},
	},
	&cli.StringFlag{
		Name:    sshUserFlag,
		Usage:   "Username for SSH connection",
		Aliases: []string{"U"},
	},
	&cli.StringFlag{
		Name:    sshPasswordFlag,
		Usage:   "Password for SSH connection",
		Aliases: []string{"p"},
	},
}

func main() {
	commands := []*cli.Command{
		command(autoCommand, "Automatically detect the IDE and open the project", nil)}

	for _, ide := range supportedIDEs {
		commands = append(commands, command(ide.Identifier, fmt.Sprintf("Debug the build with %s", ide.Name), ide.Aliases))
	}

	app := &cli.Command{
		Name:     cliName,
		Usage:    "Instantly connect to a running Bitrise CI build and debug it with an IDE",
		Commands: commands,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logger.Error(err)
		os.Exit(1)
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

	var password *string
	parsedPw, parsedPwExists := parsedArgs[sshPasswordFlag]
	if parsedPwExists {
		password = &parsedPw
	}

	onLaunchIDE := func(useIdentityKey bool, folderPath string) error {
		return openWithIDE(&ide, folderPath, password, useIdentityKey)
	}

	err := ssh.SetupSSH(parsedArgs[sshHostFlag], parsedArgs[sshPortFlag], parsedArgs[sshUserFlag], password, onLaunchIDE)

	var configErr ssh.ConfigErr
	if errors.As(err, &configErr) {
		_ = cli.ShowSubcommandHelp(cliCmd)
		return err
	}

	return err
}

func command(name, usage string, aliases []string) *cli.Command {
	return &cli.Command{
		Name:            name,
		Usage:           usage,
		UsageText:       usageTextForCommand(name),
		Action:          entry,
		Aliases:         aliases,
		Description:     "You need to add SSH arguments to connect to the remote server",
		Flags:           flags,
		SkipFlagParsing: true,
	}
}

func usageTextForCommand(command string) string {
	return fmt.Sprintf("%s %s --%s <HOSTNAME> --%s <PORT> --%s <USER> --%s <PASSWORD>", cliName, command, sshHostFlag, sshPortFlag, sshUserFlag, sshPasswordFlag)
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
		logger.Warnf("Ignored unknown flags: %v", ignoredFlags)
	}

	return parsed
}

func autoChooseIDE() (ide.IDE, error) {
	termProgram := os.Getenv("TERM_PROGRAM")

	if termProgram != "" {
		for _, ide := range supportedIDEs {
			if termProgram == ide.Identifier {
				logger.Successf("%s IDE detected automatically", ide.Name)
				return ide, nil
			}
		}
	}

	for _, ide := range supportedIDEs {
		_, installed := ide.OnTestPath()
		if installed {
			logger.Successf("%s IDE found in PATH", ide.Name)
			return ide, nil
		}
	}

	return ide.IDE{}, fmt.Errorf("IDE could not be detected automatically, please specify the IDE explicitly instead of using the '%s' subcommand", autoCommand)
}

func openWithIDE(ide *ide.IDE, folder string, password *string, usingKey bool) error {
	if folder == "" {
		confirm, err := logger.Confirm(
			"Source code location is unknown.\nWould you like to use the root directory and proceed?",
			"Using root directory",
			"Not using root directory, ending session...")

		if !confirm || err != nil {
			return fmt.Errorf("source code location could not be determined")
		}
	}

	var additionalInfo string
	if !usingKey && password != nil {
		additionalInfo = fmt.Sprintf("Your password for SSH connection:\n\n%s\n\ncopy this into the password field of the opening window", *password)
	}

	return ide.OnOpen(ssh.BitriseHostPattern, folder, additionalInfo)
}
