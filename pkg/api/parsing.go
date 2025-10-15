package api

import (
	"fmt"
	"os"

	"github.com/akamensky/argparse"
)

// ParsedArgs bundles CLI-derived inputs for running the API.
// Includes the YAML config path, optional socket config path, and the
// string-templated configuration used to generate a runnable main.
type ParsedArgs struct {
	ConfigStrings *ApiRunConfigStrings
	ConfigFile    string
	SocketFile    string
}

// ArgParse parses CLI flags into a ParsedArgs, loading ApiRunConfigStrings
// from the provided YAML path for template hydration.
func ArgParse() ParsedArgs {
	fmt.Println("\nReading in args ...")
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
	socketFile := parser.String(
		"s",
		"socket",
		&argparse.Options{
			Required: false,
			Help:     "yaml config path for socket",
		},
	)
	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}
	return ParsedArgs{
		ConfigStrings: LoadApiRunConfigStringsFromYaml(*configFile),
		ConfigFile:    *configFile,
		SocketFile:    *socketFile,
	}
}
