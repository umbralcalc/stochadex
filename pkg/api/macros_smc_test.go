package api

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestParticleTemplateInstantiation guards SMC correctness at the mechanism level:
// each particle must get its OWN partition with {particle} substituted through the
// name and every upstream reference, and its OWN params map — otherwise particles
// silently share state and the inference is quietly wrong.
func TestParticleTemplateInstantiation(t *testing.T) {
	template := simulator.PartitionConfig{
		Name:            "pred_{particle}",
		Params:          simulator.NewParams(map[string][]float64{"param_values": {0.0}}),
		InitStateValues: []float64{0.0},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"mean": {Upstream: "pred_{particle}"},
		},
		ParamsAsPartitions: map[string][]string{
			"peer": {"loglike_{particle}"},
		},
	}

	p0, err := instantiateParticle(template, 0)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := instantiateParticle(template, 1)
	if err != nil {
		t.Fatal(err)
	}

	if p0.Name != "pred_0" || p1.Name != "pred_1" {
		t.Errorf("names not substituted: %q, %q", p0.Name, p1.Name)
	}
	if p0.ParamsFromUpstream["mean"].Upstream != "pred_0" {
		t.Errorf("upstream not substituted: %q", p0.ParamsFromUpstream["mean"].Upstream)
	}
	if p1.ParamsAsPartitions["peer"][0] != "loglike_1" {
		t.Errorf("params_as_partitions not substituted: %v", p1.ParamsAsPartitions["peer"])
	}

	// Deep-copy invariant: mutating one particle's params must not touch the other's
	// or the template's — upstream injection writes into params at runtime.
	p0.Params.Map["param_values"][0] = 99.0
	if p1.Params.Map["param_values"][0] != 0.0 {
		t.Error("params not deep-copied: particle 1 saw particle 0's mutation")
	}
	if template.Params.Map["param_values"][0] != 0.0 {
		t.Error("params not deep-copied: the template was mutated")
	}
}

// TestLiveMacroRejectsAgainstStorage checks the live macros error clearly if run
// through the against-storage path, rather than misbehaving.
func TestLiveMacroRejectsAgainstStorage(t *testing.T) {
	for _, spec := range []macroSpec{&evolutionStrategySpec{}, &smcInferenceSpec{}} {
		if _, _, err := spec.resolve(nil); err == nil {
			t.Errorf("%T.resolve should error (it is a live macro)", spec)
		}
	}
}
