// Command model-graphs regenerates the "Partition wiring" Mermaid diagram in
// every models/<domain>/card.md from the domain's BuildStub wiring.
//
// The diagram is derived statically by pkg/graph, so it always matches the
// stub's actual partition wiring. Re-run after changing any stub's wiring:
//
//	go run ./cmd/model-graphs
//
// It rewrites only the region between the generated-block markers (inserting it
// just above the "## Ingests" heading on first run), so hand-written card prose
// is never touched. TestCardsUpToDate guards that the committed cards match, so
// CI fails if a stub's wiring changes without the diagrams being regenerated.
package main

//go:generate go run .

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/graph"
	"github.com/umbralcalc/stochadex/pkg/simulator"

	anglersim "github.com/umbralcalc/stochadex/models/anglersim"
	amr "github.com/umbralcalc/stochadex/models/antimicrobial-resistance"
	bathingwater "github.com/umbralcalc/stochadex/models/bathing-water-forecaster"
	bizsurvival "github.com/umbralcalc/stochadex/models/business-survival"
	energybalancer "github.com/umbralcalc/stochadex/models/energy-balancer"
	floodrisk "github.com/umbralcalc/stochadex/models/floodrisk"
	homark "github.com/umbralcalc/stochadex/models/homark"
	measles "github.com/umbralcalc/stochadex/models/measles-risk-forecaster"
	rugby "github.com/umbralcalc/stochadex/models/trywizard"
)

// model pairs a catalogue directory with its stub, built at a representative
// driver value. The wiring graph is independent of the numeric driver, so any
// in-range value yields the same diagram.
type model struct {
	dir string
	gen *simulator.ConfigGenerator
}

func models() []model {
	return []model{
		{"anglersim", anglersim.BuildStub(anglersim.DefaultWarmingTrend, 60, 42)},
		{"antimicrobial-resistance", amr.BuildStub(amr.BaselinePrescribingRate, 20)},
		{"bathing-water-forecaster", bathingwater.BuildStub(bathingwater.DefaultAnomalyVolatility, 60, 42)},
		{"business-survival", bizsurvival.BuildStub(bizsurvival.DefaultPolicyHazardScale, 24, 7001)},
		{"energy-balancer", energybalancer.BuildStub(0.5, 60, 42)},
		{"floodrisk", floodrisk.BuildStub(1.0, 60, 42)},
		{"homark", homark.BuildStub(homark.DefaultApprovalRate, 48, 42)},
		{"measles-risk-forecaster", measles.BuildStub(measles.DefaultMMR2Coverage, measles.DefaultMaxGenerations, 42)},
		{"trywizard", rugby.BuildStub(rugby.DefaultHomeSubMinute, rugby.DefaultNumSteps, 7001)},
	}
}

const (
	beginMarker  = "<!-- BEGIN generated: partition-wiring (regenerate with `go run ./cmd/model-graphs`) -->"
	endMarker    = "<!-- END generated: partition-wiring -->"
	insertBefore = "\n## Ingests"
)

const intro = `## Partition wiring

The partition dependency graph, derived statically from the stub's ` + "`BuildStub`" + ` wiring
by [` + "`pkg/graph`" + `](../../pkg/graph). Solid arrows are within-step ` + "`params_from_upstream`" + `
wiring (which imposes a computation order); dashed arrows leaving a shaded past-copy
node are lag reads of a partition's committed state from an earlier step — drawn as
separate source nodes so the graph stays a DAG.`

func block(mermaid string) string {
	var b strings.Builder
	b.WriteString(beginMarker)
	b.WriteString("\n\n")
	b.WriteString(intro)
	b.WriteString("\n\n```mermaid\n")
	b.WriteString(strings.TrimRight(mermaid, "\n"))
	b.WriteString("\n```\n\n")
	b.WriteString(endMarker)
	return b.String()
}

// splice inserts or replaces the generated block in a card's content.
func splice(content, generated string) (string, error) {
	if b := strings.Index(content, beginMarker); b >= 0 {
		e := strings.Index(content, endMarker)
		if e < 0 {
			return "", fmt.Errorf("found begin marker without end marker")
		}
		return content[:b] + generated + content[e+len(endMarker):], nil
	}
	if i := strings.Index(content, insertBefore); i >= 0 {
		return content[:i] + "\n\n" + generated + "\n" + content[i:], nil
	}
	// No "## Ingests" heading: append to the end as a fallback.
	return strings.TrimRight(content, "\n") + "\n\n" + generated + "\n", nil
}

// cardPath returns the absolute path of a model's card under the repo root.
func cardPath(root, dir string) string {
	return filepath.Join(root, "models", dir, "card.md")
}

// desiredCard returns the card content a model should have: the on-disk content
// with its generated wiring block inserted or refreshed.
func desiredCard(root string, m model) (current, desired string, err error) {
	content, err := os.ReadFile(cardPath(root, m.dir))
	if err != nil {
		return "", "", err
	}
	desired, err = splice(string(content), block(graph.Build(m.gen).Mermaid()))
	if err != nil {
		return "", "", err
	}
	return string(content), desired, nil
}

// run regenerates every card's wiring block under root. When write is false it
// only reports which cards are stale (used by TestCardsUpToDate); when true it
// rewrites them in place. It returns the directories whose cards changed.
func run(root string, write bool, logf func(string, ...any)) ([]string, error) {
	var changed []string
	for _, m := range models() {
		current, desired, err := desiredCard(root, m)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.dir, err)
		}
		if desired == current {
			logf("  unchanged  %s\n", m.dir)
			continue
		}
		changed = append(changed, m.dir)
		if !write {
			logf("  STALE      %s\n", m.dir)
			continue
		}
		if err := os.WriteFile(cardPath(root, m.dir), []byte(desired), 0o644); err != nil {
			return nil, fmt.Errorf("%s: %w", m.dir, err)
		}
		logf("  wrote      %s\n", m.dir)
	}
	return changed, nil
}

// repoRoot walks up from this source file to the module root (the directory
// holding go.mod), so the tool works whether invoked by `go run`, `go generate`,
// or `go test` regardless of the working directory.
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot determine caller source path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", file)
		}
		dir = parent
	}
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if _, err := run(root, true, func(f string, a ...any) { fmt.Printf(f, a...) }); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
