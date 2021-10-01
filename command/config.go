package command

import (
	"strings"

	"github.com/mitchellh/cli"
)

type ConfigCommand struct {
	Meta
}

func (c *ConfigCommand) Help() string {
	helpText := `
Usage: nomad config <subcommand> [options] [args]

`

	return strings.TrimSpace(helpText)
}

func (c *ConfigCommand) Synopsis() string {
	return "Validate and manage Nomad agent configuration files"
}

func (c *ConfigCommand) Name() string { return "config" }

func (c *ConfigCommand) Run(_ []string) int { return cli.RunResultHelp }
