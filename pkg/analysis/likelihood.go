package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// WindowedPartitionsData defines a windowed history of data from
// partitions in storage.
type WindowedPartitionsData struct {
	Partitions []DataRef
	Depth      int
}

// AppliedLikelihoodComparison is the base configuration for a rolling
// comparison between a referenced dataset and referenced likelihood model.
type AppliedLikelihoodComparison struct {
	Name  string
	Data  WindowedPartitionsData
	Model inference.LikelihoodDistribution
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
			MaxNumberOfSteps: applied.Data.Depth,
		},
		// These will be overwritten with the times in the data...
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	simInitStateValues := make([]float64, 0)
	simParamsAsPartitions := make(map[string][]string)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	for _, ref := range applied.Data.Partitions {
		initStateValues := ref.GetFromStorage(storage)[0]
		generator.SetPartition(&simulator.PartitionConfig{
			Name:              ref.PartitionName,
			Iteration:         &general.FromHistoryIteration{},
			Params:            simulator.NewParams(make(map[string][]float64)),
			InitStateValues:   initStateValues,
			StateHistoryDepth: 1,
			Seed:              0,
		})
		simInitStateValues = append(simInitStateValues, initStateValues...)
		simParamsAsPartitions[ref.PartitionName+"/state_memory_partition"] =
			[]string{ref.PartitionName}
		simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
			simulator.NamedUpstreamConfig{Upstream: ref.PartitionName}
	}
	params := simulator.NewParams(map[string][]float64{
		"cumulative":    {1},
		"burn_in_steps": {float64(applied.Data.Depth)},
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
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(applied.Data.Depth)},
	})
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			generator.GenerateConfigs(),
		),
		Params:             simParams,
		ParamsAsPartitions: simParamsAsPartitions,
		ParamsFromUpstream: simParamsFromUpstream,
		InitStateValues:    simInitStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	}
}

// AppliedSimulationInference is the base configuration for an online
// inference of a simulation (specified by partition configs) from a
// referenced dataset.
type AppliedSimulationInference struct {
	Name       string
	Data       WindowedPartitionsData
	Simulation []*simulator.PartitionConfig
}

// NewSimulationInferencePartition creates a new PartitionConfig for
// an online simulation inference.
func NewSimulationInferencePartition(
	applied AppliedSimulationInference,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.Data.Depth,
		},
		// These will be overwritten with the times in the data...
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	params := simulator.NewParams(map[string][]float64{
		"cumulative":    {1},
		"burn_in_steps": {float64(applied.Data.Depth)},
	})
	generator.SetPartition(&simulator.PartitionConfig{
		Name:              "comparison",
		Iteration:         &inference.DataComparisonIteration{},
		Params:            params,
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(applied.Data.Depth)},
	})
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			generator.GenerateConfigs(),
		),
		Params:            simParams,
		InitStateValues:   []float64{},
		StateHistoryDepth: 1,
		Seed:              0,
	}
}
