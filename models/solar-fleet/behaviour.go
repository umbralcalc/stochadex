package solarfleet

// Observed structural-driver behaviour of the fleet model — the shared source of
// both the behaviour_test.go assertions and the card's generated numbers. Six
// monotonic response claims: three stochastic (they run the full-covariance forward
// model — cloud volatility, geographic dispersion, mean-reversion speed all act on
// the variability of aggregate fleet output) and three deterministic geometry
// claims (tilt, orientation, latitude act on output shares).
//
// This stub is purely structural: its decision layer (capacity siting under
// output-variability risk) lives entirely downstream, so — like floodrisk — it has
// no in-stub actionable levers and is instead comprehensive on structural drivers.
// The flagship is wider_geographic_dispersion_lowers_fleet_variability, non-vacuous
// precisely because coupling derives from geography (the full-covariance form).

import (
	"sync"
	"time"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat"
)

const (
	behaviourSteps   = 400 // ~8 days at 30-min steps
	behaviourBurn    = 100 // discard OU spin-up before measuring variability
	behaviourMembers = 8   // ensemble size for the noisy stochastic claims
)

// stubBuilder matches BuildStub's signature so the declarative build can be swapped
// in for the whole-suite equivalence check.
type stubBuilder func(cloudVolatility float64, numSteps int, seed uint64) *simulator.ConfigGenerator

// runStub builds, optionally overrides, and runs the model, returning the store.
func runStub(
	build stubBuilder,
	cloudVolatility float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := build(cloudVolatility, numSteps, seed)
	if override != nil {
		override(gen)
	}
	settings, implementations := gen.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// setReversion overrides the shared OU reversion speed (theta) on the sites partition.
func setReversion(theta float64) func(*simulator.ConfigGenerator) {
	return func(g *simulator.ConfigGenerator) {
		g.GetPartition("sites").Params.Map["theta"] = []float64{theta}
	}
}

// useFleet re-points the model at a different fleet without changing its structure:
// the site set enters only through the Cholesky factor and the clear-sky driver,
// both carried as data, so a compact fleet is these two overrides and nothing else.
func useFleet(sites []Site, cloudVolatility float64, numSteps int) func(*simulator.ConfigGenerator) {
	return func(g *simulator.ConfigGenerator) {
		kernel := DefaultKernel(cloudVolatility)
		g.GetPartition("sites").Params.Map["lflat"] = CholeskyFactorFlat(sites, kernel)
		poaRows := ClearSkyDriverRows(sites, numSteps)
		clearsky := g.GetPartition("clearsky")
		clearsky.Iteration = &general.FromStorageIteration{Data: poaRows}
		clearsky.InitStateValues = append([]float64(nil), poaRows[0]...)
	}
}

// fleetIndexVariability is the std of the capacity-weighted aggregate fleet
// clear-sky index over daytime steps after burn-in. Normalising the fleet's
// generation by its clear-sky potential (read from the replayed clearsky partition)
// removes the deterministic daily envelope, isolating cloud-driven variability —
// which is exactly what the coupling structure governs.
func fleetIndexVariability(store *simulator.StateTimeStorage) float64 {
	sites := store.GetValues("sites")
	clear := store.GetValues("clearsky")
	s := len(sites[0]) / 2
	aggK := make([]float64, 0, len(sites))
	for step := behaviourBurn; step < len(sites); step++ {
		fleetGen, fleetClear := 0.0, 0.0
		for i := 0; i < s; i++ {
			fleetGen += sites[step][s+i]
			fleetClear += DefaultKwp * clear[step][i] / IStandardTestConditions * DefaultEta
		}
		if fleetClear > 0.5 { // daytime, non-trivial sun
			aggK = append(aggK, fleetGen/fleetClear)
		}
	}
	return stat.StdDev(aggK, nil)
}

// fleetIndexVariabilityEnsemble averages the variability over an ensemble of seeds,
// so each claim is a statement about the distribution rather than one realisation.
func fleetIndexVariabilityEnsemble(
	build stubBuilder,
	cloudVolatility float64,
	seedBase uint64,
	override func(*simulator.ConfigGenerator),
) float64 {
	total := 0.0
	for m := 0; m < behaviourMembers; m++ {
		store := runStub(build, cloudVolatility, behaviourSteps, seedBase+uint64(m), override)
		total += fleetIndexVariability(store)
	}
	return total / behaviourMembers
}

// --- deterministic geometry metrics (independent of the stochastic build) --------

// seasonalOutput integrates clear-sky POA over one day (48 half-hour steps) at
// longitude 0 — a proxy for a site's daily clear-sky energy.
func seasonalOutput(latitude, tilt, azimuth float64, day time.Time) float64 {
	total := 0.0
	for step := 0; step < 48; step++ {
		when := day.Add(time.Duration(step) * 30 * time.Minute)
		total += ClearSkyPOA(latitude, 0.0, when, tilt, azimuth)
	}
	return total
}

func day(year int, month time.Month) time.Time {
	return time.Date(year, month, 21, 0, 0, 0, 0, time.UTC)
}

func winterOutputShare(tilt float64) float64 {
	winter := seasonalOutput(51.5, tilt, 180.0, day(2023, time.December))
	summer := seasonalOutput(51.5, tilt, 180.0, day(2023, time.June))
	return winter / (winter + summer)
}

func annualOutput(azimuth float64) float64 {
	total := 0.0
	for m := time.January; m <= time.December; m++ {
		total += seasonalOutput(51.5, 35.0, azimuth, day(2023, m))
	}
	return total
}

func summerWinterRatio(latitude float64) float64 {
	summer := seasonalOutput(latitude, 35.0, 180.0, day(2023, time.June))
	winter := seasonalOutput(latitude, 35.0, 180.0, day(2023, time.December))
	if winter < 1e-6 {
		winter = 1e-6
	}
	return summer / winter
}

// --- the claims ------------------------------------------------------------------

// ObservedBehaviour is the claim set against the Go stub; observedBehaviour(build)
// lets the equivalence suite re-run the stochastic claims against the twin.
func ObservedBehaviour() []cardgen.Claim {
	return observedBehaviour(BuildStub)
}

func observedBehaviour(build stubBuilder) []cardgen.Claim {
	// 1) cloud volatility -> fleet variability (up).
	volLo := fleetIndexVariabilityEnsemble(build, 0.5, 1000, nil)
	volHi := fleetIndexVariabilityEnsemble(build, 1.0, 1000, nil)
	claimVol := cardgen.Claim{
		ID:        "higher_cloud_volatility_raises_fleet_variability",
		Statement: "Higher sky volatility raises the variability of aggregate fleet output",
		Unit:      "std of aggregate clear-sky index",
		Monotone:  +1,
		Observations: []cardgen.Observation{
			{Label: "sigma 0.5", Value: volLo},
			{Label: "sigma 1.0", Value: volHi},
		},
	}

	// 2) geographic dispersion -> fleet variability (down) — the flagship.
	compactV := fleetIndexVariabilityEnsemble(build, DefaultCloudVolatility, 2000,
		useFleet(CompactFleet(), DefaultCloudVolatility, behaviourSteps))
	dispersedV := fleetIndexVariabilityEnsemble(build, DefaultCloudVolatility, 2000, nil)
	claimDisp := cardgen.Claim{
		ID:        "wider_geographic_dispersion_lowers_fleet_variability",
		Statement: "Spreading sites apart lowers aggregate output variability at fixed capacity",
		Unit:      "std of aggregate clear-sky index",
		Monotone:  -1,
		Observations: []cardgen.Observation{
			{Label: "compact fleet", Value: compactV},
			{Label: "dispersed fleet", Value: dispersedV},
		},
	}

	// 3) mean-reversion speed -> fleet variability (down).
	revSlow := fleetIndexVariabilityEnsemble(build, DefaultCloudVolatility, 3000, setReversion(0.15))
	revFast := fleetIndexVariabilityEnsemble(build, DefaultCloudVolatility, 3000, setReversion(0.6))
	claimRev := cardgen.Claim{
		ID:        "faster_cloud_reversion_lowers_output_variability",
		Statement: "Faster mean-reversion of the cloud field lowers output variability",
		Unit:      "std of aggregate clear-sky index",
		Monotone:  -1,
		Observations: []cardgen.Observation{
			{Label: "theta 0.15", Value: revSlow},
			{Label: "theta 0.6", Value: revFast},
		},
	}

	// 4) tilt -> winter output share (up).
	claimTilt := cardgen.Claim{
		ID:        "steeper_tilt_shifts_output_toward_winter",
		Statement: "Increasing panel tilt raises the winter-season output share",
		Unit:      "winter output share",
		Monotone:  +1,
		Observations: []cardgen.Observation{
			{Label: "tilt 20", Value: winterOutputShare(20.0)},
			{Label: "tilt 50", Value: winterOutputShare(50.0)},
		},
	}

	// 5) orientation toward due south -> annual output (up).
	claimOrient := cardgen.Claim{
		ID:        "southward_orientation_raises_annual_output",
		Statement: "Orientation closer to due south raises annual output",
		Unit:      "annual clear-sky POA proxy",
		Monotone:  +1,
		Observations: []cardgen.Observation{
			{Label: "azimuth 120", Value: annualOutput(120.0)},
			{Label: "azimuth 180", Value: annualOutput(180.0)},
		},
	}

	// 6) latitude -> summer/winter ratio (up).
	claimLat := cardgen.Claim{
		ID:        "higher_latitude_widens_summer_winter_ratio",
		Statement: "Higher latitude widens the summer/winter output ratio",
		Unit:      "summer/winter output ratio",
		Monotone:  +1,
		Observations: []cardgen.Observation{
			{Label: "latitude 45", Value: summerWinterRatio(45.0)},
			{Label: "latitude 60", Value: summerWinterRatio(60.0)},
		},
	}

	return []cardgen.Claim{
		claimVol, claimDisp, claimRev, claimTilt, claimOrient, claimLat,
	}
}
