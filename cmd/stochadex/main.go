// Command stochadex is the CLI entry point for the simulation engine. It reads a
// YAML run configuration — a single data document naming the framework's own
// components by type (e.g. iteration: {type: wiener_process}) and any bespoke maths
// as expressions: — resolves it, and runs it in-process. No code generation, no Go
// toolchain: the whole config is data.
//
// Usage:
//
//	stochadex --config cfg/example_config.yaml
//	stochadex --config cfg/socket.yaml --socket cfg/socket_server.yaml
//
// The heavy lifting lives in pkg/api: ArgParse parses the flags, and
// RunWithParsedArgs loads and runs the config. See pkg/api for the config schema.
package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
)

func main() {
	api.RunWithParsedArgs(api.ArgParse())
}
