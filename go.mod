module github.com/bitrise-io/bitrise-remote-access-cli

go 1.21.3

require (
	github.com/kevinburke/ssh_config v1.2.0 // https://github.com/kevinburke/ssh_config/issues/50
	github.com/pkg/sftp v1.13.7
	golang.org/x/crypto v0.33.0
)

require github.com/urfave/cli/v3 v3.0.0-beta1

require (
	github.com/kr/fs v0.1.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
)
