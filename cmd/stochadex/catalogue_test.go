package main

import (
	"slices"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/api"
)

// catalogueHas reports whether a catalogue list names name.
func catalogueHas(list []api.CatalogueEntry, name string) bool {
	return slices.ContainsFunc(list, func(entry api.CatalogueEntry) bool { return entry.Name == name })
}

// TestCatalogueListsThisBuild checks that `stochadex catalogue`, built into this
// CLI, lists what this module registers on top of the engine, and the features
// it reports, as main hands them to the engine.
func TestCatalogueListsThisBuild(t *testing.T) {
	api.BuildFeatures = features
	defer func() { api.BuildFeatures = nil }()
	catalogue := api.BuildCatalogue()
	for _, source := range []string{"arrow", "s3"} {
		if !catalogueHas(catalogue.DataSources, source) {
			t.Errorf("data_sources has no %s", source)
		}
	}
	for _, sink := range []string{"arrow", "s3"} {
		if !catalogueHas(catalogue.OutputFunctions, sink) {
			t.Errorf("output_functions has no %s", sink)
		}
	}
	if slices.Contains(features, "duckdb") != catalogueHas(catalogue.OutputFunctions, "duckdb") {
		t.Errorf("the duckdb output is listed only with the duckdb feature: features %v", features)
	}
	if slices.Contains(features, "onnx") != catalogueHas(catalogue.Iterations, "onnx_inference") {
		t.Errorf("onnx_inference is listed only with the onnx feature: features %v", features)
	}
	if !catalogueHas(catalogue.StreamTransports, "keyboard") {
		t.Error("stream_transports has no keyboard")
	}
	for _, feature := range features {
		if !slices.Contains(catalogue.Features, feature) {
			t.Errorf("features has no %s", feature)
		}
	}
}
