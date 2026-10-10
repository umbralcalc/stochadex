package api

import (
	"errors"
	"fmt"
	"os"

	"github.com/akamensky/argparse"
)

// ParsedArgs bundles CLI-derived inputs for running the API: the YAML config
// path, any --set overrides (each path=value), and an optional socket config
// path.
type ParsedArgs struct {
	ConfigFile string
	Sets       []string
	SocketFile string
	// Check validates the config and exits without running it (--check).
	Check bool
	// InspectIO prints the config's I/O manifest instead of running it
	// (stochadex inspect --io).
	InspectIO bool
}

// ArgParse parses CLI flags into a ParsedArgs.
func ArgParse() ParsedArgs {
	fmt.Println("\nReading in args ...")
	parsed, err := parseArgs(os.Args)
	if err != nil {
		// Carrying on with an empty config path only fails later and less clearly.
		fmt.Print(err)
		os.Exit(ExitUsage)
	}
	return parsed
}

// parseArgs parses CLI arguments (os.Args form), returning an ErrUsage error
// carrying the usage text when they are invalid. `stochadex inspect ...` is told
// apart before parsing: argparse would require a subcommand on every run once
// one is declared.
func parseArgs(args []string) (ParsedArgs, error) {
	if len(args) > 1 && args[1] == "inspect" {
		return parseInspectArgs(append([]string{args[0] + " inspect"}, args[2:]...))
	}
	parser := argparse.NewParser(
		"stochadex",
		"A generalised simulation engine",
	)
	configFile, sets := configArgs(parser)
	socketFile := parser.String(
		"s",
		"socket",
		&argparse.Options{
			Required: false,
			Help:     "deprecated: yaml socket config path; use run: {mode: serve} in the config",
		},
	)
	check := parser.Flag(
		"",
		"check",
		&argparse.Options{
			Help: "validate the config, including its wiring, without running it or " +
				"reading its inputs; exits 0 when it is valid",
		},
	)
	if err := parser.Parse(args); err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{
		ConfigFile: *configFile,
		Sets:       *sets,
		SocketFile: *socketFile,
		Check:      *check,
	}, nil
}

// parseInspectArgs parses `stochadex inspect`'s arguments.
func parseInspectArgs(args []string) (ParsedArgs, error) {
	parser := argparse.NewParser(
		"stochadex inspect",
		"Describe a config without running it",
	)
	configFile, sets := configArgs(parser)
	io := parser.Flag(
		"",
		"io",
		&argparse.Options{
			Required: true,
			Help: "print, as JSON, what a run reads and writes: inputs, outputs, run " +
				"mode, seeds, clock and overrides",
		},
	)
	if err := parser.Parse(args); err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{ConfigFile: *configFile, Sets: *sets, InspectIO: *io}, nil
}

// configArgs declares the arguments every command takes: the config and its
// overrides.
func configArgs(parser *argparse.Parser) (configFile *string, sets *[]string) {
	configFile = parser.String(
		"c",
		"config",
		&argparse.Options{
			Required: true,
			Help:     "yaml config path",
		},
	)
	sets = parser.StringList(
		"",
		"set",
		&argparse.Options{
			Required: false,
			Help: "override a config value for this run, as path=value (repeatable); " +
				"e.g. main.partitions[name=w].seed=7",
		},
	)
	return configFile, sets
}
