package floodrisk

import "github.com/umbralcalc/stochadex/pkg/simulator"

// Default generative parameters for the flagship scenario: the Upper Calder
// Valley (West Yorkshire). These are illustrative calibrated values, NOT a live
// posterior — data ingestion and calibration against gauge records live in the
// downstream repo (see card.md). They exist so the stub runs with zero external
// inputs.
const (
	// Rainfall generator (two-state Markov + Gamma wet-day amounts).
	DefaultWetDayShape  = 0.609
	DefaultWetDayScale  = 7.944
	DefaultPWetGivenDry = 0.40
	DefaultPWetGivenWet = 0.83
	DefaultWetThreshold = 0.1

	// PDM rainfall-runoff (single lumped sub-catchment).
	DefaultFieldCapacity     = 330.0
	DefaultDrainageRate      = 0.03
	DefaultETRate            = 1.1
	DefaultRunoffShape       = 2.7
	DefaultFastRecessionRate = 0.40
	DefaultSlowRecessionRate = 0.32
	DefaultCatchmentAreaKm2  = 297.0

	// InitialSoilMoisture (mm) seeds the soil store below field capacity.
	InitialSoilMoisture = 100.0

	// DefaultNumSteps is the flagship horizon in days (10 years).
	DefaultNumSteps = 3650

	// Partition indices within the stub.
	rainfallPartition = 0
	runoffPartition   = 1
)

// BuildStub constructs the data-free generative core of the flood-risk model: a
// stochastic daily-rainfall generator drives a PDM-style rainfall-runoff cascade
// producing river flow. The rainfallMultiplier is the climate-perturbation knob
// (1.0 = baseline; 1.2 ≈ a UKCP18 +20% wet-day intensity scenario) — exactly the
// parameter the CI test sweeps to check the model's headline claim: wetter
// forcing produces higher flood peak flows.
//
// This is the generative core only — no data ingestion, no calibration, no NFM
// intervention/decision layer.
//
// Partitions (declaration order):
//
//	rainfall  two-state Markov (wet/dry) + Gamma wet-day amounts → daily rainfall
//	runoff    PDM nonlinear runoff + parallel fast/slow stores → river flow
func BuildStub(rainfallMultiplier float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	rainfall := &simulator.PartitionConfig{
		Name:      "rainfall",
		Iteration: &StochasticRainfallIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"wet_day_shape":       {DefaultWetDayShape},
			"wet_day_scale":       {DefaultWetDayScale},
			"p_wet_given_dry":     {DefaultPWetGivenDry},
			"p_wet_given_wet":     {DefaultPWetGivenWet},
			"rainfall_multiplier": {rainfallMultiplier},
			"wet_threshold":       {DefaultWetThreshold},
		}),
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              seed,
	}

	runoff := &simulator.PartitionConfig{
		Name:      "runoff",
		Iteration: &RainfallRunoffIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"field_capacity":      {DefaultFieldCapacity},
			"drainage_rate":       {DefaultDrainageRate},
			"et_rate":             {DefaultETRate},
			"runoff_shape":        {DefaultRunoffShape},
			"fast_recession_rate": {DefaultFastRecessionRate},
			"slow_recession_rate": {DefaultSlowRecessionRate},
			"catchment_area_km2":  {DefaultCatchmentAreaKm2},
			"upstream_partition":  {rainfallPartition},
		}),
		InitStateValues:   []float64{InitialSoilMoisture, 0.0, 0.0, 0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	}

	gen := simulator.NewConfigGenerator()
	gen.SetPartition(rainfall)
	gen.SetPartition(runoff)
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	return gen
}
