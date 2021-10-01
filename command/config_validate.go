package command

import (
	"flag"
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/command/agent"
)

type ConfigValidateCommand struct {
	Meta
}

func (c *ConfigValidateCommand) Help() string {
	helpText := `
Usage: nomad config <subcommand> [options] [args]

  Performs a thorough sanity test on Nomad configuration files. For each file
  or directory given, the validate command will attempt to parse the contents
  just as the "nomad agent" command would, and catch any errors.

  This is useful to do a test of the configuration only, without actually
  starting the agent. This performs all of the validation the agent would, so
  this should be given the complete set of configuration files that are going
  to be loaded by the agent. This command cannot operate on partial
  configuration fragments since those won't pass the full agent validation.

  Returns 0 if the configuration is valid, or 1 if there are problems.

Command Options

  -quiet
     When given, a successful run will produce no output.
`

	return strings.TrimSpace(helpText)
}

func (c *ConfigValidateCommand) Synopsis() string {
	return "Validate Nomad agent configuration files"
}

func (c *ConfigValidateCommand) Name() string { return "config validate" }

func (c *ConfigValidateCommand) Run(args []string) int {

	var quiet bool

	flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
	flags.Usage = func() { c.Ui.Error(c.Help()) }
	flags.BoolVar(&quiet, "quiet", false, "")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	configFiles := flags.Args()
	if len(configFiles) < 1 {
		c.Ui.Error("Must specify at least one config file or directory")
		return 1
	}

	// Use multierror, so we can collect all errors and warnings and output
	// these to the caller. This means a single validate command can find all
	// problems, rather than needing multiple runs to uncover all.
	var mErr, mWarn *multierror.Error

	for _, path := range configFiles {

		// Load the configuration, adding any error to our multierror.
		current, err := agent.LoadConfig(path)
		if err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("Error loading configuration from %s: %s", path, err))
			continue
		}

		// The user asked us to load some config here, but we didn't find any,
		// so we'll add a warning.
		if current == nil || reflect.DeepEqual(current, &agent.Config{}) {
			mWarn = multierror.Append(mWarn, fmt.Errorf("No configuration loaded from %s", path))
			continue
		}
	}

	// If we have any errors, print these out.
	if mWarn != nil && mWarn.Len() > 0 {
		c.Ui.Output("Validation Warnings:")
		for _, err := range mWarn.Errors {
			c.Ui.Warn(fmt.Sprintf("\t - %v", err))
		}
		c.Ui.Output("")
	}

	// If we have any errors, print these out and exit.
	if mErr != nil && mErr.Len() > 0 {
		c.Ui.Output("Validation Errors:")
		for _, err := range mErr.Errors {
			c.Ui.Error(fmt.Sprintf("\t - %v", err))
		}
		return 1
	}

	if !quiet {
		c.Ui.Output("Configuration is valid!")
	}
	return 0
}
