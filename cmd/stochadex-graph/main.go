// Command stochadex-graph renders the partition dependency graph of a
// simulation from its YAML configuration, and reports any within-step
// params_from_upstream cycle that would deadlock the run.
//
// Usage:
//
//	stochadex-graph --config cfg/example_inference_config.yaml            # Mermaid
//	stochadex-graph --config cfg/example_config.yaml --format dot | dot -Tsvg > graph.svg
//
// It reads only the partition wiring (params_from_upstream and
// params_as_partitions), so the iteration expressions in the config need not be
// resolvable. It exits non-zero if a deadlock cycle is found, which makes it
// usable as a CI guard.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/graph"
)

func main() {
	configFile := flag.String("config", "", "path to a simulation config YAML (required)")
	format := flag.String("format", "mermaid", "output format: mermaid | dot")
	flag.Parse()

	if *configFile == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		flag.Usage()
		os.Exit(2)
	}

	// Build the graph from the main run's wiring. GetConfigGenerator on the
	// concrete RunConfig preserves params_as_partitions and params_from_upstream
	// without needing the iteration implementations.
	gen := api.LoadApiRunConfigFromYaml(*configFile).Main.GetConfigGenerator()
	g := graph.Build(gen)

	switch *format {
	case "mermaid":
		fmt.Print(g.Mermaid())
	case "dot":
		fmt.Print(g.DOT())
	default:
		fmt.Fprintf(os.Stderr, "error: unknown format %q (want mermaid or dot)\n", *format)
		os.Exit(2)
	}

	cycles := g.InjectCycles()
	if len(cycles) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "\nDEADLOCK: within-step params_from_upstream cycle(s) detected:")
	for _, comp := range cycles {
		names := make([]string, len(comp))
		for i, v := range comp {
			names[i] = g.Names[v]
		}
		fmt.Fprintf(os.Stderr, "  - {%s}\n", strings.Join(names, ", "))
	}
	os.Exit(1)
}
