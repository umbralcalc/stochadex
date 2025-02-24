package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ParameterisedKernel defines an integration kernel over the data with
// its corresponding parameters to set.
type ParameterisedKernel struct {
	Kernel             kernels.IntegrationKernel
	Params             simulator.Params
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// Init populates the kernel parameter fields if they have not been set.
func (p *ParameterisedKernel) Init() {
	if p.ParamsAsPartitions == nil {
		p.ParamsAsPartitions = make(map[string][]string)
	}
	if p.ParamsFromUpstream == nil {
		p.ParamsFromUpstream =
			make(map[string]simulator.NamedUpstreamConfig)
	}
}

// AppliedGaussianProcessDistributionFit is the base configuration for
// fitting a Gaussian Process to the probability distribution over values
// in the specified data.
type AppliedGaussianProcessDistributionFit struct {
	Name         string
	Kernel       ParameterisedKernel
	Data         DataRef
	Window       WindowedPartitions
	LearningRate float64
}

// NewGaussianProcessDistributionFitPartition creates a new PartitionConfig
// for fitting a Gaussian Process to the probability distribution over values
// in the specified data.
func NewGaussianProcessDistributionFitPartition(
	applied AppliedGaussianProcessDistributionFit,
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
	applied.Kernel.Init()
	applied.Kernel.Params.Set(applied.Data.PartitionName+"->data", []float64{})
	applied.Kernel.Params.Set(applied.Name+"->function_values_data", []float64{})
	gradInitStateValues := make([]float64, len(applied.Data.GetValueIndices(storage)))
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "gradient",
		Iteration: &inference.GaussianProcessGradientIteration{
			Kernel: applied.Kernel.Kernel,
		},
		Params:             applied.Kernel.Params,
		ParamsAsPartitions: applied.Kernel.ParamsAsPartitions,
		ParamsFromUpstream: applied.Kernel.ParamsFromUpstream,
		InitStateValues:    gradInitStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	})
	simParamsAsPartitions["gradient/update_from_partition_history"] =
		[]string{applied.Data.PartitionName, applied.Name}
	simInitStateValues = append(simInitStateValues, gradInitStateValues...)
	gradientDescentParams := simulator.NewParams(make(map[string][]float64))
	gradientDescentParams.Set("learning_rate", []float64{applied.LearningRate})
	generator.SetPartition(&simulator.PartitionConfig{
		Name:      "gradient_descent",
		Iteration: &continuous.GradientDescentIteration{},
		Params:    gradientDescentParams,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"gradient": {Upstream: "gradient"},
		},
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	simInitStateValues = append(simInitStateValues, 0.0)
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(applied.Window.Depth)},
	})
	generator.GetPartition("gradient").Params.Set(
		"function_values_data_index",
		[]float64{float64(len(simInitStateValues) - 1)},
	)
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
