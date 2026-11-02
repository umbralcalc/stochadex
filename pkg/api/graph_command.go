package api

import (
	"fmt"
	"io"

	"github.com/umbralcalc/stochadex/pkg/graph"
)

// printGraph writes the main run's partition dependency graph to w, in format
// (mermaid or dot), for stochadex graph. It reads only the wiring
// (params_from_upstream and params_as_partitions), and returns an ErrConfig
// error naming each within-step deadlock cycle after the graph is written, so
// the command exits non-zero on one and can guard CI. A macros: config's graph
// is its expansion's, the run it runs as, which reads its inputs' shapes.
func printGraph(config *ApiRunConfig, format string, w io.Writer) error {
	if len(config.Macros) > 0 {
		expanded, err := expandToRun(config)
		if err != nil {
			return err
		}
		config = expanded
	}
	generator := config.Main.GetConfigGenerator()
	g := graph.Build(generator)
	var rendered string
	switch format {
	case "", "mermaid":
		rendered = g.Mermaid()
	case "dot":
		rendered = g.DOT()
	default:
		return &Error{Kind: ErrUsage,
			Err: fmt.Errorf("unknown graph format %q (want mermaid or dot)", format)}
	}
	if _, err := io.WriteString(w, rendered); err != nil {
		return err
	}
	return configError(CheckForDeadlock(generator))
}
