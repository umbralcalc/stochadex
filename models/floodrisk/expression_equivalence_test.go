package floodrisk

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// Two independent checks, because they fail in different ways. The step-for-step tests
// compare single Iterate calls over randomised inputs, which catches a mis-stated formula
// exactly. The suite test re-runs the model's own claim computations against the
// declarative build, which catches a model that is subtly different in a way per-step
// agreement would not: wrong wiring, wrong param values, wrong state layout.
//
// The rainfall partition draws from a stream seeded exactly as the declarative one is
// (rng.New(seed) and rand.New(rand.NewPCG(seed, seed)) are the same generator, and
// rng.Gamma reproduces distuv.Gamma's branch structure and draw order), and both take the
// same number of draws per step — one uniform always, one Gamma only on a wet day — so the
// two stay in lockstep and equivalence is decidable directly rather than only in
// distribution. Agreement is asserted to a tight tolerance rather than bit-for-bit:
// compiled Go is free to contract a + b*c into a fused multiply-add, which rounds
// differently from the evaluator's separate operations.

import (
	"math"
	"math/rand/v2"
	"testing"

	"gonum.org/v1/gonum/mat"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// tolerance is well above FMA rounding and well below any difference a real modelling
// discrepancy would produce.
const tolerance = 1e-12

// declarativeBuildStub assembles the model from declarative.yaml, matching BuildStub's
// signature so it can be dropped into the behaviour helpers. The YAML holds the model; the
// run knobs BuildStub takes as arguments are injected here, exactly as BuildStub injects
// them into its Go partitions.
func declarativeBuildStub(
	rainfallMultiplier float64,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	config := api.LoadApiRunConfigFromYaml("declarative.yaml")
	config.Main.Simulation = simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	}
	gen := config.Main.GetConfigGenerator()
	gen.GetPartition("rainfall").Params.Map["rainfall_multiplier"] =
		[]float64{rainfallMultiplier}
	gen.GetPartition("rainfall").Seed = seed
	// runoff is deterministic, so its seed is fixed at zero exactly as BuildStub fixes it.
	gen.GetPartition("runoff").Seed = 0
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition,
// alongside the params it declares.
func declarativeIteration(
	t *testing.T,
	partition string,
) (*general.ExpressionIteration, map[string][]float64) {
	t.Helper()
	config := declarativeBuildStub(1.0, 10, 0).GetPartition(partition)
	iteration, ok := config.Iteration.(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("%s is not expression-backed: got %T", partition, config.Iteration)
	}
	return iteration, config.Params.Map
}

// assertClose fails when two values differ by more than the FMA-scale tolerance, comparing
// relatively once the magnitude exceeds 1.
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

func TestDeclarativeRainfallMatchesBespoke(t *testing.T) {
	bespoke := &StochasticRainfallIteration{}
	declarative, _ := declarativeIteration(t, "rainfall")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "rainfall", Seed: 5}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(31, 32))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// Previous rainfall straddles the threshold so both Markov transition rows are used,
		// and the shape/scale floors are swept periodically so the degenerate-param clamps
		// are compared rather than assumed. A tiny scale drives wet-day amounts under the
		// threshold, which is what exercises the floor.
		prevRainfall := rng.Float64() * 2.0
		shape := 0.05 + rng.Float64()*2.0
		if i%7 == 0 {
			shape = rng.Float64() * 0.005
		}
		scale := 1.0 + rng.Float64()*11.0
		if i%11 == 0 {
			scale = rng.Float64() * 0.005
		}
		threshold := 0.05 + rng.Float64()*1.5
		p := simulator.NewParams(map[string][]float64{
			"wet_day_shape":       {shape},
			"wet_day_scale":       {scale},
			"p_wet_given_dry":     {rng.Float64()},
			"p_wet_given_wet":     {rng.Float64()},
			"rainfall_multiplier": {0.5 + rng.Float64()*1.5},
			"wet_threshold":       {threshold},
		})
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{{
				Values:            mat.NewDense(1, 1, []float64{prevRainfall}),
				StateWidth:        1,
				StateHistoryDepth: 1,
			}}
		}
		ts := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{float64(i)}),
			NextIncrement:     1.0,
			CurrentStepNumber: i + 1,
		}

		want := bespoke.Iterate(&p, 0, mk(), ts)
		got := declarative.Iterate(&p, 0, mk(), ts)

		if prevRainfall > threshold {
			branches["prev_wet"]++
		} else {
			branches["prev_dry"]++
		}
		if want[0] > 0 {
			branches["today_wet"]++
			if want[0] == threshold {
				branches["amount_floored"]++
			}
		} else {
			branches["today_dry"]++
		}
		if shape < 0.01 {
			branches["shape_floored"]++
		}
		if scale < 0.01 {
			branches["scale_floored"]++
		}
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "rainfall"))
	}
	for _, b := range []string{
		"prev_wet", "prev_dry", "today_wet", "today_dry",
		"amount_floored", "shape_floored", "scale_floored",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeRunoffMatchesBespoke(t *testing.T) {
	bespoke := &RainfallRunoffIteration{}
	declarative, _ := declarativeIteration(t, "runoff")
	// runoff sits at index 1 and reads rainfall at index 0. The bespoke iteration resolves
	// that index from its settings param; the declarative one resolves it from the partition
	// name in the YAML's upstreams block — the two must land on the same partition.
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{Name: "rainfall", Seed: 5},
			{
				Name: "runoff",
				Seed: 0,
				Params: simulator.NewParams(map[string][]float64{
					"upstream_partition": {0},
				}),
			},
		},
	}
	bespoke.Configure(1, settings)
	declarative.Configure(1, settings)

	rng := rand.New(rand.NewPCG(41, 42))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// Soil moisture ranges below zero and well past any field capacity drawn here, so
		// both ends of the saturation clamp, the spill over field capacity, and the
		// zero-floor on the drained store all trigger. Evapotranspiration overlaps the
		// rainfall range so the net-rainfall clip triggers too.
		fieldCapacity := 200.0 + rng.Float64()*300.0
		soilMoisture := rng.Float64()*600.0 - 60.0
		rainfall := rng.Float64() * 40.0
		etRate := rng.Float64() * 10.0
		dt := 0.5 + rng.Float64()
		p := simulator.NewParams(map[string][]float64{
			"field_capacity":      {fieldCapacity},
			"drainage_rate":       {rng.Float64() * 0.1},
			"et_rate":             {etRate},
			"runoff_shape":        {0.5 + rng.Float64()*4.0},
			"fast_recession_rate": {rng.Float64()},
			"slow_recession_rate": {rng.Float64()},
			"catchment_area_km2":  {50.0 + rng.Float64()*500.0},
		})
		state := []float64{
			soilMoisture,
			0.0,
			rng.Float64() * 50.0,
			rng.Float64() * 50.0,
		}
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{
				{
					Values:            mat.NewDense(1, 1, []float64{rainfall}),
					StateWidth:        1,
					StateHistoryDepth: 1,
				},
				{
					Values:            mat.NewDense(1, 4, append([]float64{}, state...)),
					StateWidth:        4,
					StateHistoryDepth: 1,
				},
			}
		}
		ts := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{float64(i)}),
			NextIncrement:     dt,
			CurrentStepNumber: i + 1,
		}

		want := bespoke.Iterate(&p, 1, mk(), ts)
		got := declarative.Iterate(&p, 1, mk(), ts)

		if rainfall > etRate {
			branches["net_rain_positive"]++
		} else {
			branches["net_rain_clipped"]++
		}
		switch {
		case soilMoisture < 0:
			branches["saturation_clamped_low"]++
		case soilMoisture > fieldCapacity:
			branches["saturation_clamped_high"]++
		default:
			branches["saturation_interior"]++
		}
		if soilMoisture+math.Max(rainfall-etRate, 0.0)*dt > fieldCapacity {
			branches["spilled_excess"]++
		} else {
			branches["no_excess"]++
		}
		if want[0] == 0 && soilMoisture < 0 {
			branches["soil_floored_at_zero"]++
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "runoff state"))
		}
	}
	for _, b := range []string{
		"net_rain_positive", "net_rain_clipped",
		"saturation_clamped_low", "saturation_clamped_high", "saturation_interior",
		"spilled_excess", "no_excess", "soil_floored_at_zero",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeFloodRiskAnswersTheSameClaims(t *testing.T) {
	// The oracle is the model's own behaviour suite: every claim recomputed against the
	// declarative build must still hold, and must hold with the same numbers the card
	// reports — not merely point the same way.
	if testing.Short() {
		t.Skip("runs the full claim ensemble twice")
	}
	bespoke := ObservedBehaviour()
	declarative := observedBehaviour(declarativeBuildStub)

	if len(declarative) != len(bespoke) {
		t.Fatalf("got %d claims, want %d", len(declarative), len(bespoke))
	}
	maxDev := 0.0
	for i, claim := range declarative {
		reference := bespoke[i]
		if claim.ID != reference.ID {
			t.Fatalf("claim %d: got ID %q, want %q", i, claim.ID, reference.ID)
		}
		// Still a true claim when the model is data.
		if err := cardgen.Verify(claim); err != nil {
			t.Errorf("claim %q does not hold for the declarative model: %v", claim.ID, err)
			continue
		}
		if len(claim.Observations) != len(reference.Observations) {
			t.Fatalf("claim %q: got %d observations, want %d",
				claim.ID, len(claim.Observations), len(reference.Observations))
		}
		for k, obs := range claim.Observations {
			maxDev = math.Max(maxDev, assertClose(t, obs.Value,
				reference.Observations[k].Value, claim.ID+" / "+obs.Label))
		}
	}
	t.Logf("max deviation across every observation on every claim: %g", maxDev)
}
