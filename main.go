package main

import (
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "bitrise-connect-vscode",
		Usage: "Instantly connect to a running Bitrise CI build and debug it with VSCode",
		Action: func(ctx *cli.Context) error {
			if ctx.Args().Len() == 0 {
				return cli.ShowAppHelp(ctx)
			}

			sshSnippet := ctx.Args().Get(0)
			sshConfigEntry, err := parseBitriseSSHSnippet(sshSnippet)
			if err != nil {
				return fmt.Errorf("parse SSH snippet: %w", err)
			}

			ensureSSHConfig(sshConfigEntry)

			var folder = os.Getenv("BITRISE_SOURCE_DIR")
			if folder == "" {
				return fmt.Errorf("BITRISE_SOURCE_DIR environment variable is not set")
			}
			openWindow(bitriseHostPattern, folder)

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
