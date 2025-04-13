package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// WindowedPartition configures a partition to simulate within a
// windowed duration.
type WindowedPartition struct {
	Partition        *simulator.PartitionConfig
	OutsideUpstreams map[string]simulator.NamedUpstreamConfig
}

// WindowedPartitions defines a windowed history of data from
// partitions in storage and possible additional partitions to include
// when simulating the window duration.
type WindowedPartitions struct {
	Partitions []WindowedPartition
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
			MaxNumberOfSteps: applied.Window.Depth,
		},
		// These will be overwritten with the times in the data...
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	simInitStateValues := make([]float64, 0)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	if applied.Window.Partitions != nil {
		for _, partition := range applied.Window.Partitions {
			generator.SetPartition(partition.Partition)
			simInitStateValues = append(
				simInitStateValues,
				partition.Partition.InitStateValues...,
			)
			if partition.OutsideUpstreams == nil {
				continue
			}
			for paramsName, upstream := range partition.OutsideUpstreams {
				simParamsFromUpstream[partition.Partition.Name+
					"/"+paramsName] = upstream
			}
		}
	}
	simParamsAsPartitions := make(map[string][]string)
	if applied.Window.Data != nil {
		for _, ref := range applied.Window.Data {
			if ref.ValueIndices != nil {
				panic("value indices are not supported in window data")
			}
			initStateValues := ref.GetTimeIndexFromStorage(storage, 0)
			generator.SetPartition(&simulator.PartitionConfig{
				Name:              ref.PartitionName,
				Iteration:         &general.FromHistoryIteration{},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   initStateValues,
				StateHistoryDepth: 1,
				Seed:              0,
			})
			simInitStateValues = append(simInitStateValues, initStateValues...)
			simParamsAsPartitions[ref.PartitionName+
				"/update_from_partition_history"] = []string{ref.PartitionName}
			simParamsAsPartitions[ref.PartitionName+
				"/initial_state_from_partition_history"] = []string{ref.PartitionName}
			simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
				simulator.NamedUpstreamConfig{Upstream: ref.PartitionName}
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

// ParameterisedModelWithGradient defines a ParameterisedModel
// which has a gradient.
type ParameterisedModelWithGradient struct {
	Likelihood         inference.LikelihoodDistributionWithGradient
	Params             simulator.Params
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// Init populates the model parameter fields if they have not been set.
func (p *ParameterisedModelWithGradient) Init() {
	if p.ParamsAsPartitions == nil {
		p.ParamsAsPartitions = make(map[string][]string)
	}
	if p.ParamsFromUpstream == nil {
		p.ParamsFromUpstream =
			make(map[string]simulator.NamedUpstreamConfig)
	}
}

// LikelihoodMeanGradient defines the function which takes the gradient
// of the likelihood mean with respect to the desired fit parameters.
type LikelihoodMeanGradient struct {
	Function func(
		params *simulator.Params,
		likeMeanGrad []float64,
	) []float64
	Width int
}

// AppliedLikelihoodMeanFunctionFit is the base configuration for
// fitting the mean of a referenced likelihood model with a function
// specified by its gradient with respect to the desired fit parameters.
type AppliedLikelihoodMeanFunctionFit struct {
	Name              string
	Model             ParameterisedModelWithGradient
	Gradient          LikelihoodMeanGradient
	Data              DataRef
	Window            WindowedPartitions
	LearningRate      float64
	DescentIterations int
}

// NewLikelihoodMeanFunctionFitPartition creates a new PartitionConfig
// fitting the mean of a referenced likelihood model with a function
// specified by its gradient with respect to the desired fit parameters.
func NewLikelihoodMeanFunctionFitPartition(
	applied AppliedLikelihoodMeanFunctionFit,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.DescentIterations,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	simInitStateValues := make([]float64, 0)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	if applied.Window.Partitions != nil {
		for _, partition := range applied.Window.Partitions {
			generator.SetPartition(partition.Partition)
			simInitStateValues = append(
				simInitStateValues,
				partition.Partition.InitStateValues...,
			)
			if partition.OutsideUpstreams == nil {
				continue
			}
			for paramsName, upstream := range partition.OutsideUpstreams {
				simParamsFromUpstream[partition.Partition.Name+
					"/"+paramsName] = upstream
			}
		}
	}
	simParamsAsPartitions := make(map[string][]string)
	if applied.Window.Data != nil {
		for _, ref := range applied.Window.Data {
			if ref.ValueIndices != nil {
				panic("value indices are not supported in window data")
			}
			initStateValues := ref.GetTimeIndexFromStorage(storage, 0)
			generator.SetPartition(&simulator.PartitionConfig{
				Name:              ref.PartitionName,
				Iteration:         &general.FromHistoryIteration{},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   initStateValues,
				StateHistoryDepth: 1,
				Seed:              0,
			})
			simInitStateValues = append(simInitStateValues, initStateValues...)
			simParamsAsPartitions[ref.PartitionName+
				"/update_from_partition_history"] = []string{ref.PartitionName}
			simParamsAsPartitions[ref.PartitionName+
				"/initial_state_from_partition_history"] = []string{ref.PartitionName}
			simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
				simulator.NamedUpstreamConfig{Upstream: ref.PartitionName}
		}
	}
	applied.Model.Init()
	applied.Model.Params.Set(applied.Data.PartitionName+"->data_values", []float64{})
	applied.Model.ParamsAsPartitions["mean_partition"] = []string{"gradient_descent"}
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "gradient",
		Iteration: &inference.DataComparisonGradientIteration{
			Likelihood:   applied.Model.Likelihood,
			GradientFunc: applied.Gradient.Function,
		},
		Params:             applied.Model.Params,
		ParamsAsPartitions: applied.Model.ParamsAsPartitions,
		ParamsFromUpstream: applied.Model.ParamsFromUpstream,
		InitStateValues:    make([]float64, applied.Gradient.Width),
		StateHistoryDepth:  1,
		Seed:               0,
	})
	simParamsAsPartitions["gradient/update_from_partition_history"] =
		[]string{applied.Data.PartitionName}
	simParamsFromUpstream["gradient/target_state"] =
		simulator.NamedUpstreamConfig{Upstream: applied.Data.PartitionName}
	simInitStateValues = append(
		simInitStateValues, make([]float64, applied.Gradient.Width)...)
	gradientDescentParams := simulator.NewParams(make(map[string][]float64))
	gradientDescentParams.Set("learning_rate", []float64{applied.LearningRate})
	generator.SetPartition(&simulator.PartitionConfig{
		Name:      "gradient_descent",
		Iteration: &continuous.GradientDescentIteration{},
		Params:    gradientDescentParams,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"gradient": {Upstream: "gradient"},
		},
		InitStateValues:   make([]float64, applied.Gradient.Width),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	simInitStateValues = append(
		simInitStateValues, make([]float64, applied.Gradient.Width)...)
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps":           {float64(applied.Window.Depth)},
		"ignore_timestep_history": {1},
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
		StateHistoryDepth:  applied.Window.Depth,
		Seed:               0,
	}
}
