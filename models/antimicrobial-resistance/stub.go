package amr

import "github.com/umbralcalc/stochadex/pkg/simulator"

// Default generative parameters for the flagship scenario. These are the
// data-free defaults (a plausible hospital regime), NOT calibrated posteriors —
// calibration against real STATS/surveillance data lives in the downstream repo
// (see card.md). They exist so the stub runs with zero external inputs.
const (
	DefaultCommunitySusceptiblePrevalence = 0.15
	DefaultCommunityResistantPrevalence   = 0.1164
	DefaultTurnoverRate                   = 0.1
	DefaultTransmissionRate               = 0.0384
	DefaultSelectionCoefficient           = 0.1351
	DefaultFitnessCost                    = 0.0170
	DefaultNoiseScale                     = 0.01

	DefaultInfectionProbability = 0.01
	DefaultPatientPopulation    = 500.0

	// BaselinePrescribingRate is the constant cephalosporin prescribing rate of
	// the "do nothing" policy in the downstream dashboard.
	BaselinePrescribingRate = 0.3

	// DefaultNumSteps is the flagship horizon (days), matching the downstream
	// dashboard's SimSteps.
	DefaultNumSteps = 200
)

// BuildStub constructs the data-free generative core of the antimicrobial-
// resistance model: a constant cephalosporin prescribing pressure drives a
// two-strain colonisation SDE, which in turn drives a Poisson bloodstream-
// infection process.
//
// This is the generative core only — no data ingestion, no inference, no policy
// or decision layer. Prescribing enters as a single constant (prescribingRate)
// rather than an upstream policy partition, which is exactly the knob the CI test
// sweeps to check the model's headline claim: more cephalosporin selection
// pressure produces a larger resistant colonisation fraction.
//
// Partitions (declaration order):
//
//	colonisation  two-strain S/R SDE on hospital colonisation fractions
//	infection     Poisson BSI counts driven by colonisation
func BuildStub(prescribingRate float64, numSteps int) *simulator.ConfigGenerator {
	colonisation := &simulator.PartitionConfig{
		Name:      "colonisation",
		Iteration: &ColonisationDynamicsIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"community_susceptible_prevalence": {DefaultCommunitySusceptiblePrevalence},
			"community_resistant_prevalence":   {DefaultCommunityResistantPrevalence},
			"turnover_rate":                    {DefaultTurnoverRate},
			"transmission_rate":                {DefaultTransmissionRate},
			"selection_coefficient":            {DefaultSelectionCoefficient},
			"fitness_cost":                     {DefaultFitnessCost},
			"noise_scale":                      {DefaultNoiseScale},
			// Constant prescribing pressure — no upstream policy partition, so the
			// iteration reads prescribing_rate directly.
			"prescribing_rate": {prescribingRate},
		}),
		InitStateValues:   []float64{0.15, 0.05},
		StateHistoryDepth: 1,
		Seed:              9182,
	}

	infection := &simulator.PartitionConfig{
		Name:      "infection",
		Iteration: &InfectionProcessIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"infection_probability": {DefaultInfectionProbability},
			"patient_population":    {DefaultPatientPopulation},
		}),
		// infection reads the colonisation partition's state history; referenced
		// by name so the index survives reordering (and is visible to pkg/graph).
		ParamsAsPartitions: map[string][]string{
			"colonisation_partition": {"colonisation"},
		},
		InitStateValues:   []float64{0.0, 0.0},
		StateHistoryDepth: 1,
		Seed:              3347,
	}

	gen := simulator.NewConfigGenerator()
	gen.SetPartition(colonisation)
	gen.SetPartition(infection)
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
