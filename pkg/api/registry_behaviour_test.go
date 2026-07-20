package api

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
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
