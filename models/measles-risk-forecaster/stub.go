package measles

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Default generative parameters for the flagship scenario: a synthetic set of local
// authorities (UTLAs) sharing one national measles importation latent. These are
// illustrative values chosen so the generative core runs with zero external inputs —
// NOT calibrated posteriors. In the downstream measles-risk-forecaster repo the
// susceptibility surface is built from COVER MMR coverage (CAR-smoothed over the ONS
// adjacency graph), the reachable-cluster size and importation band are calibrated
// against observed UTLA outbreak totals under a censored likelihood, and R0 carries
// its literature uncertainty (~12–18); see card.md and
// https://github.com/umbralcalc/measles-risk-forecaster.
//
// The coverage defaults sit deliberately below the ~95% herd-immunity threshold so
// the swept driver (vaccine coverage) moves R_local = R0·s across the critical value
// of 1, which is exactly the transition the CI test checks.
const (
	// Basic reproduction number (mid of the measles 12–18 range) and negative-
	// binomial offspring dispersion (smaller k = more superspreading).
	DefaultR0         = 15.0
	DefaultDispersion = 0.5

	// Reachable susceptible community for one introduction (school/neighbourhood
	// scale, ~400 from the downstream calibration) and the fraction of the nominal
	// pool s·N that is well-mixed (1.0 = homogeneous-mixing upper bound).
	DefaultReachableCluster = 400.0
	DefaultPoolFraction     = 1.0

	// National seed-total band M ~ log-uniform(seed_low, seed_high): the wide
	// importation-pressure uncertainty. A wider/higher band seeds more UTLAs at once.
	DefaultSeedLow  = 20.0
	DefaultSeedHigh = 120.0

	// Synthetic UTLA surface. Coverage is spread ±DefaultCoverageSpread (MMR2) around
	// the swept central value across DefaultNumUTLAs areas; MMR1 sits DefaultDoseGap
	// above MMR2. DefaultMMR2Coverage is the illustrative baseline for the one swept
	// driver (BuildStub takes it as its argument).
	DefaultNumUTLAs       = 30
	DefaultMMR2Coverage   = 0.88
	DefaultCoverageSpread = 0.06
	DefaultDoseGap        = 0.05

	// DefaultMaxGenerations is the flagship horizon: generations of the branching
	// process (enough for supercritical outbreaks to reach the reachable-cluster cap
	// or go extinct).
	DefaultMaxGenerations = 14
)

// clamp01 keeps a coverage fraction inside a sane [0.5, 0.999] band.
func clamp01(x float64) float64 {
	if x < 0.5 {
		return 0.5
	}
	if x > 0.999 {
		return 0.999
	}
	return x
}

// BuildUTLASurface constructs the synthetic per-UTLA vectors (susceptibility,
// receptivity, susceptible pool) for a given central MMR2 coverage. Coverage is
// spread deterministically across areas so some sit above and some below the
// herd-immunity threshold; population is scrambled independently of coverage so
// receptivity does not spuriously track susceptibility.
func BuildUTLASurface(mmr2Coverage float64) (susceptibility, receptivity, pool []float64) {
	n := DefaultNumUTLAs
	susceptibility = make([]float64, n)
	receptivity = make([]float64, n)
	pool = make([]float64, n)
	pops := make([]float64, n)
	total := 0.0
	for i := 0; i < n; i++ {
		frac := 0.0
		if n > 1 {
			frac = float64(i) / float64(n-1)
		}
		c2 := clamp01(mmr2Coverage + DefaultCoverageSpread*(2*frac-1))
		c1 := math.Min(0.999, c2+DefaultDoseGap)
		susceptibility[i] = SusceptibilityFromCoverage(c1, c2, DefaultMMR1Efficacy, DefaultMMR2Efficacy)
		// Population scrambled independently of coverage (120k..380k).
		popFrac := float64((i*13)%n) / float64(n-1)
		pops[i] = 120000.0 + 260000.0*popFrac
		total += pops[i]
		pool[i] = effectivePool(susceptibility[i], pops[i], DefaultPoolFraction, DefaultReachableCluster)
	}
	for i := range receptivity {
		receptivity[i] = pops[i] / total // importation receptivity = population share
	}
	return susceptibility, receptivity, pool
}

// BuildStub constructs the data-free generative core of the measles transmission-
// risk model: a shared national importation latent (partition 0) draws a seed total
// M per scenario, and a joint outbreak partition (partition 1) seeds every UTLA with
// Poisson(M·receptivity) and branches each one under susceptible depletion at its own
// R_local = R0·s. The shared M correlates the UTLAs, so outbreaks co-occur.
//
// This is the generative core only — no COVER data ingestion, no CAR spatial
// smoothing, no censored-likelihood calibration, no reporting-lag nowcast, and no
// risk-ranking decision layer. The one scientifically-interesting driver,
// mmr2Coverage (central two-dose MMR coverage), is exactly the parameter the CI test
// sweeps to check the model's headline claim: lower coverage raises effective
// susceptibility, pushes R_local above 1 in more areas, and so raises the total
// simulated case count.
//
// Partitions (declaration order):
//
//	national_importation  shared national seed total M ~ logUniform(seed_low, seed_high)
//	outbreaks             all UTLAs jointly: [infectious_1..N, cumulative_1..N] (width 2N)
func BuildStub(mmr2Coverage float64, maxGenerations int, seed uint64) *simulator.ConfigGenerator {
	susceptibility, receptivity, pool := BuildUTLASurface(mmr2Coverage)
	n := len(susceptibility)

	gen := simulator.NewConfigGenerator()

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "national_importation",
		Iteration: &NationalImportationIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"seed_low":  {DefaultSeedLow},
			"seed_high": {DefaultSeedHigh},
		}),
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 2,
		Seed:              seed,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "outbreaks",
		Iteration: &JointOutbreakIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"susceptibility":      susceptibility,
			"receptivity":         receptivity,
			"susceptible_pool":    pool,
			"r0":                  {DefaultR0},
			"dispersion":          {DefaultDispersion},
			"national_seed_total": {0.0},
		}),
		// The shared national total feeds this step's seeding within the step.
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"national_seed_total": {Upstream: "national_importation"},
		},
		InitStateValues:   make([]float64, 2*n),
		StateHistoryDepth: 2,
		Seed:              seed + 7919, // distinct random stream
	})

	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: maxGenerations,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	return gen
}
