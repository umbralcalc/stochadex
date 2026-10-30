package api

import (
	"reflect"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CatalogueVersion is the version of the Catalogue's JSON shape. It changes
// only when a key is removed or its meaning changes; new keys may be added
// within a version.
const CatalogueVersion = 1

// Catalogue describes what this build can run: its version and features, and
// the type name of every component a config can name, read from the
// registries the config resolves through (so downstream registrations, such as
// onnx_inference in a build with the onnx tag, are included). Every list is
// sorted and never null, so the JSON is identical for identical builds.
type Catalogue struct {
	CatalogueVersion int      `json:"catalogue_version"`
	Version          string   `json:"version"`
	Features         []string `json:"features"`
	// Iterations are the iteration: {type: ...} names.
	Iterations []CatalogueEntry `json:"iterations"`
	// DataSources are the keys of a data: or inputs: source, e.g. csv in
	// source: {csv: {...}}.
	DataSources []CatalogueEntry `json:"data_sources"`
	// StreamTransports are the keys of an inputs: stream, e.g. websocket in
	// stream: {websocket: {...}}.
	StreamTransports      []CatalogueEntry `json:"stream_transports"`
	OutputFunctions       []CatalogueEntry `json:"output_functions"`
	OutputConditions      []CatalogueEntry `json:"output_conditions"`
	TerminationConditions []CatalogueEntry `json:"termination_conditions"`
	TimestepFunctions     []CatalogueEntry `json:"timestep_functions"`
	ExecutionStrategies   []CatalogueEntry `json:"execution_strategies"`
	// Macros are the macros: {type: ...} names.
	Macros []CatalogueEntry `json:"macros"`
	// Kernels, Likelihoods, Priors and Jumps are the nested specs composable
	// iterations and macros take, e.g. kernel: {type: exponential}.
	Kernels     []CatalogueEntry `json:"kernels"`
	Likelihoods []CatalogueEntry `json:"likelihoods"`
	Priors      []CatalogueEntry `json:"priors"`
	Jumps       []CatalogueEntry `json:"jumps"`
	// Environments are the mcts_* environment: and mcts_self_play env: names:
	// the engine's own simulation, and those registered with
	// RegisterEnvironment.
	Environments []CatalogueEntry `json:"environments"`
}

// CatalogueEntry is one registered type. Params lists the spec fields it
// takes, where the registry knows them; it is omitted otherwise.
type CatalogueEntry struct {
	Name   string   `json:"name"`
	Params []string `json:"params,omitempty"`
}

// BuildCatalogue reads the catalogue of this build from the registries.
func BuildCatalogue() Catalogue {
	features := append([]string{}, BuildFeatures...)
	sort.Strings(features)
	return Catalogue{
		CatalogueVersion: CatalogueVersion,
		Version:          BuildVersion,
		Features:         features,
		Iterations: entries(keys(iterationBuilders), keys(scopedIterationBuilders),
			keys(extraIterationBuilders)),
		DataSources:           entries(yamlKeys(reflect.TypeOf(DataSource{})), keys(extraDataSources)),
		StreamTransports:      entries(yamlKeys(reflect.TypeOf(StreamConfig{})), registeredStreamTransportNames()),
		OutputFunctions:       entries(simulator.ComponentTypes("output_function")),
		OutputConditions:      entries(simulator.ComponentTypes("output_condition")),
		TerminationConditions: entries(simulator.ComponentTypes("termination_condition")),
		TimestepFunctions:     entries(simulator.ComponentTypes("timestep_function")),
		ExecutionStrategies:   entries(simulator.ComponentTypes("execution_strategy")),
		Macros:                entries(keys(macroSpecFactories)),
		Kernels:               entries(keys(kernelBuilders)),
		Likelihoods:           entries(keys(likelihoodBuilders)),
		Priors:                entries(keys(priorBuilders)),
		Jumps:                 entries(keys(jumpBuilders)),
		Environments:          entries([]string{simulationEnvironmentType}, keys(environmentBuilders)),
	}
}

// entries merges name lists into sorted, de-duplicated catalogue entries.
func entries(lists ...[]string) []CatalogueEntry {
	seen := map[string]bool{}
	for _, names := range lists {
		for _, name := range names {
			seen[name] = true
		}
	}
	result := make([]CatalogueEntry, 0, len(seen))
	for _, name := range keys(seen) {
		result = append(result, CatalogueEntry{Name: name})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// keys returns a map's keys.
func keys[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names
}

// yamlKeys returns the YAML keys of a struct's named fields: the spellings it
// declares itself, leaving out its ,inline catch-all for registered ones.
func yamlKeys(t reflect.Type) []string {
	names := []string{}
	for i := 0; i < t.NumField(); i++ {
		name, options, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name == "" || name == "-" || strings.Contains(options, "inline") {
			continue
		}
		names = append(names, name)
	}
	return names
}

// registeredStreamTransportNames returns the transports registered with
// RegisterStreamTransport.
func registeredStreamTransportNames() []string {
	streamTransportsMu.RLock()
	defer streamTransportsMu.RUnlock()
	return keys(streamTransports)
}
