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
	// SeedRange runs one shard of an ensemble: FROM:TO, the seeds FROM to TO
	// inclusive, in place of run.seeds (--seed-range).
	SeedRange  string
	SocketFile string
	// Check validates the config and exits without running it (--check).
	Check bool
	// InspectIO prints the config's I/O manifest instead of running it
	// (stochadex inspect --io).
	InspectIO bool
	// InspectProvenance prints the run's provenance, with its key, instead of
	// running it (stochadex inspect --provenance).
	InspectProvenance bool
	// Provenance writes a provenance sidecar beside each file output
	// (--provenance); SkipIfUnchanged skips a run whose outputs already carry
	// its key, and implies Provenance (--skip-if-unchanged).
	Provenance      bool
	SkipIfUnchanged bool
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
	configFile, sets, seedRange := configArgs(parser)
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
	provenance := parser.Flag(
		"",
		"provenance",
		&argparse.Options{
			Help: "write <output>" + sidecarSuffix + " beside each file output: what made it, " +
				"and a key identifying the run",
		},
	)
	skip := parser.Flag(
		"",
		"skip-if-unchanged",
		&argparse.Options{
			Help: "do not run when every file output already carries this run's key; " +
				"implies --provenance",
		},
	)
	if err := parser.Parse(args); err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{
		ConfigFile:      *configFile,
		Sets:            *sets,
		SeedRange:       *seedRange,
		SocketFile:      *socketFile,
		Check:           *check,
		Provenance:      *provenance || *skip,
		SkipIfUnchanged: *skip,
	}, nil
}

// parseInspectArgs parses `stochadex inspect`'s arguments.
func parseInspectArgs(args []string) (ParsedArgs, error) {
	parser := argparse.NewParser(
		"stochadex inspect",
		"Describe a config without running it",
	)
	configFile, sets, seedRange := configArgs(parser)
	io := parser.Flag(
		"",
		"io",
		&argparse.Options{
			Help: "print, as JSON, what a run reads and writes: inputs, outputs, run " +
				"mode, seeds, clock and overrides",
		},
	)
	provenance := parser.Flag(
		"",
		"provenance",
		&argparse.Options{
			Help: "print, as JSON, what a run would be made from, with the key that " +
				"identifies it (reads its inputs to fingerprint them)",
		},
	)
	err := parser.Parse(args)
	if err == nil && *io == *provenance {
		err = errors.New("give exactly one of --io and --provenance")
	}
	if err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{ConfigFile: *configFile, Sets: *sets, SeedRange: *seedRange,
		InspectIO: *io, InspectProvenance: *provenance}, nil
}

// configArgs declares the arguments every command takes: the config and its
// overrides.
func configArgs(parser *argparse.Parser) (configFile *string, sets *[]string, seedRange *string) {
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
	seedRange = parser.String(
		"",
		"seed-range",
		&argparse.Options{
			Required: false,
			Help: "run this shard of an ensemble: the seeds FROM to TO inclusive, as " +
				"FROM:TO, in place of run.seeds",
		},
	)
	return configFile, sets, seedRange
}
