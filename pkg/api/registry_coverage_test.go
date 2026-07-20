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
// packages that is deliberately NOT in the registry (data-only in Phase A,
// composable in Phase B), each with the reason:
//   - "composable-deferred": composable in principle, but its constructor takes a
//     positional partition index that is fragile to express as raw data.
//   - "live-object": holds a live object, channel, RNG source, or bulk [][]float64
//     data with no data form at all.
//
// Drift test 2 asserts every Iterate-implementing type in the candidate packages
// is either registered (wantIterationType) or listed here. A NEW iteration then
// fails CI until it is classified — the guard that stops the registry silently
// lagging the framework.
var excludedIterations = map[string]string{
	// composable but deliberately deferred: the constructor takes a positional
	// partition index, which is fragile to express as raw data — revisit if a
	// name-resolving form lands.
	"HawkesProcessIntensityIteration": "composable-deferred: kernel + positional partition index",

	// live-object — no data form
	"FromStorageIteration":              "live-object: [][]float64 bulk data",
	"ValuesChangingEventsIteration":     "live-object: map[float64]Iteration + nested Iteration",
	"ValuesWeightedResamplingIteration": "live-object: rand.Source",
	"DataComparisonGradientIteration":   "live-object: *StateHistory batch + gradient func",
	"EmbeddedSimulationRunIteration":    "live-object: *Settings/*Implementations",
}

// candidatePackages are the packages whose Iteration implementations are
// candidates for the data-only registry (agents/analysis iterations are the
// macro tier, out of scope here).
var candidatePackages = []string{
	"../continuous", "../discrete", "../general", "../inference",
}

// TestIterationCoverage is drift test 2: it scans the candidate packages for every
// type with an Iterate method and asserts each is either registered or excluded
// with a reason, so a newly-added iteration cannot silently escape classification.
func TestIterationCoverage(t *testing.T) {
	registered := make(map[string]bool)
	for _, goType := range wantIterationType {
		// goType is like "*continuous.WienerProcessIteration"; take the bare name.
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
	ident, ok := star.X.(*ast.Ident)
	if !ok || !ident.IsExported() {
		return ""
	}
	return ident.Name
}
