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

			var folder string
			if sshConfigEntry.User == "vagrant" {
				folder = "/Users/vagrant/"
			} else if sshConfigEntry.User == "ubuntu"{
				folder = "/bitrise/src/"
			} else {
				folder = "" // Open root
			}
			openWindow(bitriseHostPattern, folder)

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
