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
	SeedRange string
	// DebugEmbedded writes embedded runs' inner steps to json_logs, each
	// NAME=PATH (--debug-embedded).
	DebugEmbedded []string
	SocketFile    string
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
	// Graph prints the main run's partition dependency graph instead of running
	// it, in GraphFormat (mermaid or dot), and fails on a within-step deadlock
	// cycle (stochadex graph).
	Graph       bool
	GraphFormat string
	// Catalogue prints, as JSON, what this build can run: its version, features
	// and every registered component name (stochadex catalogue). It needs no
	// config.
	Catalogue bool
	// Expand prints a macros: config written out in the core language, the
	// run it runs as (stochadex expand).
	Expand bool
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
// carrying the usage text when they are invalid. stochadex inspect ... is told
// apart before parsing: argparse would require a subcommand on every run once
// one is declared. The same holds for stochadex graph, stochadex expand and
// stochadex catalogue.
func parseArgs(args []string) (ParsedArgs, error) {
	if len(args) > 1 {
		switch args[1] {
		case "inspect":
			return parseInspectArgs(append([]string{args[0] + " inspect"}, args[2:]...))
		case "graph":
			return parseGraphArgs(append([]string{args[0] + " graph"}, args[2:]...))
		case "expand":
			return parseExpandArgs(append([]string{args[0] + " expand"}, args[2:]...))
		case "catalogue":
			return parseCatalogueArgs(append([]string{args[0] + " catalogue"}, args[2:]...))
		}
	}
	parser := argparse.NewParser(
		"stochadex",
		"A generalised simulation engine. Other commands: stochadex inspect, "+
			"stochadex graph, stochadex expand and stochadex catalogue (each takes --help)",
	)
	configFile, sets, seedRange, debugEmbedded := configArgs(parser)
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
		DebugEmbedded:   *debugEmbedded,
		SocketFile:      *socketFile,
		Check:           *check,
		Provenance:      *provenance || *skip,
		SkipIfUnchanged: *skip,
	}, nil
}

// parseInspectArgs parses stochadex inspect's arguments.
func parseInspectArgs(args []string) (ParsedArgs, error) {
	parser := argparse.NewParser(
		"stochadex inspect",
		"Describe a config without running it",
	)
	configFile, sets, seedRange, debugEmbedded := configArgs(parser)
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
		DebugEmbedded: *debugEmbedded, InspectIO: *io, InspectProvenance: *provenance}, nil
}

// parseGraphArgs parses stochadex graph's arguments.
func parseGraphArgs(args []string) (ParsedArgs, error) {
	parser := argparse.NewParser(
		"stochadex graph",
		"Print the main run's partition dependency graph without running it; "+
			"exits non-zero on a within-step deadlock cycle",
	)
	configFile, sets := configFileArgs(parser)
	format := parser.Selector(
		"",
		"format",
		[]string{"mermaid", "dot"},
		&argparse.Options{
			Default: "mermaid",
			Help:    "output format: mermaid or dot",
		},
	)
	if err := parser.Parse(args); err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{ConfigFile: *configFile, Sets: *sets, Graph: true,
		GraphFormat: *format}, nil
}

// parseExpandArgs parses stochadex expand's arguments.
func parseExpandArgs(args []string) (ParsedArgs, error) {
	parser := argparse.NewParser(
		"stochadex expand",
		"Print a macros: config written out in the core language, the run it runs "+
			"as, without running it; reads its inputs' shapes",
	)
	configFile, sets := configFileArgs(parser)
	if err := parser.Parse(args); err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{ConfigFile: *configFile, Sets: *sets, Expand: true}, nil
}

// parseCatalogueArgs parses stochadex catalogue's arguments: there are none.
func parseCatalogueArgs(args []string) (ParsedArgs, error) {
	parser := argparse.NewParser(
		"stochadex catalogue",
		"Print, as JSON, this build's version, features and registered component names",
	)
	if err := parser.Parse(args); err != nil {
		return ParsedArgs{}, &Error{Kind: ErrUsage, Err: errors.New(parser.Usage(err))}
	}
	return ParsedArgs{Catalogue: true}, nil
}

// configArgs declares the arguments every run-shaped command takes: the config,
// its overrides, a seed shard and embedded-run debugging.
func configArgs(parser *argparse.Parser) (
	configFile *string, sets *[]string, seedRange *string, debugEmbedded *[]string,
) {
	configFile, sets = configFileArgs(parser)
	seedRange = parser.String(
		"",
		"seed-range",
		&argparse.Options{
			Required: false,
			Help: "run this shard of an ensemble: the seeds FROM to TO inclusive, as " +
				"FROM:TO, in place of run.seeds",
		},
	)
	debugEmbedded = parser.StringList(
		"",
		"debug-embedded",
		&argparse.Options{
			Required: false,
			Help: "write an embedded run's inner steps, every one of every outer step, to " +
				"a json_log, as NAME=PATH (repeatable); by default an embedded run writes nothing",
		},
	)
	return configFile, sets, seedRange, debugEmbedded
}

// configFileArgs declares the arguments every command that reads a config takes:
// the config and its overrides.
func configFileArgs(parser *argparse.Parser) (configFile *string, sets *[]string) {
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
