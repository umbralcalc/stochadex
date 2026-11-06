package api

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// A macro has two routes to its partitions: a config expands it (pkg/api) and Go calls
// its constructor (pkg/macros). These tests hold the two to one another: each case
// states a macro both ways over the same data, and the runs agree bit for bit at every
// row of every partition the macro makes.

// crossCheckRows is the data the storage-reading macros read: flow, two values a step,
// and groups, the group each of flow's values belongs to. The values have no pattern a
// mistake could hide behind, and repeat exactly from one run to the next.
func crossCheckRows() (times []float64, flow, groups [][]float64) {
	const rows = 40
	for i := range rows {
		x := float64(i)
		times = append(times, 0.5*x)
		flow = append(flow, []float64{math.Sin(1.3*x) + 0.05*x, 3 * math.Cos(0.7*x*x)})
		groups = append(groups, []float64{float64(i % 3), float64((i + 1) % 2)})
	}
	return times, flow, groups
}

// crossCheckInputs is crossCheckRows as a config's inline input.
func crossCheckInputs() string {
	times, flow, groups := crossCheckRows()
	return fmt.Sprintf("inputs:\n  series:\n    source:\n      inline:\n        times: %v\n"+
		"        partitions:\n          flow: %v\n          groups: %v\nmacros:\n",
		inlineFloats(times), inlineRows(flow), inlineRows(groups))
}

func inlineFloats(values []float64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprint(value)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func inlineRows(rows [][]float64) string {
	parts := make([]string, len(rows))
	for i, row := range rows {
		parts[i] = inlineFloats(row)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// crossCheckStorage is crossCheckRows as a constructor's storage.
func crossCheckStorage() *simulator.StateTimeStorage {
	times, flow, groups := crossCheckRows()
	storage := simulator.NewStateTimeStorage()
	storage.SetTimes(times)
	storage.SetValues("flow", flow)
	storage.SetValues("groups", groups)
	return storage
}

var crossCheckFlow = analysis.DataRef{PartitionName: "flow"}

// timescale is the exponential kernel's param, which a constructor's caller sets.
func withTimescale(partition *simulator.PartitionConfig) *simulator.PartitionConfig {
	partition.Params.Set("exponential_weighting_timescale", []float64{2.0})
	return partition
}

// addMean runs the rolling mean the variance and covariance macros read.
func addMean(storage *simulator.StateTimeStorage) *simulator.StateTimeStorage {
	return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
		withTimescale(macros.NewVectorMeanPartition(macros.AppliedAggregation{
			Name: "m", Data: crossCheckFlow, Kernel: &kernels.ExponentialIntegrationKernel{},
		}, storage)),
	}, map[string]int{"flow": 5})
}

const crossCheckMeanMacro = "- {type: vector_mean, name: m, data: {partition_name: flow}, " +
	"kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, window: 5}\n"

// runOwnClock runs partitions that step on a clock of their own, as a live macro does.
func runOwnClock(partitions []*simulator.PartitionConfig, steps int) *simulator.StateTimeStorage {
	return analysis.NewStateTimeStorageFromPartitions(partitions,
		&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0}, 0.0)
}

// crossCheckMacros are the macros with a constructor, each stated both ways. A macro is
// here, or in macrosWithOneRoute saying why it need not be.
var crossCheckMacros = []struct {
	macro, name string
	config      func(t *testing.T) string
	partitions  []string
	direct      func() *simulator.StateTimeStorage
}{
	{
		macro: "vector_mean", name: "a rolling mean",
		config:     func(*testing.T) string { return crossCheckInputs() + crossCheckMeanMacro },
		partitions: []string{"m"},
		direct:     func() *simulator.StateTimeStorage { return addMean(crossCheckStorage()) },
	},
	{
		macro: "vector_mean", name: "a mean of chosen values, from a default",
		config: func(*testing.T) string {
			return crossCheckInputs() + "- {type: vector_mean, name: m, default_value: 7.0, " +
				"data: {partition_name: flow, value_indices: [1]}, kernel: {type: instantaneous}, window: 3}\n"
		},
		partitions: []string{"m"},
		direct: func() *simulator.StateTimeStorage {
			storage := crossCheckStorage()
			return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
				macros.NewVectorMeanPartition(macros.AppliedAggregation{
					Name: "m", DefaultValue: 7.0, Kernel: &kernels.InstantaneousIntegrationKernel{},
					Data: analysis.DataRef{PartitionName: "flow", ValueIndices: []int{1}},
				}, storage),
			}, map[string]int{"flow": 3})
		},
	},
	{
		macro: "vector_mean", name: "a cumulative mean",
		config: func(*testing.T) string {
			return crossCheckInputs() + "- {type: vector_mean, name: m, data: {partition_name: flow}, cumulative: true}\n"
		},
		partitions: []string{"m"},
		direct: func() *simulator.StateTimeStorage {
			storage := crossCheckStorage()
			return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
				macros.NewVectorCumulativeMeanPartition(macros.AppliedAggregation{Name: "m", Data: crossCheckFlow}, storage),
			}, nil)
		},
	},
	{
		macro: "vector_variance", name: "a rolling variance around a macro's mean",
		config: func(*testing.T) string {
			return crossCheckInputs() + crossCheckMeanMacro + "- {type: vector_variance, name: v, " +
				"mean: {partition_name: m}, data: {partition_name: flow}, kernel: {type: exponential}, " +
				"params: {exponential_weighting_timescale: [2.0]}, window: 5, default_value: 1.5}\n"
		},
		partitions: []string{"m", "v"},
		direct: func() *simulator.StateTimeStorage {
			storage := addMean(crossCheckStorage())
			return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
				withTimescale(macros.NewVectorVariancePartition(analysis.DataRef{PartitionName: "m"},
					macros.AppliedAggregation{Name: "v", Data: crossCheckFlow, DefaultValue: 1.5,
						Kernel: &kernels.ExponentialIntegrationKernel{}}, storage)),
			}, map[string]int{"flow": 5})
		},
	},
	{
		macro: "vector_covariance", name: "a rolling covariance around a macro's mean",
		config: func(*testing.T) string {
			return crossCheckInputs() + crossCheckMeanMacro + "- {type: vector_covariance, name: c, " +
				"mean: {partition_name: m}, data: {partition_name: flow}, kernel: {type: exponential}, " +
				"params: {exponential_weighting_timescale: [2.0]}, window: 5, default_value: 1.5}\n"
		},
		partitions: []string{"m", "c"},
		direct: func() *simulator.StateTimeStorage {
			storage := addMean(crossCheckStorage())
			return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
				withTimescale(macros.NewVectorCovariancePartition(analysis.DataRef{PartitionName: "m"},
					macros.AppliedAggregation{Name: "c", Data: crossCheckFlow, DefaultValue: 1.5,
						Kernel: &kernels.ExponentialIntegrationKernel{}}, storage)),
			}, map[string]int{"flow": 5})
		},
	},
	{
		macro: "grouped_aggregation", name: "a sum by group",
		config: func(*testing.T) string {
			return crossCheckInputs() + "- {type: grouped_aggregation, name: g, aggregation: sum, " +
				"group_by: [{partition_name: groups}], data: {partition_name: flow}, " +
				"kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, " +
				"window: 4, precision: 0, default_value: 0.5}\n"
		},
		partitions: []string{"g"},
		direct: func() *simulator.StateTimeStorage {
			storage := crossCheckStorage()
			grouped := analysis.NewGroupedStateTimeStorage(analysis.AppliedGrouping{
				GroupBy: []analysis.DataRef{{PartitionName: "groups"}}, Precision: 0}, storage)
			return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
				withTimescale(macros.NewGroupedAggregationPartition(general.SumAggregation,
					macros.AppliedAggregation{Name: "g", Data: crossCheckFlow, DefaultValue: 0.5,
						Kernel: &kernels.ExponentialIntegrationKernel{}}, grouped)),
			}, map[string]int{"flow": 4, "groups": 4})
		},
	},
	{
		macro: "scalar_regression_stats", name: "cumulative statistics with an intercept",
		config: func(*testing.T) string {
			return crossCheckInputs() + "- {type: scalar_regression_stats, name: r, intercept: true, " +
				"y: {partition_name: flow, value_indices: [0]}, x: {partition_name: flow, value_indices: [1]}}\n"
		},
		partitions: []string{"r"},
		direct: func() *simulator.StateTimeStorage {
			return crossCheckRegression(macros.AppliedScalarRegressionStats{Name: "r", Intercept: true})
		},
	},
	{
		macro: "scalar_regression_stats", name: "windowed statistics through the origin",
		config: func(*testing.T) string {
			return crossCheckInputs() + "- {type: scalar_regression_stats, name: r, mode: window, window_length: 6, " +
				"min_denominator: 15.0, state_history_depth: 3, " +
				"y: {partition_name: flow, value_indices: [0]}, x: {partition_name: flow, value_indices: [1]}}\n"
		},
		partitions: []string{"r"},
		direct: func() *simulator.StateTimeStorage {
			return crossCheckRegression(macros.AppliedScalarRegressionStats{Name: "r",
				Mode: macros.RegressionStatsWindow, WindowLength: 6, MinDenominator: 15.0, StateHistoryDepth: 3})
		},
	},
	{
		macro: "likelihood_comparison", name: "a rolling comparison against a replayed window",
		config: func(*testing.T) string {
			return crossCheckInputs() + `- type: likelihood_comparison
  name: l
  model:
    likelihood: {type: normal}
    params: {mean: [0.5, 1.0], covariance_matrix: [2.0, 0.0, 0.0, 3.0]}
  data: {partition_name: flow}
  window:
    data: [{partition_name: flow}]
    depth: 6
  window_data_history_depth: {flow: 6}
`
		},
		partitions: []string{"l"},
		direct: func() *simulator.StateTimeStorage {
			storage := crossCheckStorage()
			return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
				macros.NewLikelihoodComparisonPartition(macros.AppliedLikelihoodComparison{
					Name: "l",
					Model: macros.ParameterisedModel{
						Likelihood: &inference.NormalLikelihoodDistribution{},
						Params: simulator.NewParams(map[string][]float64{
							"mean": {0.5, 1.0}, "covariance_matrix": {2, 0, 0, 3}}),
					},
					Data:                   crossCheckFlow,
					Window:                 macros.WindowedPartitions{Data: []analysis.DataRef{crossCheckFlow}, Depth: 6},
					WindowDataHistoryDepth: map[string]int{"flow": 6},
				}, storage),
			}, map[string]int{"flow": 6})
		},
	},
	{
		macro: "likelihood_comparison", name: "a window running a model read from outside it",
		config: func(*testing.T) string {
			return crossCheckInputs() + crossCheckMeanMacro + `- type: likelihood_comparison
  name: l
  model:
    likelihood: {type: normal}
    params: {variance: [2.0, 3.0]}
    params_from_upstream: {mean: {upstream: walk}}
  data: {partition_name: flow}
  window:
    partitions:
    - partition:
        name: walk
        iteration: {type: wiener_process}
        params: {variances: [0.5, 0.5]}
        init_state_values: [0.0, 0.0]
        state_history_depth: 1
        seed: 3
    - partition:
        name: level
        iteration: {type: param_values}
        params: {param_values: [0.0, 0.0]}
        init_state_values: [0.0, 0.0]
        state_history_depth: 1
      outside_upstreams: {param_values: {upstream: m}}
    data: [{partition_name: flow}]
    depth: 5
  embedded_burn_in_steps: 7
  window_data_history_depth: {flow: 5}
`
		},
		partitions: []string{"m", "l"},
		direct: func() *simulator.StateTimeStorage {
			storage := addMean(crossCheckStorage())
			burnIn := 7
			return analysis.AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{
				macros.NewLikelihoodComparisonPartition(macros.AppliedLikelihoodComparison{
					Name: "l",
					Model: macros.ParameterisedModel{
						Likelihood: &inference.NormalLikelihoodDistribution{},
						Params:     simulator.NewParams(map[string][]float64{"variance": {2, 3}}),
						ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
							"mean": {Upstream: "walk"}},
					},
					Data: crossCheckFlow,
					Window: macros.WindowedPartitions{
						Partitions: []macros.WindowedPartition{
							{Partition: &simulator.PartitionConfig{
								Name:              "walk",
								Iteration:         &continuous.WienerProcessIteration{},
								Params:            simulator.NewParams(map[string][]float64{"variances": {0.5, 0.5}}),
								InitStateValues:   []float64{0, 0},
								StateHistoryDepth: 1,
								Seed:              3,
							}},
							{
								Partition: &simulator.PartitionConfig{
									Name:              "level",
									Iteration:         &general.ParamValuesIteration{},
									Params:            simulator.NewParams(map[string][]float64{"param_values": {0, 0}}),
									InitStateValues:   []float64{0, 0},
									StateHistoryDepth: 1,
								},
								OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{"param_values": {Upstream: "m"}},
							},
						},
						Data:  []analysis.DataRef{crossCheckFlow},
						Depth: 5,
					},
					EmbeddedBurnInSteps:    &burnIn,
					WindowDataHistoryDepth: map[string]int{"flow": 5},
				}, storage),
			}, map[string]int{"flow": 5})
		},
	},
	{
		macro: "posterior_estimation", name: "a posterior over a stream",
		config: func(*testing.T) string { return posteriorMacroYAML },
		partitions: []string{"test_post_log_norm", "test_post_mean", "test_post_cov",
			"test_post_sampler", "test_likelihood"},
		direct: func() *simulator.StateTimeStorage { return directPosteriorStorage() },
	},
	{
		macro: "posterior_estimation", name: "a posterior keeping only a variance",
		config: func(t *testing.T) string {
			return strings.NewReplacer(
				"covariance: {name: test_post_cov, default: [2.5, 0.0, 0.0, 9.0]}",
				"covariance: {name: test_post_cov, default: [2.5, 9.0], just_variance: true}",
				"        covariance_matrix: {upstream: test_post_cov}\n", "",
			).Replace(posteriorMacroYAML)
		},
		partitions: []string{"test_post_log_norm", "test_post_mean", "test_post_cov",
			"test_post_sampler", "test_likelihood"},
		direct: func() *simulator.StateTimeStorage {
			return directPosteriorStorage(func(applied *macros.AppliedPosteriorEstimation) {
				applied.Covariance = macros.PosteriorCovariance{Name: "test_post_cov",
					Default: []float64{2.5, 9.0}, JustVariance: true}
				delete(applied.Sampler.Distribution.ParamsFromUpstream, "covariance_matrix")
			})
		},
	},
	{
		macro: "likelihood_mean_function_fit", name: "a mean fitted by gradient descent",
		config:     func(*testing.T) string { return meanFitYAML },
		partitions: []string{"mean_fit"},
		direct:     func() *simulator.StateTimeStorage { return directMeanFitStorage() },
	},
	{
		macro: "likelihood_mean_function_fit", name: "a warm-started fit with its own burn-in",
		config: func(*testing.T) string {
			return strings.Replace(meanFitYAML, "  learning_rate: 0.005\n",
				"  learning_rate: 0.005\n  warm_start: true\n  embedded_burn_in_steps: 4\n", 1)
		},
		partitions: []string{"mean_fit"},
		direct: func() *simulator.StateTimeStorage {
			burnIn := 4
			return directMeanFitStorage(func(applied *macros.AppliedLikelihoodMeanFunctionFit) {
				applied.WarmStart, applied.EmbeddedBurnInSteps = true, &burnIn
			})
		},
	},
	{
		macro: "evolution_strategy_optimisation", name: "the optimisation example",
		config: func(t *testing.T) string { return esConfig(t, "steps: 1000", "steps: 60") },
		partitions: []string{"es_sampler", "es_sampler_simulation", "es_sorting", "es_mean",
			"es_covariance"},
		direct: func() *simulator.StateTimeStorage { return directESStorage(0) },
	},
	{
		macro: "evolution_strategy_optimisation", name: "a discounted reward, which the window's length changes",
		config: func(t *testing.T) string {
			return esConfig(t, "steps: 1000", "steps: 60", "discount_factor: 0.0", "discount_factor: 0.5")
		},
		partitions: []string{"es_sampler", "es_sampler_simulation", "es_sorting", "es_mean",
			"es_covariance"},
		direct: func() *simulator.StateTimeStorage { return directESStorage(0.5) },
	},
	{
		macro: "smc_inference", name: "the inference example",
		config:     func(t *testing.T) string { return smcConfig(t, "num_particles: 100", "num_particles: 20") },
		partitions: []string{"smc_proposals", "smc_sim", "smc_posterior"},
		direct: func() *simulator.StateTimeStorage {
			return directSMCStorage(func() simulator.Iteration { return &general.ParamValuesIteration{} }, "param_values", 0)
		},
	},
	{
		macro: "smc_inference", name: "a model that draws, so its seeds show",
		config: func(t *testing.T) string {
			return smcConfig(t, "num_particles: 100", "num_particles: 20",
				"iteration: {type: param_values}\n      params: {param_values: [0.0]}",
				"iteration: {type: expression, fields: [{name: p}], outputs: [\"center + shared(normal(0, 0.3))\"]}\n"+
					"      params: {center: [0.0]}\n      seed: 5",
				"pred/param_values", "pred/center")
		},
		partitions: []string{"smc_proposals", "smc_sim", "smc_posterior"},
		direct: func() *simulator.StateTimeStorage {
			return directSMCStorage(func() simulator.Iteration {
				return &general.ExpressionIteration{
					Fields:  []general.ExpressionField{{Name: "p"}},
					Outputs: []string{"center + shared(normal(0, 0.3))"},
				}
			}, "center", 5)
		},
	},
	{
		macro: "mcts_self_play", name: "tic-tac-toe from one move before a win",
		config: func(*testing.T) string {
			return `macros:
- type: mcts_self_play
  name: ttt
  steps: 4
  sims_per_ply: 40
  seed: 99
  env:
    type: tictactoe
    init_grid: [2, 0, 0, 2, 0, 0, 1, 1, 0]
    current_player: 0
`
		},
		partitions: []string{"ttt_apply", "ttt_search"},
		direct: func() *simulator.StateTimeStorage {
			environment, err := ResolveEnvironment(simulator.ComponentSpec{Type: "tictactoe",
				Fields: map[string]interface{}{
					"init_grid":      []interface{}{2, 0, 0, 2, 0, 0, 1, 1, 0},
					"current_player": 0,
				}})
			if err != nil {
				panic(err)
			}
			return runOwnClock(environment.selfPlay(macros.MCTSSearchSettings{Name: "ttt", SimsPerPly: 40, Seed: 99}), 4)
		},
	},
}

// macrosWithOneRoute are the macros with nothing to compare, and why.
var macrosWithOneRoute = map[string]string{
	"mcts_planning": "it has no constructor: only a config states it",
}

func crossCheckRegression(applied macros.AppliedScalarRegressionStats) *simulator.StateTimeStorage {
	storage := crossCheckStorage()
	applied.Y = analysis.DataRef{PartitionName: "flow", ValueIndices: []int{0}}
	applied.X = analysis.DataRef{PartitionName: "flow", ValueIndices: []int{1}}
	return analysis.AddPartitionsToStateTimeStorage(storage,
		[]*simulator.PartitionConfig{macros.NewScalarRegressionStatsPartition(applied, storage)}, nil)
}

// directESStorage is cfg/example_evolution_strategy_config.yaml through the constructor,
// for 60 steps, with the given discount.
func directESStorage(discount float64) *simulator.StateTimeStorage {
	return runOwnClock(macros.NewEvolutionStrategyOptimisationPartitions(
		macros.AppliedEvolutionStrategyOptimisation{
			Sampler: macros.EvolutionStrategySampler{Name: "es_sampler", Default: []float64{0, 0}},
			Sorting: macros.EvolutionStrategySorting{Name: "es_sorting", CollectionSize: 10, EmptyValue: -9999},
			Mean: macros.EvolutionStrategyMean{Name: "es_mean", Default: []float64{0, 0},
				Weights: []float64{0.5, 0.3, 0.2}, LearningRate: 0.5},
			Covariance: macros.EvolutionStrategyCovariance{Name: "es_covariance",
				Default: []float64{4, 0, 0, 4}, LearningRate: 0.1},
			Reward: macros.EvolutionStrategyReward{
				DiscountFactor: discount,
				Partition: macros.WindowedPartition{
					Partition: &simulator.PartitionConfig{
						Name: "reward",
						Iteration: &general.ExpressionIteration{
							Fields: []general.ExpressionField{{Name: "r"}},
							Outputs: []string{"-((sample_values[0]-3)*(sample_values[0]-3) + " +
								"(sample_values[1]+2)*(sample_values[1]+2))"},
						},
						Params:            simulator.NewParams(map[string][]float64{"sample_values": {0, 0}}),
						InitStateValues:   []float64{0},
						StateHistoryDepth: 1,
					},
					OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{"sample_values": {Upstream: "es_sampler"}},
				},
			},
			Window: macros.WindowedPartitions{
				Partitions: []macros.WindowedPartition{{Partition: &simulator.PartitionConfig{
					Name:              "sim_partition",
					Iteration:         &general.ConstantValuesIteration{},
					Params:            simulator.NewParams(map[string][]float64{}),
					InitStateValues:   []float64{0},
					StateHistoryDepth: 1,
				}}},
				Depth: 5,
			},
			Seed: 12345,
		}, nil), 60)
}

// directSMCStorage is cfg/example_smc_config.yaml through the constructor, with 20
// particles: the same stream replayed for each particle, from a run of its own.
func directSMCStorage(predict func() simulator.Iteration, param string, seed uint64) *simulator.StateTimeStorage {
	observed := analysis.NewStateTimeStorageFromPartitions([]*simulator.PartitionConfig{{
		Name:      "obs",
		Iteration: &inference.DataGenerationIteration{Likelihood: &inference.NormalLikelihoodDistribution{}},
		Params: simulator.NewParams(map[string][]float64{
			"mean": {2.0}, "covariance_matrix": {0.5}}),
		InitStateValues: []float64{2.0}, StateHistoryDepth: 1, Seed: 7,
	}}, &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 60},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0}, 0.0)
	rows, times := observed.GetValues("obs"), observed.GetTimes()
	return runOwnClock(macros.NewSMCInferencePartitions(macros.AppliedSMCInference{
		ProposalName: "smc_proposals", SimName: "smc_sim", PosteriorName: "smc_posterior",
		NumParticles: 20, NumRounds: 3, Seed: 42,
		Priors:     []inference.Prior{&inference.UniformPrior{Lo: -5, Hi: 10}},
		ParamNames: []string{"mean"},
		Model: macros.SMCParticleModel{Build: func(int) *macros.SMCInnerSimConfig {
			return &macros.SMCInnerSimConfig{
				Partitions: []*simulator.PartitionConfig{
					{
						Name:              "observed_data",
						Iteration:         &general.FromStorageIteration{Data: rows},
						Params:            simulator.NewParams(map[string][]float64{}),
						InitStateValues:   rows[0],
						StateHistoryDepth: 2,
					},
					{
						Name:              "pred",
						Iteration:         predict(),
						Params:            simulator.NewParams(map[string][]float64{param: {0}}),
						InitStateValues:   []float64{0},
						StateHistoryDepth: 2,
						Seed:              seed,
					},
					{
						Name:      "loglike",
						Iteration: &inference.DataComparisonIteration{Likelihood: &inference.NormalLikelihoodDistribution{}},
						Params: simulator.NewParams(map[string][]float64{
							"mean": {0}, "variance": {0.5}, "latest_data_values": {2},
							"cumulative": {1}, "burn_in_steps": {0}}),
						ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
							"mean":               {Upstream: "pred"},
							"latest_data_values": {Upstream: "observed_data"},
						},
						InitStateValues:   []float64{0},
						StateHistoryDepth: 2,
					},
				},
				Simulation: &simulator.SimulationConfig{
					OutputCondition:      &simulator.NilOutputCondition{},
					OutputFunction:       &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: len(times) - 1},
					TimestepFunction:     &general.FromStorageTimestepFunction{Data: times},
					InitTimeValue:        times[0],
				},
				LoglikePartition: "loglike",
				ParamForwarding:  map[string][]int{"pred/" + param: {0}},
			}
		}},
	}), 3)
}

// sameRun fails unless two runs agree bit for bit at every row of the named partitions,
// and between them those partitions moved, so that agreeing shows something.
func sameRun(t *testing.T, got, want *simulator.StateTimeStorage, partitions []string) {
	t.Helper()
	moved := false
	for _, name := range partitions {
		gotRows, wantRows := got.GetValues(name), want.GetValues(name)
		if len(wantRows) == 0 || len(gotRows) != len(wantRows) {
			t.Fatalf("%s: the config gives %d rows, the constructor %d", name, len(gotRows), len(wantRows))
		}
		for step, wantRow := range wantRows {
			if len(gotRows[step]) != len(wantRow) {
				t.Fatalf("%s, row %d: the config gives width %d, the constructor %d",
					name, step, len(gotRows[step]), len(wantRow))
			}
			for i, value := range wantRow {
				if math.Float64bits(gotRows[step][i]) != math.Float64bits(value) {
					t.Fatalf("%s, row %d, element %d: the config gives %v, the constructor %v",
						name, step, i, gotRows[step][i], value)
				}
				moved = moved || value != wantRows[0][i]
			}
		}
	}
	if !moved {
		t.Errorf("%v never moved, so agreeing shows nothing", partitions)
	}
}

func TestMacroRoutesAgree(t *testing.T) {
	covered := map[string]bool{}
	for _, c := range crossCheckMacros {
		covered[c.macro] = true
		t.Run(c.macro+": "+c.name, func(t *testing.T) {
			sameRun(t, runToStorageT(t, c.config(t)), c.direct(), c.partitions)
		})
	}
	for macro := range macroSpecFactories {
		if _, oneRoute := macrosWithOneRoute[macro]; covered[macro] == oneRoute {
			t.Errorf("%s: a macro has a cross-check here, or a reason in macrosWithOneRoute, not both or neither", macro)
		}
	}
}
