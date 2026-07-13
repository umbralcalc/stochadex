// Command model-graphs regenerates the machine-derived blocks in every
// models/<domain>/card.md:
//
//   - the "Partition wiring" Mermaid diagram, from the domain's BuildStub wiring
//     (via pkg/graph, so it always matches the stub's actual partition wiring); and
//   - the "Observed behaviour" table, for models whose behaviour suite exposes an
//     ObservedBehaviour() — the ensemble numbers emitted by the model's own tests,
//     so a card's numbers are generated from the code, never hand-typed.
//
// Re-run after changing any stub's wiring or generative behaviour:
//
//	go run ./cmd/model-graphs
//
// It rewrites only the regions between the generated-block markers (inserting the
// wiring block above "## Ingests" and the observed-behaviour block above
// "## Bespoke extensions" on first run), so hand-written card prose is never
// touched. TestCardsUpToDate guards that the committed cards match, so CI fails if
// a stub's wiring or numbers change without the cards being regenerated.
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

	"github.com/umbralcalc/stochadex/models/cardgen"

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
// in-range value yields the same diagram. obs, when non-nil, is the model's
// verified response claims with their observed numbers — rendered into the card's
// "Observed behaviour" block. Only models whose behaviour suite exposes an
// ObservedBehaviour() supply it (anglersim leads; others follow as generalised).
type model struct {
	dir     string
	gen     *simulator.ConfigGenerator
	obs     []cardgen.Claim
	binding cardgen.Binding
}

func models() []model {
	return []model{
		{
			dir:     "anglersim",
			gen:     anglersim.BuildStub(anglersim.DefaultWarmingTrend, 60, 42),
			obs:     anglersim.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestAnglersimExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "antimicrobial-resistance",
			gen:     amr.BuildStub(amr.BaselinePrescribingRate, 20),
			obs:     amr.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestAMRExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "bathing-water-forecaster",
			gen:     bathingwater.BuildStub(bathingwater.DefaultAnomalyVolatility, 60, 42),
			obs:     bathingwater.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestBathingWaterExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "business-survival",
			gen:     bizsurvival.BuildStub(bizsurvival.DefaultPolicyHazardScale, 24, 7001),
			obs:     bizsurvival.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestBusinessSurvivalExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "energy-balancer",
			gen:     energybalancer.BuildStub(0.5, 60, 42),
			obs:     energybalancer.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestEnergyBalancerExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "floodrisk",
			gen:     floodrisk.BuildStub(1.0, 60, 42),
			obs:     floodrisk.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestFloodRiskExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "homark",
			gen:     homark.BuildStub(homark.DefaultApprovalRate, 48, 42),
			obs:     homark.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestHomarkExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "measles-risk-forecaster",
			gen:     measles.BuildStub(measles.DefaultMMR2Coverage, measles.DefaultMaxGenerations, 42),
			obs:     measles.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestMeaslesExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
		{
			dir:     "trywizard",
			gen:     rugby.BuildStub(rugby.DefaultHomeSubMinute, rugby.DefaultNumSteps, 7001),
			obs:     rugby.ObservedBehaviour(),
			binding: cardgen.Binding{TestName: "TestRugbyExpectedBehaviour", TestFile: "behaviour_test.go"},
		},
	}
}

const (
	beginMarker  = "<!-- BEGIN generated: partition-wiring (regenerate with `go run ./cmd/model-graphs`) -->"
	endMarker    = "<!-- END generated: partition-wiring -->"
	insertBefore = "\n## Ingests"

	obsBeginMarker  = "<!-- BEGIN generated: observed-behaviour (regenerate with `go run ./cmd/model-graphs`) -->"
	obsEndMarker    = "<!-- END generated: observed-behaviour -->"
	obsInsertBefore = "\n## Bespoke extensions"

	regenCmd = "go run ./cmd/model-graphs"
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

// obsBlock renders the marker-wrapped "Observed behaviour" block from a model's
// claims. Returns "" when the model has no claims (so no block is spliced).
func obsBlock(claims []cardgen.Claim, binding cardgen.Binding) string {
	body := cardgen.ObservedBehaviourMarkdown(claims, binding, regenCmd)
	if body == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(obsBeginMarker)
	b.WriteString("\n\n")
	b.WriteString(body)
	b.WriteString("\n\n")
	b.WriteString(obsEndMarker)
	return b.String()
}

// splice inserts or replaces a marker-delimited generated block in a card's
// content. If the begin/end markers are present it replaces between them; else it
// inserts before the insertBefore heading; else it appends at the end.
func splice(content, generated, begin, end, insertBefore string) (string, error) {
	if b := strings.Index(content, begin); b >= 0 {
		e := strings.Index(content, end)
		if e < 0 {
			return "", fmt.Errorf("found begin marker %q without its end marker", begin)
		}
		return content[:b] + generated + content[e+len(end):], nil
	}
	if i := strings.Index(content, insertBefore); i >= 0 {
		return content[:i] + "\n\n" + generated + "\n" + content[i:], nil
	}
	return strings.TrimRight(content, "\n") + "\n\n" + generated + "\n", nil
}

// cardPath returns the absolute path of a model's card under the repo root.
func cardPath(root, dir string) string {
	return filepath.Join(root, "models", dir, "card.md")
}

// desiredCard returns the card content a model should have: the on-disk content
// with its generated wiring block — and, if the model exposes response claims, its
// "Observed behaviour" block — inserted or refreshed.
func desiredCard(root string, m model) (current, desired string, err error) {
	content, err := os.ReadFile(cardPath(root, m.dir))
	if err != nil {
		return "", "", err
	}
	out, err := splice(string(content), block(graph.Build(m.gen).Mermaid()),
		beginMarker, endMarker, insertBefore)
	if err != nil {
		return "", "", err
	}
	if ob := obsBlock(m.obs, m.binding); ob != "" {
		out, err = splice(out, ob, obsBeginMarker, obsEndMarker, obsInsertBefore)
		if err != nil {
			return "", "", err
		}
	}
	return string(content), out, nil
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
