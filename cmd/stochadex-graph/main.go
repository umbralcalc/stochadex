// Command stochadex-graph is a deprecated alias for stochadex graph, kept so
// existing scripts and CI guards keep working; use the subcommand instead:
//
//	stochadex graph -c cfg/example_inference_config.yaml            # Mermaid
//	stochadex graph -c cfg/example_config.yaml --format dot | dot -Tsvg > graph.svg
//
// It takes the same flags it always did and renders the same graph, through
// the subcommand. It exits 1 if a within-step params_from_upstream deadlock
// cycle is found (or the config cannot be loaded) and 2 on a usage error, as
// before.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/umbralcalc/stochadex/pkg/api"
)

func main() {
	configFile := flag.String("config", "", "path to a simulation config YAML (required)")
	format := flag.String("format", "mermaid", "output format: mermaid | dot")
	flag.Parse()

	fmt.Fprintln(os.Stderr, "stochadex-graph is deprecated: use `stochadex graph -c CONFIG [--format mermaid|dot]`")
	if *configFile == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		flag.Usage()
		os.Exit(2)
	}
	err := api.Execute([]string{"stochadex", "graph", "--config", *configFile, "--format", *format})
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "stochadex-graph:", err)
	if api.KindOf(err) == api.ErrUsage {
		os.Exit(2)
	}
	os.Exit(1)
}
