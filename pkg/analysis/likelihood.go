package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AppliedLikelihoodComparison is the base configuration for a rolling
// comparison between a referenced dataset and referenced likelihood model.
type AppliedLikelihoodComparison struct {
	Name       string
	Data       DataRef
	Model      inference.LikelihoodDistribution
	WindowSize int
}

// NewLikelihoodComparisonPartition creates a new PartitionConfig for
// a rolling likelihood comparison.
func NewLikelihoodComparisonPartition(
	applied AppliedLikelihoodComparison,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.WindowSize,
		},
		// TODO: Find a way to pass in the correct times from the data!
		// TimestepFunction: &simulator.ConstantTimestepFunction{},
		// InitTimeValue: 0.0,
	})
	params := simulator.NewParams(map[string][]float64{
		"cumulative":    {1},
		"burn_in_steps": {float64(applied.WindowSize)},
	})
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "comparison",
		Iteration: &inference.DataComparisonIteration{
			Likelihood: applied.Model,
		},
		Params:            params,
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			generator.GenerateConfigs(),
		),
	}
}

// AppliedSimulationInference is the base configuration for an online
// inference of a simulation (specified by partition configs) from a
// referenced dataset.
type AppliedSimulationInference struct {
	Name       string
	Data       DataRef
	Simulation []*simulator.PartitionConfig
}

// NewSimulationInferencePartition creates a new PartitionConfig for
// an online simulation inference.
func NewSimulationInferencePartition(
	applied AppliedSimulationInference,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := simulator.NewConfigGenerator()
	generator.SetPartition(&simulator.PartitionConfig{
		Name:      "",
		Iteration: &inference.DataComparisonIteration{},
	})
	return &simulator.PartitionConfig{
		Name: "",
		Iteration: general.NewEmbeddedSimulationRunIteration(
			generator.GenerateConfigs(),
		),
	}
}
