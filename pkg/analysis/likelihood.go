package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AppliedLikelihoodComparison is the base configuration for a rolling
// comparison between a referenced dataset and referenced likelihood model.
type AppliedLikelihoodComparison struct {
	Name  string
	Data  DataRef
	Model inference.LikelihoodDistribution
}

// NewLikelihoodComparisonPartition creates a new PartitionConfig for
// a rolling likelihood comparison.
func NewLikelihoodComparisonPartition(
	applied AppliedLikelihoodComparison,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := simulator.NewConfigGenerator()
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "",
		Iteration: &inference.DataComparisonIteration{
			Likelihood: applied.Model,
		},
	})
	return &simulator.PartitionConfig{
		Name: "",
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
