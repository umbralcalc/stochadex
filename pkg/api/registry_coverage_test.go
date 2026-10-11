package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// excludedIterations names every Iteration implementation in the candidate
// packages that is deliberately NOT in the registry, each with the reason. Only
// MAST, which nothing wires yet, is left. The last live-object exclusions were
// bound at run time — a batch *StateHistory an embedded run's host feeds in
// (from_history, data_comparison_gradient), and a whole
// *Settings/*Implementations pair (embedded_simulation_run, whose run: is now
// stated as data, to any depth). validateMemoryFeeds checks at load that
// something feeds the former. (Bulk [][]float64 data was never a reason:
// from_storage carries its rows inline as config data.)
//
// Drift test 2 asserts every Iterate-implementing type in the candidate packages
// is either registered (wantIterationType) or listed here. A NEW iteration then
// fails CI until it is classified — the guard that stops the registry silently
// lagging the framework.
//
// A field of live-object *type* is not on its own grounds for exclusion: an RNG
// source that Configure assigns from the partition seed is inert as config, and
// nested iterations are expressible as recursive specs.
var excludedIterations = map[string]string{
	// MAST is an optional rollout strategy that no macro wires yet; its data
	// spec waits for a config that needs it (it would take the environment
	// like mcts_rollout, plus its aggregation partition).
	"MASTRolloutIteration":     "unwired: optional MAST rollout strategy, no macro or config uses it yet",
	"MASTAggregationIteration": "unwired: MAST's per-action statistics, used only with MASTRolloutIteration",
}

// candidatePackages are the packages whose Iteration implementations are
// candidates for the data-only registry. pkg/agents is among them: its MCTS
// iterations have data specs over a {type: ...} environment.
var candidatePackages = []string{
	"../continuous", "../discrete", "../general", "../inference", "../agents",
}

// TestIterationCoverage is drift test 2: it scans the candidate packages for every
// type with an Iterate method and asserts each is either registered or excluded
// with a reason, so a newly-added iteration cannot silently escape classification.
func TestIterationCoverage(t *testing.T) {
	registered := make(map[string]bool)
	for _, goType := range wantIterationType {
		// goType is like "*continuous.WienerProcessIteration", or
		// "*agents.MCTSTreeIteration[[]float64,int]"; take the bare name.
		goType, _, _ = strings.Cut(goType, "[")
		registered[goType[strings.LastIndex(goType, ".")+1:]] = true
	}

	found := iterationTypesInPackages(t, candidatePackages)
	if len(found) == 0 {
		t.Fatal("found no Iterate-implementing types; the scan is broken")
	}

	for _, typeName := range found {
		_, isRegistered := registered[typeName]
		_, isExcluded := excludedIterations[typeName]
		switch {
		case isRegistered && isExcluded:
			t.Errorf("%s is both registered and excluded — pick one", typeName)
		case !isRegistered && !isExcluded:
			t.Errorf(
				"%s implements Iterate but is neither registered nor excluded: "+
					"add it to the registry (data-only) or to excludedIterations "+
					"with a reason (composable / live-object)",
				typeName,
			)
		}
	}

	// Also guard the reverse: an excluded name that no longer exists is stale.
	for typeName := range excludedIterations {
		present := false
		for _, found := range found {
			if found == typeName {
				present = true
				break
			}
		}
		if !present {
			t.Errorf("excludedIterations lists %s, which no longer implements Iterate", typeName)
		}
	}
}

// iterationTypesInPackages parses the given package directories and returns the
// exported type names that have a method named Iterate with a pointer receiver.
func iterationTypesInPackages(t *testing.T, dirs []string) []string {
	t.Helper()
	var types []string
	fileSet := token.NewFileSet()
	for _, dir := range dirs {
		pkgs, err := parser.ParseDir(fileSet, dir, func(info os.FileInfo) bool {
			return !strings.HasSuffix(info.Name(), "_test.go")
		}, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", dir, err)
		}
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				for _, decl := range file.Decls {
					funcDecl, ok := decl.(*ast.FuncDecl)
					if !ok || funcDecl.Name.Name != "Iterate" || funcDecl.Recv == nil {
						continue
					}
					if name := pointerReceiverType(funcDecl.Recv); name != "" {
						types = append(types, name)
					}
				}
			}
		}
	}
	return types
}

// pointerReceiverType returns the bare type name of a *T pointer receiver, or "".
func pointerReceiverType(recv *ast.FieldList) string {
	if len(recv.List) == 0 {
		return ""
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return ""
	}
	// A generic receiver, *T[S, A], names its type through an index expression.
	base := star.X
	switch generic := base.(type) {
	case *ast.IndexExpr:
		base = generic.X
	case *ast.IndexListExpr:
		base = generic.X
	}
	ident, ok := base.(*ast.Ident)
	if !ok || !ident.IsExported() {
		return ""
	}
	return ident.Name
}
