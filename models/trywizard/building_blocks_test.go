package rugby

// Does building_blocks.yaml, this model with built-in iterations wherever one states a
// partition's update, run the same model as declarative.yaml? The built-ins draw the same
// streams in the same order as the expressions they replace, so the two agree step for
// step, to rounding.

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/twins"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// buildingBlocksBuildStub assembles the model from building_blocks.yaml, matching
// buildStubWithStrategy's signature.
func buildingBlocksBuildStub(
	strategy *SubstitutionStrategy,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	return buildFromYAML("building_blocks.yaml", strategy, numSteps, seed)
}

func TestBuildingBlocksRunTheDeclarativeModel(t *testing.T) {
	for _, seed := range []uint64{0, 7, 42} {
		strategy := DefaultSubstitutionStrategy(DefaultHomeSubMinute)
		// At a stepsize of 1, a dropped dt or sqrt(dt) changes nothing, so the twins are
		// compared at another stepsize too.
		for _, stepsize := range []float64{1, 0.37} {
			want := twins.Run(twins.WithStepsize(declarativeBuildStub(strategy, DefaultNumSteps, seed), stepsize))
			got := twins.Run(twins.WithStepsize(buildingBlocksBuildStub(strategy, DefaultNumSteps, seed), stepsize))
			if err := twins.Compare(got, want, tolerance); err != nil {
				t.Errorf("seed %d, stepsize %v: %v", seed, stepsize, err)
			}
		}
	}
}

// BenchmarkTwins runs the model for a match three ways, timing the run alone: the Go
// stub, the declarative twin, and the building-block twin.
func BenchmarkTwins(b *testing.B) {
	for _, build := range []struct {
		name  string
		build func(*SubstitutionStrategy, int, uint64) *simulator.ConfigGenerator
	}{
		{"stub", buildStubWithStrategy},
		{"declarative", declarativeBuildStub},
		{"building_blocks", buildingBlocksBuildStub},
	} {
		b.Run(build.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				// Building a twin loads its YAML, which is not what this times.
				b.StopTimer()
				gen := build.build(DefaultSubstitutionStrategy(DefaultHomeSubMinute), DefaultNumSteps, 0)
				b.StartTimer()
				twins.RunQuietly(gen)
			}
		})
	}
}
