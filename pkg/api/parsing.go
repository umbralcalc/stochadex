package api

import (
	"fmt"
	"os"

	"github.com/akamensky/argparse"
)

// ParsedArgs contains the information that needs to pass from the CLI to
// the runner in order to run the API.
type ParsedArgs struct {
	ConfigStrings *ApiRunConfigStrings
	ConfigFile    string
	SocketFile    string
}

// ArgParse builds the configs parsed as args to the stochadex binary and
// also retrieves other args.
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
