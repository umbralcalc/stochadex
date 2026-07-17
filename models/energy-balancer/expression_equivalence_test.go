package energybalancer

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// Two independent kinds of check, because they fail in different ways. The step-for-step
// tests compare single Iterate calls over randomised inputs, one per bespoke iteration
// type, which catches a mis-stated formula exactly. The suite test re-runs the model's own
// claim computations against the declarative build, which catches a model that is subtly
// different in a way per-step agreement would not: wrong wiring, wrong param values, wrong
// state layout.
//
// Every draw in this model is taken by OrnsteinUhlenbeckExactGaussianIteration, which
// samples from rng.New(seed) — the same generator the expression evaluator uses, since
// rng.New(seed) is rand.New(rand.NewPCG(seed, seed)) — and takes exactly one NormFloat64
// per dimension per step. The declarative OU takes exactly one shared(normal(...)) over a
// one-wide field, and sampler.Normal(mu, sigma) is NormFloat64()*sigma + mu, so the two
// streams stay in lockstep and equivalence is decidable directly rather than only in
// distribution. Every other iteration here is deterministic.
//
// Agreement is asserted to a tight tolerance rather than bit-for-bit: compiled Go is free
// to contract a + b*c into a fused multiply-add, which rounds differently from the
// evaluator's separate operations. Deviation at that scale is the FMA, not the model, and
// moves no card claim.

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
//
// The penetration-to-volatility map is BuildStub's own translation of its argument into a
// param — the knob, not the generative core — so it is restated here rather than in the
// YAML. Keeping sigmas the param it acts on is also what lets the behaviour suite's
// volatility overrides reach the declarative model unchanged.
func declarativeBuildStub(
	renewablePenetration float64,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	config := api.LoadApiRunConfigFromYaml("declarative.yaml")
	config.Main.Simulation = simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSizeHours},
		InitTimeValue:    0.0,
	}
	gen := config.Main.GetConfigGenerator()
	gen.GetPartition("residual_demand").Params.Map["sigmas"] = []float64{
		DefaultBaselineVolatilityMW +
			renewablePenetration*DefaultVolatilityPerPenetrationMW,
	}
	gen.GetPartition("residual_demand").Seed = seed
	gen.GetPartition("price_noise").Seed = seed + 1
	gen.GetPartition("carbon_noise").Seed = seed + 2
	return gen
}

// assembly is one generated build of the model, kept whole so that a step-for-step test can
// reach an iteration together with the params and partition indices the generator resolved
// for it.
type assembly struct {
	settings        *simulator.Settings
	implementations *simulator.Implementations
}

// assemble generates configs from a builder. GenerateConfigs configures every iteration, so
// the samplers here are already seeded from the partition seeds.
func assemble(t *testing.T, build stubBuilder, penetration float64, seed uint64) *assembly {
	t.Helper()
	settings, implementations := build(penetration, DefaultNumSteps, seed).GenerateConfigs()
	return &assembly{settings: settings, implementations: implementations}
}

func (a *assembly) index(t *testing.T, name string) int {
	t.Helper()
	for i, iteration := range a.settings.Iterations {
		if iteration.Name == name {
			return i
		}
	}
	t.Fatalf("no partition named %q", name)
	return -1
}

func (a *assembly) iteration(t *testing.T, name string) simulator.Iteration {
	t.Helper()
	return a.implementations.Iterations[a.index(t, name)]
}

// params returns the params the generator resolved for a partition, which for the bespoke
// build is where params_as_partitions has become concrete partition indices.
func (a *assembly) params(t *testing.T, name string) map[string][]float64 {
	t.Helper()
	return a.settings.Iterations[a.index(t, name)].Params.Map
}

// histories builds a fresh, correctly-shaped lag-1 state history for every partition in the
// model, with the named rows filled in. Fresh per call because iterations return slices that
// alias their own history buffers.
func (a *assembly) histories(rows map[string][]float64) []*simulator.StateHistory {
	out := make([]*simulator.StateHistory, len(a.settings.Iterations))
	for i, iteration := range a.settings.Iterations {
		row := make([]float64, iteration.StateWidth)
		if values, ok := rows[iteration.Name]; ok {
			copy(row, values)
		}
		out[i] = &simulator.StateHistory{
			Values:            mat.NewDense(1, iteration.StateWidth, row),
			StateWidth:        iteration.StateWidth,
			StateHistoryDepth: 1,
		}
	}
	return out
}

// timesteps returns a history whose increment is dt, which is what every dt-sensitive
// iteration here reads.
func timesteps(step int, dt float64) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{float64(step) * dt}),
		NextIncrement:     dt,
		CurrentStepNumber: step + 1,
	}
}

// declarativeIteration asserts a partition is expression-backed and returns it, so a test
// that means to exercise the DSL cannot silently be handed a Go iteration.
func declarativeIteration(
	t *testing.T,
	a *assembly,
	partition string,
) *general.ExpressionIteration {
	t.Helper()
	iteration, ok := a.iteration(t, partition).(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("%s is not expression-backed: got %T", partition, a.iteration(t, partition))
	}
	return iteration
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

// assertBranchesExercised fails when a named branch was never reached, since a comparison
// that never entered a branch says nothing about it.
func assertBranchesExercised(t *testing.T, branches map[string]int, names ...string) {
	t.Helper()
	for _, name := range names {
		if branches[name] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", name)
		}
	}
	t.Logf("branches exercised: %v", branches)
}

// TestDeclarativeConfigMatchesBespoke checks the two builds are the same model before any
// per-step comparison: same partitions in the same order, same state layout, same seeds. A
// step-for-step test drives one iteration and so cannot see a partition that is missing,
// misordered or wrongly initialised.
//
// Params are deliberately NOT compared: the declarative build carries no resolved
// *_partition indices (it names its upstreams instead) and carries an actual_dispatch_mask
// the Go build hardcodes as an index. That difference is the point of the exercise.
func TestDeclarativeConfigMatchesBespoke(t *testing.T) {
	bespoke := assemble(t, BuildStub, 0.6, 7)
	declarative := assemble(t, declarativeBuildStub, 0.6, 7)

	if len(declarative.settings.Iterations) != len(bespoke.settings.Iterations) {
		t.Fatalf("got %d partitions, want %d",
			len(declarative.settings.Iterations), len(bespoke.settings.Iterations))
	}
	for i, want := range bespoke.settings.Iterations {
		got := declarative.settings.Iterations[i]
		if got.Name != want.Name {
			t.Fatalf("partition %d: got name %q, want %q", i, got.Name, want.Name)
		}
		if got.StateWidth != want.StateWidth {
			t.Errorf("%s: got state width %d, want %d", want.Name, got.StateWidth, want.StateWidth)
		}
		if got.Seed != want.Seed {
			t.Errorf("%s: got seed %d, want %d", want.Name, got.Seed, want.Seed)
		}
		if len(got.InitStateValues) != len(want.InitStateValues) {
			t.Fatalf("%s: got %d init values, want %d",
				want.Name, len(got.InitStateValues), len(want.InitStateValues))
		}
		for k := range want.InitStateValues {
			assertClose(t, got.InitStateValues[k], want.InitStateValues[k],
				want.Name+" init state")
		}
	}
	// The one within-step edge in the model. Everything else is a lag-1 history read, which
	// the declarative build spells as an upstreams alias rather than a resolved index.
	for _, battery := range []string{"price_battery", "carbon_battery"} {
		got := declarative.settings.Iterations[declarative.index(t, battery)].ParamsFromUpstream
		want := bespoke.settings.Iterations[bespoke.index(t, battery)].ParamsFromUpstream
		if len(got) != len(want) {
			t.Fatalf("%s: got %d upstream params, want %d", battery, len(got), len(want))
		}
		if got["dispatch_mw"].Upstream != want["dispatch_mw"].Upstream {
			t.Errorf("%s: dispatch_mw comes from partition %d, want %d",
				battery, got["dispatch_mw"].Upstream, want["dispatch_mw"].Upstream)
		}
	}
}

// TestDeclarativeOrnsteinUhlenbeckMatchesBespoke drives residual_demand, the OU partition
// the headline driver acts on. The other two OU partitions share this expression by YAML
// anchor and this Go type, so they are the same comparison with different params; the config
// and suite tests cover their wiring.
//
// The theta=0 degenerate case (Brownian motion) is swept even though this model never
// configures it, because the bespoke iteration branches on it and the declarative guard has
// to agree. The conditional-variance clamp at zero is not swept: for theta>0 the variance is
// positive by construction, so the clamp is defensive and unreachable from any params here.
func TestDeclarativeOrnsteinUhlenbeckMatchesBespoke(t *testing.T) {
	const partition = "residual_demand"
	bespokeBuild := assemble(t, BuildStub, 0.6, 3)
	declarativeBuild := assemble(t, declarativeBuildStub, 0.6, 3)

	bespoke := bespokeBuild.iteration(t, partition)
	declarative := declarativeIteration(t, declarativeBuild, partition)
	index := bespokeBuild.index(t, partition)

	rng := rand.New(rand.NewPCG(31, 32))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// theta=0 on a sweep of the cases so the Brownian branch is compared too, and both
		// sides still take exactly one draw, keeping the streams in lockstep.
		theta := 0.0
		if i%4 != 0 {
			theta = rng.Float64() * 3
		}
		mu := rng.Float64() * 30000
		sigma := rng.Float64() * 4000
		x := rng.Float64() * 40000
		dt := 0.1 + rng.Float64()

		values := map[string][]float64{
			"thetas": {theta},
			"mus":    {mu},
			"sigmas": {sigma},
		}
		bespokeParams := simulator.NewParams(values)
		declarativeParams := simulator.NewParams(values)
		rows := map[string][]float64{partition: {x}}
		ts := timesteps(i, dt)

		want := append([]float64{}, bespoke.Iterate(
			&bespokeParams, index, bespokeBuild.histories(rows), ts)...)
		got := declarative.Iterate(
			&declarativeParams, index, declarativeBuild.histories(rows), ts)

		if theta > 0 {
			branches["mean_reverting"]++
		} else {
			branches["brownian"]++
		}
		if len(got) != len(want) {
			t.Fatalf("case %d: width %d, want %d", i, len(got), len(want))
		}
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "residual demand"))
	}
	assertBranchesExercised(t, branches, "mean_reverting", "brownian")
	t.Logf("max deviation: %g", maxDev)
}

// TestDeclarativeStructuralSignalsMatchBespoke drives the price and carbon-intensity
// reduced forms. Both are straight-line, so there are no branches to sweep; what is being
// checked is that each reads the right upstream, since both read residual demand but must
// take their noise from different partitions.
func TestDeclarativeStructuralSignalsMatchBespoke(t *testing.T) {
	bespokeBuild := assemble(t, BuildStub, 0.6, 11)
	declarativeBuild := assemble(t, declarativeBuildStub, 0.6, 11)

	for _, signal := range []struct {
		partition    string
		noise        string
		slopeKey     string
		interceptKey string
	}{
		{"price", "price_noise", "demand_slope", "demand_intercept"},
		{"carbon_intensity", "carbon_noise", "carbon_slope", "carbon_intercept"},
	} {
		t.Run(signal.partition, func(t *testing.T) {
			bespoke := bespokeBuild.iteration(t, signal.partition)
			declarative := declarativeIteration(t, declarativeBuild, signal.partition)
			index := bespokeBuild.index(t, signal.partition)
			resolved := bespokeBuild.params(t, signal.partition)

			rng := rand.New(rand.NewPCG(41, 42))
			maxDev := 0.0

			const cases = 20000
			for i := 0; i < cases; i++ {
				slope := rng.Float64() * 0.03
				intercept := rng.Float64()*200 - 100
				demand := rng.Float64() * 45000
				// Both noise partitions are given a value, so a signal reading the wrong one
				// diverges rather than silently agreeing on a shared number.
				priceNoise := rng.Float64()*20 - 10
				carbonNoise := rng.Float64()*80 - 40

				bespokeParams := simulator.NewParams(map[string][]float64{
					signal.slopeKey:     {slope},
					signal.interceptKey: {intercept},
					"demand_partition":  resolved["demand_partition"],
					"noise_partition":   resolved["noise_partition"],
				})
				declarativeParams := simulator.NewParams(map[string][]float64{
					signal.slopeKey:     {slope},
					signal.interceptKey: {intercept},
				})
				rows := map[string][]float64{
					"residual_demand": {demand},
					"price_noise":     {priceNoise},
					"carbon_noise":    {carbonNoise},
				}
				ts := timesteps(i, stepSizeHours)

				want := bespoke.Iterate(
					&bespokeParams, index, bespokeBuild.histories(rows), ts)
				got := declarative.Iterate(
					&declarativeParams, index, declarativeBuild.histories(rows), ts)

				maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], signal.partition))
			}
			t.Logf("cases: %d, max deviation: %g", cases, maxDev)
		})
	}
}

// TestDeclarativeDispatchMatchesBespoke drives both threshold policies over a signal range
// wide enough that all three arms — discharge, charge and hold — are compared.
func TestDeclarativeDispatchMatchesBespoke(t *testing.T) {
	bespokeBuild := assemble(t, BuildStub, 0.6, 13)
	declarativeBuild := assemble(t, declarativeBuildStub, 0.6, 13)

	for _, policy := range []struct {
		partition   string
		signal      string
		signalParam string
		highKey     string
		lowKey      string
		low, span   float64
	}{
		{"price_dispatch", "price", "price_partition", "price_high", "price_low", 0, 80},
		{"carbon_dispatch", "carbon_intensity", "carbon_partition",
			"carbon_high", "carbon_low", 0, 400},
	} {
		t.Run(policy.partition, func(t *testing.T) {
			bespoke := bespokeBuild.iteration(t, policy.partition)
			declarative := declarativeIteration(t, declarativeBuild, policy.partition)
			index := bespokeBuild.index(t, policy.partition)
			resolved := bespokeBuild.params(t, policy.partition)
			defaults := declarativeBuild.params(t, policy.partition)
			high, low := defaults[policy.highKey][0], defaults[policy.lowKey][0]

			rng := rand.New(rand.NewPCG(51, 52))
			branches := map[string]int{}
			maxDev := 0.0

			const cases = 20000
			for i := 0; i < cases; i++ {
				signal := policy.low + rng.Float64()*policy.span

				bespokeParams := simulator.NewParams(map[string][]float64{
					policy.highKey:     {high},
					policy.lowKey:      {low},
					"power_rating_mw":  {DefaultPowerRatingMW},
					policy.signalParam: resolved[policy.signalParam],
				})
				declarativeParams := simulator.NewParams(map[string][]float64{
					policy.highKey:    {high},
					policy.lowKey:     {low},
					"power_rating_mw": {DefaultPowerRatingMW},
				})
				rows := map[string][]float64{policy.signal: {signal}}
				ts := timesteps(i, stepSizeHours)

				want := bespoke.Iterate(
					&bespokeParams, index, bespokeBuild.histories(rows), ts)
				got := declarative.Iterate(
					&declarativeParams, index, declarativeBuild.histories(rows), ts)

				switch {
				case signal > high:
					branches["discharge"]++
				case signal < low:
					branches["charge"]++
				default:
					branches["hold"]++
				}
				maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], policy.partition))
			}
			assertBranchesExercised(t, branches, "discharge", "charge", "hold")
			t.Logf("max deviation: %g", maxDev)
		})
	}
}

// TestDeclarativeBatteryMatchesBespoke drives the hardest control flow in the model: a power
// clip, a sign branch on the dispatch direction, and a three-way state-of-charge switch that
// back-calculates the dispatch actually achieved.
func TestDeclarativeBatteryMatchesBespoke(t *testing.T) {
	const partition = "price_battery"
	bespokeBuild := assemble(t, BuildStub, 0.6, 17)
	declarativeBuild := assemble(t, declarativeBuildStub, 0.6, 17)

	bespoke := bespokeBuild.iteration(t, partition)
	declarative := declarativeIteration(t, declarativeBuild, partition)
	index := bespokeBuild.index(t, partition)

	rng := rand.New(rand.NewPCG(1, 2))
	capacity := DefaultEnergyCapacityMWh
	branches := map[string]int{}
	maxDev, ulpDiffs := 0.0, 0

	const cases = 20000
	for i := 0; i < cases; i++ {
		soc := rng.Float64() * capacity
		// Deliberately over-range so the power clip and both SoC limits all trigger.
		dispatch := (rng.Float64()*4 - 2) * DefaultPowerRatingMW
		dt := 0.25 + rng.Float64()

		values := map[string][]float64{
			"dispatch_mw":          {dispatch},
			"energy_capacity_mwh":  {capacity},
			"power_rating_mw":      {DefaultPowerRatingMW},
			"charge_efficiency":    {DefaultChargeEfficiency},
			"discharge_efficiency": {DefaultDischargeEfficiency},
			"min_soc_fraction":     {DefaultMinSoCFraction},
			"max_soc_fraction":     {DefaultMaxSoCFraction},
		}
		bespokeParams := simulator.NewParams(values)
		declarativeParams := simulator.NewParams(values)
		rows := map[string][]float64{partition: {soc, 0}}
		ts := timesteps(i, dt)

		want := bespoke.Iterate(&bespokeParams, index, bespokeBuild.histories(rows), ts)
		got := declarative.Iterate(&declarativeParams, index, declarativeBuild.histories(rows), ts)

		if math.Abs(dispatch) > DefaultPowerRatingMW {
			branches["power_clip"]++
		}
		if dispatch >= 0 {
			branches["discharging"]++
		} else {
			branches["charging"]++
		}
		switch {
		case want[0] <= DefaultMinSoCFraction*capacity+1e-12:
			branches["soc_floor"]++
		case want[0] >= DefaultMaxSoCFraction*capacity-1e-12:
			branches["soc_ceiling"]++
		default:
			branches["normal"]++
		}

		if len(got) != len(want) {
			t.Fatalf("case %d: width %d, want %d", i, len(got), len(want))
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "battery"))
			if got[k] != want[k] {
				ulpDiffs++
			}
		}
	}
	assertBranchesExercised(t, branches,
		"normal", "power_clip", "soc_floor", "soc_ceiling", "charging", "discharging")
	t.Logf("max deviation: %g over %d comparisons", maxDev, cases*2)
	t.Logf("outputs differing only at ULP scale (FMA contraction): %d", ulpDiffs)
}

// TestDeclarativeAccumulatorsMatchBespoke drives the three accumulators that read the
// battery's achieved dispatch out of its state. All three are where the Go hardcodes the
// battery's state index 1, and the declarative build states it as actual_dispatch_mask, so
// this is the test that mask has to earn.
func TestDeclarativeAccumulatorsMatchBespoke(t *testing.T) {
	bespokeBuild := assemble(t, BuildStub, 0.6, 19)
	declarativeBuild := assemble(t, declarativeBuildStub, 0.6, 19)

	for _, accumulator := range []struct {
		partition string
		extra     []string
	}{
		{"price_efc", []string{"energy_capacity_mwh"}},
		{"price_revenue", nil},
		{"price_co2_saved", nil},
	} {
		t.Run(accumulator.partition, func(t *testing.T) {
			bespoke := bespokeBuild.iteration(t, accumulator.partition)
			declarative := declarativeIteration(t, declarativeBuild, accumulator.partition)
			index := bespokeBuild.index(t, accumulator.partition)
			resolved := bespokeBuild.params(t, accumulator.partition)
			defaults := declarativeBuild.params(t, accumulator.partition)

			rng := rand.New(rand.NewPCG(61, 62))
			branches := map[string]int{}
			maxDev := 0.0

			const cases = 20000
			for i := 0; i < cases; i++ {
				// Straddles zero so the discharge-only carbon credit and the revenue sign
				// are both exercised on either side.
				actualDispatch := rng.Float64()*2*DefaultPowerRatingMW - DefaultPowerRatingMW
				soc := rng.Float64() * DefaultEnergyCapacityMWh
				price := rng.Float64()*100 - 20
				carbon := rng.Float64() * 400
				previous := rng.Float64() * 100
				dt := 0.25 + rng.Float64()

				bespokeValues := map[string][]float64{}
				declarativeValues := map[string][]float64{}
				for _, key := range accumulator.extra {
					bespokeValues[key] = resolved[key]
					declarativeValues[key] = defaults[key]
				}
				for _, key := range []string{
					"battery_partition", "price_partition", "carbon_partition",
				} {
					if value, ok := resolved[key]; ok {
						bespokeValues[key] = value
					}
				}
				declarativeValues["actual_dispatch_mask"] = defaults["actual_dispatch_mask"]

				bespokeParams := simulator.NewParams(bespokeValues)
				declarativeParams := simulator.NewParams(declarativeValues)
				rows := map[string][]float64{
					"price_battery":       {soc, actualDispatch},
					"price":               {price},
					"carbon_intensity":    {carbon},
					accumulator.partition: {previous},
				}
				ts := timesteps(i, dt)

				want := bespoke.Iterate(
					&bespokeParams, index, bespokeBuild.histories(rows), ts)
				got := declarative.Iterate(
					&declarativeParams, index, declarativeBuild.histories(rows), ts)

				if actualDispatch >= 0 {
					branches["discharging"]++
				} else {
					branches["charging"]++
				}
				maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], accumulator.partition))
			}
			assertBranchesExercised(t, branches, "discharging", "charging")
			t.Logf("max deviation: %g", maxDev)
		})
	}
}

func TestDeclarativeEnergyBalancerAnswersTheSameClaims(t *testing.T) {
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
