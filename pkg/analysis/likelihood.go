package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// WindowedPartitions defines a windowed history of data from
// partitions in storage and possible additional partitions to include
// when simulating the window duration.
type WindowedPartitions struct {
	Partitions []*simulator.PartitionConfig
	Data       []DataRef
	Depth      int
}

// ParameterisedModel defines a likelihood model for the data with its
// corresponding parameters to set.
type ParameterisedModel struct {
	Likelihood         inference.LikelihoodDistribution
	Params             simulator.Params
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// Init populates the model parameter fields if they have not been set.
func (p *ParameterisedModel) Init() {
	if p.ParamsAsPartitions == nil {
		p.ParamsAsPartitions = make(map[string][]string)
	}
	if p.ParamsFromUpstream == nil {
		p.ParamsFromUpstream =
			make(map[string]simulator.NamedUpstreamConfig)
	}
}

// AppliedLikelihoodComparison is the base configuration for a rolling
// comparison between a referenced dataset and referenced likelihood model.
type AppliedLikelihoodComparison struct {
	Name   string
	Model  ParameterisedModel
	Data   DataRef
	Window WindowedPartitions
}

// newLikelihoodComparisonGenerator creates a new config generator for
// a rolling likelihood comparison.
func newLikelihoodComparisonGenerator(
	applied AppliedLikelihoodComparison,
	storage *simulator.StateTimeStorage,
) *simulator.ConfigGenerator {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.Window.Depth,
		},
		// These will be overwritten with the times in the data...
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	for _, ref := range applied.Window.Data {
		initStateValues := ref.GetIndexFromStorage(storage, 0)
		generator.SetPartition(&simulator.PartitionConfig{
			Name:              ref.PartitionName,
			Iteration:         &general.FromHistoryIteration{},
			Params:            simulator.NewParams(make(map[string][]float64)),
			InitStateValues:   initStateValues,
			StateHistoryDepth: 1,
			Seed:              0,
		})
	}
	if applied.Window.Partitions != nil {
		for _, partition := range applied.Window.Partitions {
			generator.SetPartition(partition)
		}
	}
	applied.Model.Init()
	applied.Model.Params.Set("cumulative", []float64{1})
	applied.Model.Params.Set("burn_in_steps", []float64{0})
	applied.Model.ParamsFromUpstream["latest_data_values"] =
		simulator.NamedUpstreamConfig{
			Upstream: applied.Data.PartitionName,
			Indices:  applied.Data.ValueIndices,
		}
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "comparison",
		Iteration: &inference.DataComparisonIteration{
			Likelihood: applied.Model.Likelihood,
		},
		Params:             applied.Model.Params,
		ParamsAsPartitions: applied.Model.ParamsAsPartitions,
		ParamsFromUpstream: applied.Model.ParamsFromUpstream,
		InitStateValues:    []float64{0.0},
		StateHistoryDepth:  1,
		Seed:               0,
	})
	return generator
}

// NewLikelihoodComparisonPartition creates a new PartitionConfig for
// a rolling likelihood comparison.
func NewLikelihoodComparisonPartition(
	applied AppliedLikelihoodComparison,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := newLikelihoodComparisonGenerator(applied, storage)
	simInitStateValues := make([]float64, 0)
	simParamsAsPartitions := make(map[string][]string)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	for _, ref := range applied.Window.Data {
		initStateValues := ref.GetIndexFromStorage(storage, 0)
		simInitStateValues = append(simInitStateValues, initStateValues...)
		simParamsAsPartitions[ref.PartitionName+"/state_memory_partition"] =
			[]string{ref.PartitionName}
		simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
			simulator.NamedUpstreamConfig{
				Upstream: ref.PartitionName,
				Indices:  ref.ValueIndices,
			}
	}
	simInitStateValues = append(simInitStateValues, 0.0)
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(applied.Window.Depth)},
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

// AppliedInference is the base configuration for an online
// inference of a simulation (specified by partition configs) from a
// referenced dataset.
type AppliedInference struct {
	Name string
}

// NewSimulationInferencePartition creates a new PartitionConfig for
// an online simulation inference.
func NewSimulationInferencePartition(
	comparison AppliedLikelihoodComparison,
	inference AppliedInference,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := newLikelihoodComparisonGenerator(comparison, storage)
	simInitStateValues := make([]float64, 0)
	simParamsAsPartitions := make(map[string][]string)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	for _, ref := range comparison.Window.Data {
		initStateValues := ref.GetIndexFromStorage(storage, 0)
		simInitStateValues = append(simInitStateValues, initStateValues...)
		simParamsAsPartitions[ref.PartitionName+"/state_memory_partition"] =
			[]string{ref.PartitionName}
		simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
			simulator.NamedUpstreamConfig{
				Upstream: ref.PartitionName,
				Indices:  ref.ValueIndices,
			}
	}
	simInitStateValues = append(simInitStateValues, 0.0)
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(comparison.Window.Depth)},
	})
	return &simulator.PartitionConfig{
		Name: inference.Name,
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
