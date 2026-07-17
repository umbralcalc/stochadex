package energybalancer

// Corpus validation for general.ExpressionIteration: can a real bespoke catalogue iteration
// be re-expressed declaratively, as data, with no Go?
//
// BatteryIteration is the hardest control flow in this catalogue (a power clip, a sign
// branch, and a three-way state-of-charge switch that back-calculates the dispatch actually
// achieved) but it is deterministic, so equivalence is decidable by direct comparison rather
// than by distribution matching.
//
// Note this asserts a tight relative tolerance rather than bit-identity: the compiled Go is
// free to contract `soc + (-dispatch*dt*eff)` into a fused multiply-add, which rounds
// differently from the evaluator's separate operations. Deviation at that scale is the FMA,
// not the model, and moves no card claim.

import (
	"math"
	"math/rand/v2"
	"testing"

	"gonum.org/v1/gonum/mat"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// declarativeBattery is BatteryIteration expressed entirely as data.
func declarativeBattery() *general.ExpressionIteration {
	return &general.ExpressionIteration{
		Fields: []general.ExpressionField{{Name: "soc"}, {Name: "actual_dispatch"}},
		Bindings: []general.ExpressionBinding{
			{Name: "min_soc", Expr: "min_soc_fraction * energy_capacity_mwh"},
			{Name: "max_soc", Expr: "max_soc_fraction * energy_capacity_mwh"},
			{Name: "dispatch", Expr: "clamp(dispatch_mw, -power_rating_mw, power_rating_mw)"},
			{Name: "energy_delta", Expr: "where(dispatch >= 0, " +
				"-dispatch * dt / discharge_efficiency, " +
				"-dispatch * dt * charge_efficiency)"},
			{Name: "soc_raw", Expr: "soc + energy_delta"},
			{Name: "new_soc", Expr: "clamp(soc_raw, min_soc, max_soc)"},
			{Name: "actual", Expr: "where(soc_raw < min_soc, " +
				"(soc - min_soc) * discharge_efficiency / dt, " +
				"where(soc_raw > max_soc, " +
				"-(max_soc - soc) / (charge_efficiency * dt), dispatch))"},
		},
		Outputs: []string{"new_soc", "actual"},
	}
}

func TestDeclarativeBatteryMatchesBespoke(t *testing.T) {
	bespoke := &BatteryIteration{}
	declarative := declarativeBattery()
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "battery", Seed: 0}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

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

		params := simulator.NewParams(map[string][]float64{
			"dispatch_mw":          {dispatch},
			"energy_capacity_mwh":  {capacity},
			"power_rating_mw":      {DefaultPowerRatingMW},
			"charge_efficiency":    {DefaultChargeEfficiency},
			"discharge_efficiency": {DefaultDischargeEfficiency},
			"min_soc_fraction":     {DefaultMinSoCFraction},
			"max_soc_fraction":     {DefaultMaxSoCFraction},
		})
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{{
				Values:            mat.NewDense(1, 2, []float64{soc, 0}),
				StateWidth:        2,
				StateHistoryDepth: 1,
			}}
		}
		ts := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{float64(i) * dt}),
			NextIncrement:     dt,
			CurrentStepNumber: i + 1,
		}

		want := bespoke.Iterate(&params, 0, mk(), ts)
		got := declarative.Iterate(&params, 0, mk(), ts)

		if math.Abs(dispatch) > DefaultPowerRatingMW {
			branches["power_clip"]++
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
			d := math.Abs(got[k] - want[k])
			if s := math.Abs(want[k]); s > 1 {
				d /= s
			}
			if d > maxDev {
				maxDev = d
			}
			if d > 1e-12 {
				t.Fatalf("case %d field %d: declarative=%v bespoke=%v rel-dev=%g "+
					"(soc=%v dispatch=%v dt=%v)",
					i, k, got[k], want[k], d, soc, dispatch, dt)
			}
			if got[k] != want[k] {
				ulpDiffs++
			}
		}
	}
	for _, b := range []string{"normal", "power_clip", "soc_floor", "soc_ceiling"} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max relative deviation: %g over %d comparisons", maxDev, cases*2)
	t.Logf("outputs differing only at ULP scale (FMA contraction): %d", ulpDiffs)
}
