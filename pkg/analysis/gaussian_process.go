package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AppliedGaussianProcessDistributionFit is the base configuration for
// fitting a Gaussian Process to the probability distribution over values
// in the specified data.
type AppliedGaussianProcessDistributionFit struct {
	Name              string
	Data              DataRef
	Window            WindowedPartitions
	KernelCovariance  []float64
	BaseVariance      float64
	PastDiscount      float64
	LearningRate      float64
	DescentIterations int
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
	gradParams := simulator.NewParams(make(map[string][]float64))
	gradParams.Set(applied.Data.PartitionName+"->data", []float64{})
	gradParams.Set(applied.Name+"->function_values_data", []float64{})
	gradParams.Set("covariance_matrix", applied.KernelCovariance)
	gradParams.Set("base_variance", []float64{applied.BaseVariance})
	gradParams.Set("past_discounting_factor", []float64{applied.PastDiscount})
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "gradient",
		Iteration: &inference.GaussianProcessGradientIteration{
			Kernel: &kernels.SquaredExponentialStateIntegrationKernel{},
		},
		Params: gradParams,
		ParamsAsPartitions: map[string][]string{
			"function_values_partition": {"gradient_descent"},
		},
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	simParamsAsPartitions["gradient/update_from_partition_history"] =
		[]string{applied.Data.PartitionName, applied.Name}
	simParamsFromUpstream["gradient/target_state"] =
		simulator.NamedUpstreamConfig{Upstream: applied.Data.PartitionName}
	simInitStateValues = append(simInitStateValues, 0.0)
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
		"burn_in_steps":           {float64(applied.Window.Depth)},
		"ignore_timestep_history": {1},
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
		StateHistoryDepth:  applied.Window.Depth,
		Seed:               0,
	}
}
