package solarfleet

// Proves declarative.yaml is the SAME model as the Go stub, in two independent layers:
//
//  1. Step-for-step — randomised inputs through single Iterate calls on each bespoke
//     iteration and its declarative twin, asserting exact agreement. Catches a
//     mis-stated formula immediately.
//  2. Whole-suite — re-run ObservedBehaviour() against the declarative build. Catches
//     what per-step agreement cannot: wrong wiring, wrong params, wrong state layout.
//
// Oracle: EXACT (tolerance 1e-12, FMA-scale). The sites partition draws its S normals
// from rng.New(seed) in the same order `iid(4, normal(0,1))` does, and every other
// operation is deterministic, so both models run on one stream and equivalence is
// decidable value by value. The residue is Go's fused multiply-add contracting
// `a + b*c`, which the evaluator computes as separate ops — the FMA, not the model.

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

const tolerance = 1e-12

// declarativeBuildStub assembles the model from declarative.yaml, matching BuildStub's
// signature so it drops into the behaviour helpers. The YAML holds the model (the
// expressions and wiring); the site-dependent DATA that BuildStub computes in Go — the
// clear-sky driver and the Cholesky factor — is injected here identically, so both forms
// run on identical inputs and streams. This is what makes the whole-suite comparison a
// test of the DSL formulas, not of the data.
func declarativeBuildStub(cloudVolatility float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	config := api.LoadApiRunConfigFromYaml("declarative.yaml")
	config.Main.Simulation = simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSize},
		InitTimeValue:    0.0,
	}
	gen := config.Main.GetConfigGenerator()

	sites := DefaultFleet()
	kernel := DefaultKernel(cloudVolatility)
	poaRows := ClearSkyDriverRows(sites, numSteps)
	clearsky := gen.GetPartition("clearsky")
	clearsky.Iteration = &general.FromStorageIteration{Data: poaRows}
	clearsky.InitStateValues = append([]float64(nil), poaRows[0]...)
	clearsky.Seed = seed
	gen.GetPartition("sites").Params.Map["lflat"] = CholeskyFactorFlat(sites, kernel)
	gen.GetPartition("sites").Seed = seed + 1
	gen.GetPartition("fleet").Seed = seed + 2
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition.
func declarativeIteration(t *testing.T, partition string) *general.ExpressionIteration {
	t.Helper()
	config := declarativeBuildStub(DefaultCloudVolatility, 10, 0).GetPartition(partition)
	iteration, ok := config.Iteration.(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("%s is not expression-backed: got %T", partition, config.Iteration)
	}
	return iteration
}

// assertClose fails when two values differ by more than the FMA-scale tolerance,
// comparing relatively once the magnitude exceeds 1.
func assertClose(t *testing.T, got, want float64, context string) float64 {
	t.Helper()
	d := math.Abs(got - want)
	if s := math.Abs(want); s > 1 {
		d /= s
	}
	if d > tolerance {
		t.Fatalf("%s: declarative=%v bespoke=%v deviation=%g", context, got, want, d)
	}
	return d
}

func hist(width int, row []float64) *simulator.StateHistory {
	return &simulator.StateHistory{
		Values:            mat.NewDense(1, width, append([]float64{}, row...)),
		StateWidth:        width,
		StateHistoryDepth: 1,
	}
}

func tsAt(step int, dt float64) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{float64(step) * dt}),
		NextIncrement:     dt,
		CurrentStepNumber: step,
	}
}

// sitesParams builds the sites partition's params for a direct Iterate call, with a
// given clear-sky POA vector (the value the clearsky upstream would supply this step).
func sitesParams(lflat, poa []float64) simulator.Params {
	return simulator.NewParams(map[string][]float64{
		"theta": {DefaultReversion},
		"mu":    {DefaultMu, DefaultMu, DefaultMu, DefaultMu},
		"kwp":   {DefaultKwp, DefaultKwp, DefaultKwp, DefaultKwp},
		"eta":   {DefaultEta, DefaultEta, DefaultEta, DefaultEta},
		"i_stc": {IStandardTestConditions},
		"k_max": {DefaultKMax},
		"lflat": lflat,
		"poa":   poa,
	})
}

// TestDeclarativeSitesMatchesBespoke — the wide clear-sky-index field. The 4 normals of
// the innovation are drawn identically on both sides, so the comparison is exact. Inputs
// sweep night vs day (poa 0 vs positive), the kidx clip (log K driven above and below the
// cap), and dt below/at/above 1 (so a dropped `* dt` or `sqrt(dt)` cannot hide).
func TestDeclarativeSitesMatchesBespoke(t *testing.T) {
	lflat := CholeskyFactorFlat(DefaultFleet(), DefaultKernel(DefaultCloudVolatility))
	bespoke := &FleetSkyIteration{}
	declarative := declarativeIteration(t, "sites")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{
			Name:            "sites",
			Seed:            5,
			InitStateValues: make([]float64, 8), // fixes numSites = 4
		}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(11, 12))
	branches := map[string]int{"night": 0, "day": 0, "kidx_clipped": 0, "kidx_free": 0}
	maxDev := 0.0
	for c := 0; c < 4000; c++ {
		// Previous row: log K spread wide enough to drive kidx both below and above k_max,
		// plus a previous-gen half that the update ignores.
		prev := make([]float64, 8)
		for i := 0; i < 4; i++ {
			prev[i] = -2.0 + 4.0*rng.Float64() // log K in [-2, 2]; exp spans well past k_max
			prev[4+i] = 100.0 * rng.Float64()
		}
		poa := make([]float64, 4)
		night := c%3 == 0
		for i := 0; i < 4; i++ {
			if !night {
				poa[i] = 900.0 * rng.Float64()
			}
		}
		if night {
			branches["night"]++
		} else {
			branches["day"]++
		}
		dt := []float64{0.5, 1.0, 1.7}[c%3]

		params := sitesParams(lflat, poa)
		pB, pD := params, params
		gotB := append([]float64(nil), bespoke.Iterate(&pB, 0, []*simulator.StateHistory{hist(8, prev)}, tsAt(c+1, dt))...)
		gotD := declarative.Iterate(&pD, 0, []*simulator.StateHistory{hist(8, prev)}, tsAt(c+1, dt))
		if len(gotB) != 8 || len(gotD) != 8 {
			t.Fatalf("case %d: widths %d/%d, want 8", c, len(gotB), len(gotD))
		}
		for i := 0; i < 8; i++ {
			maxDev = math.Max(maxDev, assertClose(t, gotD[i], gotB[i], "sites output"))
		}
		// Classify the kidx clip from the log K the update produced (first 4 outputs).
		for i := 0; i < 4; i++ {
			if math.Exp(gotB[i]) >= DefaultKMax {
				branches["kidx_clipped"]++
			} else {
				branches["kidx_free"]++
			}
		}
	}
	for name, n := range branches {
		if n == 0 {
			t.Errorf("branch %q never exercised", name)
		}
	}
	t.Logf("branches: %v  max deviation: %g", branches, maxDev)
}

// TestDeclarativeFleetMatchesBespoke — the aggregate is a deterministic sum of the
// generation half of the sites row; no draws, so exact.
func TestDeclarativeFleetMatchesBespoke(t *testing.T) {
	bespoke := &FleetAggregateIteration{}
	declarative := declarativeIteration(t, "fleet")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "fleet", Seed: 0, InitStateValues: []float64{0}}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(7, 8))
	maxDev := 0.0
	for c := 0; c < 2000; c++ {
		s := make([]float64, 8)
		for i := range s {
			s[i] = 50.0 * rng.Float64()
		}
		params := simulator.NewParams(map[string][]float64{"s": s})
		pB, pD := params, params
		state := []*simulator.StateHistory{hist(1, []float64{rng.Float64()})}
		gotB := bespoke.Iterate(&pB, 0, state, tsAt(c+1, 1.0))
		gotD := declarative.Iterate(&pD, 0, state, tsAt(c+1, 1.0))
		maxDev = math.Max(maxDev, assertClose(t, gotD[0], gotB[0], "fleet total"))
	}
	t.Logf("max deviation: %g", maxDev)
}

// TestDeclarativeAnswersTheSameClaims — the whole-suite layer: every claim, re-measured
// against the declarative build, must match the Go stub's number (not merely its
// direction). Skipped under -short (it runs the full behaviour ensembles twice).
func TestDeclarativeAnswersTheSameClaims(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping whole-suite equivalence under -short")
	}
	fromGo := ObservedBehaviour()
	fromYaml := observedBehaviour(declarativeBuildStub)
	if len(fromGo) != len(fromYaml) {
		t.Fatalf("claim counts differ: go %d, yaml %d", len(fromGo), len(fromYaml))
	}
	for k := range fromGo {
		if fromGo[k].ID != fromYaml[k].ID {
			t.Fatalf("claim %d id mismatch: go %q, yaml %q", k, fromGo[k].ID, fromYaml[k].ID)
		}
		if err := cardgen.Verify(fromYaml[k]); err != nil {
			t.Errorf("%s: declarative claim fails to verify: %v", fromYaml[k].ID, err)
		}
		if len(fromGo[k].Observations) != len(fromYaml[k].Observations) {
			t.Fatalf("%s: observation counts differ", fromGo[k].ID)
		}
		for j := range fromGo[k].Observations {
			assertClose(t, fromYaml[k].Observations[j].Value, fromGo[k].Observations[j].Value,
				fromGo[k].ID+"/"+fromGo[k].Observations[j].Label)
		}
	}
}
