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
// carrying the usage text when they are invalid.
func parseArgs(args []string) (ParsedArgs, error) {
	parser := argparse.NewParser(
		"stochadex",
		"A generalised simulation engine",
	)
	configFile := parser.String(
		"c",
		"config",
		&argparse.Options{
			Required: true,
			Help:     "yaml config path",
		},
	)
	sets := parser.StringList(
		"",
		"set",
		&argparse.Options{
			Required: false,
			Help: "override a config value for this run, as path=value (repeatable); " +
				"e.g. main.partitions[name=w].seed=7",
		},
	)
	socketFile := parser.String(
		"s",
		"socket",
		&argparse.Options{
			Required: false,
			Help:     "deprecated: yaml socket config path; use run: {mode: serve} in the config",
		},
	)
	if err := parser.Parse(args); err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{
		ConfigFile: *configFile,
		Sets:       *sets,
		SocketFile: *socketFile,
	}, nil
}
