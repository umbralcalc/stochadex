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
//   - "live-object": holds a live object bound at runtime by its caller — bulk
//     [][]float64 data, a batch *StateHistory, or a whole *Settings/*Implementations
//     pair — with no data form as a standalone partition. All three are still
//     reachable from config indirectly, constructed by the data:, macros: and
//     embedded: tiers respectively.
//
// Drift test 2 asserts every Iterate-implementing type in the candidate packages
// is either registered (wantIterationType) or listed here. A NEW iteration then
// fails CI until it is classified — the guard that stops the registry silently
// lagging the framework.
//
// A field of live-object *type* is not on its own grounds for exclusion: an RNG
// source that Configure assigns from the partition seed is inert as config, and
// nested iterations are expressible as recursive specs. Both of those cases were
// excluded here once and have since been registered.
var excludedIterations = map[string]string{
	// live-object — no data form as a standalone partition
	"FromStorageIteration":            "live-object: [][]float64 bulk data (built by the data: tier)",
	"DataComparisonGradientIteration": "live-object: *StateHistory batch + gradient func (built by macros:)",
	"EmbeddedSimulationRunIteration":  "live-object: *Settings/*Implementations (built by the embedded: tier)",
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
