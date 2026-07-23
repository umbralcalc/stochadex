package api

import (
	"fmt"
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runOnePartition runs a single seeded partition to `steps` and returns its
// recorded trajectory — the substrate for behaviour-equivalence checks.
func runOnePartition(
	iteration simulator.Iteration,
	params map[string][]float64,
	init []float64,
	seed uint64,
	steps int,
) [][]float64 {
	storage := analysis.NewStateTimeStorageFromPartitions(
		[]*simulator.PartitionConfig{{
			Name:              "p",
			Iteration:         iteration,
			Params:            simulator.NewParams(params),
			InitStateValues:   init,
			StateHistoryDepth: 1,
			Seed:              seed,
		}},
		&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0},
		0.0,
	)
	return storage.GetValues("p")
}

// TestIterationRegistryBehaviourEquivalence is the core registry invariant: an
// iteration built from a data spec must produce output *identical* to the same
// iteration constructed directly in Go (same params, same seed). This catches a
// registry that constructs the wrong type or fails to wire a field — mistakes
// that a "does it run?" test would miss because the wrong iteration still runs.
func TestIterationRegistryBehaviourEquivalence(t *testing.T) {
	cases := []struct {
		name   string
		spec   simulator.ComponentSpec
		goIter simulator.Iteration
		params map[string][]float64
		init   []float64
	}{
		{
			name:   "wiener_process (no fields)",
			spec:   simulator.ComponentSpec{Type: "wiener_process"},
			goIter: &continuous.WienerProcessIteration{},
			params: map[string][]float64{"variances": {1.0, 4.0}},
			init:   []float64{0.0, 0.0},
		},
		{
			name:   "ornstein_uhlenbeck (no fields)",
			spec:   simulator.ComponentSpec{Type: "ornstein_uhlenbeck"},
			goIter: &continuous.OrnsteinUhlenbeckIteration{},
			params: map[string][]float64{"mus": {1.0}, "sigmas": {1.5}, "thetas": {1.0}},
			init:   []float64{3.0},
		},
		{
			name:   "poisson_process (no fields)",
			spec:   simulator.ComponentSpec{Type: "poisson_process"},
			goIter: &discrete.PoissonProcessIteration{},
			params: map[string][]float64{"rates": {0.5, 1.0}},
			init:   []float64{0.0, 0.0},
		},
		{
			// The composable case that matters most: a nested interface value with a
			// data field (the bool). A registry that dropped the bool, or built a
			// different likelihood, would diverge here.
			name: "data_generation with normal(allow_default_covariance_fallback)",
			spec: simulator.ComponentSpec{
				Type: "data_generation",
				Fields: map[string]interface{}{
					"likelihood": map[string]interface{}{
						"type": "normal", "allow_default_covariance_fallback": true,
					},
				},
			},
			goIter: &inference.DataGenerationIteration{
				Likelihood: &inference.NormalLikelihoodDistribution{AllowDefaultCovarianceFallback: true},
			},
			params: map[string][]float64{
				"mean": {1.8, 5.0}, "covariance_matrix": {2.5, 0, 0, 9.0},
			},
			init: []float64{1.3, 8.3},
		},
		{
			// A nested iteration inside a composable one (cumulative wraps a
			// Configure-free iteration, its intended use).
			name: "cumulative wrapping constant_values",
			spec: simulator.ComponentSpec{
				Type:   "cumulative",
				Fields: map[string]interface{}{"iteration": map[string]interface{}{"type": "constant_values"}},
			},
			goIter: &general.CumulativeIteration{Iteration: &general.ConstantValuesIteration{}},
			params: map[string][]float64{},
			init:   []float64{2.0, -1.0},
		},
		{
			// A nested iteration that needs Configure (a sampler) — guards the
			// Configure-propagation fix in CumulativeIteration.
			name: "cumulative wrapping wiener_process",
			spec: simulator.ComponentSpec{
				Type:   "cumulative",
				Fields: map[string]interface{}{"iteration": map[string]interface{}{"type": "wiener_process"}},
			},
			goIter: &general.CumulativeIteration{Iteration: &continuous.WienerProcessIteration{}},
			params: map[string][]float64{"variances": {1.0}},
			init:   []float64{0.0},
		},
		{
			// The expressions DSL as an inline iteration (the general fix that lets a
			// reward/objective be written as maths inside a macro window).
			name: "expression iteration (inline)",
			spec: simulator.ComponentSpec{
				Type: "expression",
				Fields: map[string]interface{}{
					"fields":  []interface{}{map[string]interface{}{"name": "x"}},
					"outputs": []interface{}{"x + rate * dt"},
				},
			},
			goIter: &general.ExpressionIteration{
				Fields:  []general.ExpressionField{{Name: "x"}},
				Outputs: []string{"x + rate * dt"},
			},
			params: map[string][]float64{"rate": {0.5}},
			init:   []float64{0.0},
		},
		{
			// A nested-iteration *map* keyed by event value, plus a whole named
			// value function — the two pieces of machinery this iteration needs.
			// The mapped iteration is seeded, so a registry that built the map but
			// dropped the Configure propagation would diverge here.
			name: "values_changing_events switching on a params event",
			spec: simulator.ComponentSpec{
				Type: "values_changing_events",
				Fields: map[string]interface{}{
					"event_iteration": map[string]interface{}{
						"type": "values_function", "function": "params_event",
					},
					"iteration_by_event": []interface{}{
						map[string]interface{}{
							"event":     1.0,
							"iteration": map[string]interface{}{"type": "wiener_process"},
						},
					},
				},
			},
			goIter: &general.ValuesChangingEventsIteration{
				EventIteration: &general.ValuesFunctionIteration{
					Function: general.ParamsEventFunction,
				},
				IterationByEvent: map[float64]simulator.Iteration{
					1.0: &continuous.WienerProcessIteration{},
				},
			},
			params: map[string][]float64{"event": {1.0}, "variances": {1.0}},
			init:   []float64{0.0},
		},
		{
			// A composable one with a nested JumpDistribution.
			name: "compound_poisson_process with gamma_jump",
			spec: simulator.ComponentSpec{
				Type:   "compound_poisson_process",
				Fields: map[string]interface{}{"jump_dist": map[string]interface{}{"type": "gamma_jump"}},
			},
			goIter: &continuous.CompoundPoissonProcessIteration{JumpDist: &continuous.GammaJumpDistribution{}},
			params: map[string][]float64{
				"rates": {0.5}, "gamma_alphas": {1.0}, "gamma_betas": {1.0},
			},
			init: []float64{0.0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := ResolveIteration(tc.spec)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			fromData := runOnePartition(resolved, tc.params, tc.init, 42, 30)
			fromGo := runOnePartition(tc.goIter, tc.params, tc.init, 42, 30)
			if len(fromData) != len(fromGo) {
				t.Fatalf("row counts differ: data %d, go %d", len(fromData), len(fromGo))
			}
			for step := range fromGo {
				for i := range fromGo[step] {
					if fromData[step][i] != fromGo[step][i] {
						t.Fatalf("step %d value %d: data-spec %v != go %v (registry mis-wired)",
							step, i, fromData[step][i], fromGo[step][i])
					}
				}
			}
		})
	}
}

// runPartitions runs a multi-partition simulation and returns the named
// partition's trajectory. Iterations that read *other* partitions' histories
// cannot be exercised by runOnePartition, so they get their wiring built here.
func runPartitions(
	partitions []*simulator.PartitionConfig,
	subject string,
	steps int,
) [][]float64 {
	storage := analysis.NewStateTimeStorageFromPartitions(
		partitions,
		&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0},
		0.0,
	)
	return storage.GetValues(subject)
}

// hawkesPartitions wires an intensity iteration to the Hawkes counting process
// it excites: the counter reads the intensity's output within-step, and the
// intensity reads the counter's history by name via params_as_partitions —
// the name-based form of the partition index its Configure resolves.
func hawkesPartitions(intensity simulator.Iteration) []*simulator.PartitionConfig {
	return []*simulator.PartitionConfig{
		{
			Name:      "intensity",
			Iteration: intensity,
			Params: simulator.NewParams(map[string][]float64{
				"background_rates":                {1.4, 1.2},
				"exponential_weighting_timescale": {1.1},
			}),
			ParamsAsPartitions: map[string][]string{
				"hawkes_partition_index": {"hawkes"},
			},
			InitStateValues:   []float64{0.0, 0.0},
			StateHistoryDepth: 10,
			Seed:              253,
		},
		{
			Name:      "hawkes",
			Iteration: &discrete.HawkesProcessIteration{},
			Params:    simulator.NewParams(map[string][]float64{}),
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"intensity": {Upstream: "intensity"},
			},
			InitStateValues:   []float64{0.0, 0.0},
			StateHistoryDepth: 10,
			Seed:              1112,
		},
	}
}

// resamplingPartitions wires a resampling iteration to two weight partitions and
// two data partitions, all named rather than indexed.
func resamplingPartitions(resampling simulator.Iteration) []*simulator.PartitionConfig {
	constant := func(name string, init []float64) *simulator.PartitionConfig {
		return &simulator.PartitionConfig{
			Name:              name,
			Iteration:         &general.ConstantValuesIteration{},
			Params:            simulator.NewParams(map[string][]float64{}),
			InitStateValues:   init,
			StateHistoryDepth: 5,
			Seed:              0,
		}
	}
	return []*simulator.PartitionConfig{
		constant("weights_a", []float64{-0.5}),
		constant("weights_b", []float64{-1.5}),
		constant("data_a", []float64{1.0, 2.0, 3.0}),
		constant("data_b", []float64{4.0, 5.0, 6.0}),
		{
			Name:      "resampled",
			Iteration: resampling,
			Params: simulator.NewParams(map[string][]float64{
				"past_discounting_factor": {0.98},
			}),
			ParamsAsPartitions: map[string][]string{
				"log_weight_partitions":  {"weights_a", "weights_b"},
				"data_values_partitions": {"data_a", "data_b"},
			},
			InitStateValues:   []float64{0.0, 0.0, 0.0},
			StateHistoryDepth: 2,
			Seed:              1735,
		},
	}
}

// TestMultiPartitionRegistryBehaviourEquivalence is the same invariant as
// TestIterationRegistryBehaviourEquivalence for the iterations that read other
// partitions' state histories, which a single-partition run cannot reach.
func TestMultiPartitionRegistryBehaviourEquivalence(t *testing.T) {
	cases := []struct {
		name    string
		spec    simulator.ComponentSpec
		goIter  simulator.Iteration
		wire    func(simulator.Iteration) []*simulator.PartitionConfig
		subject string
	}{
		{
			name: "hawkes_process_intensity with an exponential kernel",
			spec: simulator.ComponentSpec{
				Type: "hawkes_process_intensity",
				Fields: map[string]interface{}{
					"kernel": map[string]interface{}{"type": "exponential"},
				},
			},
			goIter: &discrete.HawkesProcessIntensityIteration{
				ExcitingKernel: &kernels.ExponentialIntegrationKernel{},
			},
			wire:    hawkesPartitions,
			subject: "intensity",
		},
		{
			name:    "values_weighted_resampling (seeded from the partition seed)",
			spec:    simulator.ComponentSpec{Type: "values_weighted_resampling"},
			goIter:  &general.ValuesWeightedResamplingIteration{},
			wire:    resamplingPartitions,
			subject: "resampled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := ResolveIteration(tc.spec)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			fromData := runPartitions(tc.wire(resolved), tc.subject, 30)
			fromGo := runPartitions(tc.wire(tc.goIter), tc.subject, 30)
			if len(fromData) != len(fromGo) {
				t.Fatalf("row counts differ: data %d, go %d", len(fromData), len(fromGo))
			}
			// A subject that never moves would make the comparison below vacuous:
			// two identical constant series agree however badly the registry is wired.
			distinct := make(map[string]bool)
			for _, row := range fromGo {
				distinct[fmt.Sprint(row)] = true
			}
			if len(distinct) < 2 {
				t.Fatalf("subject %q never varies over the run — "+
					"the equivalence check would be vacuous", tc.subject)
			}
			for step := range fromGo {
				for i := range fromGo[step] {
					if fromData[step][i] != fromGo[step][i] {
						t.Fatalf("step %d value %d: data-spec %v != go %v (registry mis-wired)",
							step, i, fromData[step][i], fromGo[step][i])
					}
				}
			}
		})
	}
}

// TestSimulationComponentBehaviourEquivalence does the same for a resolved
// data-spec simulation component: a data-spec ConstantTimestepFunction must step
// the same as its Go twin.
func TestSimulationComponentBehaviourEquivalence(t *testing.T) {
	dataSpec, err := simulator.ResolveTimestepFunction(simulator.ComponentSpec{
		Type: "constant", Fields: map[string]interface{}{"stepsize": 0.25},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A ConstantTimestepFunction ignores its arguments; both should return 0.25.
	if got := dataSpec.NextIncrement(nil); math.Abs(got-0.25) > 1e-12 {
		t.Errorf("data-spec constant timestep = %v, want 0.25", got)
	}
}
