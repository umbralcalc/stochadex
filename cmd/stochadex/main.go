// Command stochadex is the CLI entry point for the simulation engine. It reads a
// YAML run configuration, hydrates it into a temporary Go program, and executes
// that program with `go run`. The indirection is what lets iteration types be
// named as Go expressions in the config (e.g. "&continuous.WienerProcessIteration{}")
// and wired dynamically at run time, without recompiling the binary.
//
// Usage:
//
//	stochadex --config cfg/example_config.yaml
//	stochadex --config cfg/socket.yaml --socket cfg/socket_server.yaml
//
// The heavy lifting lives in pkg/api: ArgParse parses the flags and loads the
// templated config (ApiRunConfigStrings), and RunWithParsedArgs generates and
// runs the temporary main. See pkg/api for the config schema.
package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
)

func main() {
	api.RunWithParsedArgs(api.ArgParse())
}
