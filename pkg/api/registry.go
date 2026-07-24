package api

import (
	"fmt"
	"sort"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// iterationBuilder constructs a simulator.Iteration from a data spec's fields,
// validating them strictly (an unknown field is an error, not silently ignored).
type iterationBuilder func(fields map[string]interface{}) (simulator.Iteration, error)

// iterationBuilders is the data-only iteration registry: each name maps to a
// builder for a framework iteration whose whole configuration is data (its
// numeric parameters come from the partition's params:, so the spec itself
// usually carries no fields). Composable iterations (those with an interface- or
// func-typed field — kernels, likelihoods, nested iterations, aggregations) are
// deliberately absent; they are Phase B (recursive specs). Live-object iterations
// (from_storage, from_history, embedded_simulation_run, …) have no data form at
// all. Both groups are enumerated with reasons in the coverage drift test.
var iterationBuilders = map[string]iterationBuilder{
	// pkg/continuous
	"wiener_process":                    nullary(func() simulator.Iteration { return &continuous.WienerProcessIteration{} }),
	"gradient_descent":                  nullary(func() simulator.Iteration { return &continuous.GradientDescentIteration{} }),
	"ornstein_uhlenbeck":                nullary(func() simulator.Iteration { return &continuous.OrnsteinUhlenbeckIteration{} }),
	"ornstein_uhlenbeck_exact_gaussian": nullary(func() simulator.Iteration { return &continuous.OrnsteinUhlenbeckExactGaussianIteration{} }),
	"geometric_brownian_motion":         nullary(func() simulator.Iteration { return &continuous.GeometricBrownianMotionIteration{} }),
	"drift_diffusion":                   nullary(func() simulator.Iteration { return &continuous.DriftDiffusionIteration{} }),
	"cumulative_time":                   nullary(func() simulator.Iteration { return &continuous.CumulativeTimeIteration{} }),

	// pkg/discrete
	"poisson_process":              nullary(func() simulator.Iteration { return &discrete.PoissonProcessIteration{} }),
	"cox_process":                  nullary(func() simulator.Iteration { return &discrete.CoxProcessIteration{} }),
	"bernoulli_process":            nullary(func() simulator.Iteration { return &discrete.BernoulliProcessIteration{} }),
	"binomial_observation_process": nullary(func() simulator.Iteration { return &discrete.BinomialObservationProcessIteration{} }),
	"categorical_state_transition": nullary(func() simulator.Iteration { return &discrete.CategoricalStateTransitionIteration{} }),
	"hawkes_process":               nullary(func() simulator.Iteration { return &discrete.HawkesProcessIteration{} }),

	// pkg/general
	"values_sorted_collection_mean":       nullary(func() simulator.Iteration { return &general.ValuesSortedCollectionMeanIteration{} }),
	"values_sorted_collection_covariance": nullary(func() simulator.Iteration { return &general.ValuesSortedCollectionCovarianceIteration{} }),
	"constant_values":                     nullary(func() simulator.Iteration { return &general.ConstantValuesIteration{} }),
	"copy_values":                         nullary(func() simulator.Iteration { return &general.CopyValuesIteration{} }),
	"param_values":                        nullary(func() simulator.Iteration { return &general.ParamValuesIteration{} }),
	// values_weighted_resampling holds a rand.Source, but Configure assigns it from
	// the partition seed, so the zero value is a complete construction and every
	// other input (log_weight_partitions, data_values_partitions, …) is params.
	"values_weighted_resampling": nullary(func() simulator.Iteration { return &general.ValuesWeightedResamplingIteration{} }),

	// pkg/inference
	"posterior_covariance":        nullary(func() simulator.Iteration { return &inference.PosteriorCovarianceIteration{} }),
	"posterior_log_normalisation": nullary(func() simulator.Iteration { return &inference.PosteriorLogNormalisationIteration{} }),
	"smc_posterior":               buildSMCPosterior,

	// from_history copies another partition's history into an embedded window. Its
	// Data field is not config: the embedded-run machinery injects it at runtime
	// via UpdateMemory (StateMemoryIteration), exactly as it does for the Go form
	// &general.FromHistoryIteration{}. It is only meaningful inside an embedded run.
	"from_history": buildFromHistory,
}

// extraIterationBuilders holds iteration builders contributed by a package
// layered above this one, so a component with a heavy dependency can add a
// {type: ...} spelling without the engine importing it. Imports drive go.mod;
// keeping these out of iterationBuilders is what lets the core stay lean and
// CGO_ENABLED=0-clean. The opt-in pkg/onnx module self-registers its
// onnx_inference iteration this way (it pulls in a cgo ONNX Runtime), exactly as
// data sources (RegisterDataSource) and output sinks (simulator.RegisterComponent)
// are contributed from downstream. The coverage drift test only scans the core
// candidate packages, so a downstream iteration needs no exclusion entry there.
var extraIterationBuilders = map[string]func(simulator.ComponentSpec) (simulator.Iteration, error){}

// RegisterIteration registers an iteration builder for a {type: ...} spelling
// that lives downstream of this package. Call it from an init(); it panics on a
// duplicate so two packages cannot silently claim one name. The builder receives
// the whole ComponentSpec (Type plus Fields) and is responsible for strict field
// validation, just like the core builders.
func RegisterIteration(
	typeName string,
	build func(simulator.ComponentSpec) (simulator.Iteration, error),
) {
	if _, exists := iterationBuilders[typeName]; exists {
		panic("api: iteration name already claimed by the core registry: " + typeName)
	}
	if _, exists := extraIterationBuilders[typeName]; exists {
		panic("api: duplicate iteration registration " + typeName)
	}
	extraIterationBuilders[typeName] = build
}

// ResolveIteration builds a simulator.Iteration from a data-spec ComponentSpec.
func ResolveIteration(spec simulator.ComponentSpec) (simulator.Iteration, error) {
	if build, ok := iterationBuilders[spec.Type]; ok {
		return build(spec.Fields)
	}
	if build, ok := extraIterationBuilders[spec.Type]; ok {
		return build(spec)
	}
	return nil, fmt.Errorf(
		"iteration: unknown data-spec type %q (is it registered, or is it a "+
			"composable/live-object iteration with no data form yet? some spellings "+
			"like onnx_inference are only in the distributed cmd/stochadex build)",
		spec.Type,
	)
}

// nullary adapts a no-field iteration constructor into a builder that rejects any
// provided fields (a mistyped key is an error, not silently ignored).
func nullary(construct func() simulator.Iteration) iterationBuilder {
	return func(fields map[string]interface{}) (simulator.Iteration, error) {
		if len(fields) > 0 {
			return nil, fmt.Errorf(
				"iteration takes no fields, got %v (its parameters go in the "+
					"partition's params:, not the iteration spec)",
				sortedFieldKeys(fields),
			)
		}
		return construct(), nil
	}
}

// buildSMCPosterior builds an SMCPosteriorIteration, whose only data field is the
// optional param_names label list (its counts come from params:).
func buildSMCPosterior(fields map[string]interface{}) (simulator.Iteration, error) {
	iteration := &inference.SMCPosteriorIteration{}
	for key, value := range fields {
		if key != "param_names" {
			return nil, fmt.Errorf("smc_posterior: unknown field %q", key)
		}
		raw, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("smc_posterior: param_names must be a list, got %T", value)
		}
		names := make([]string, len(raw))
		for i, element := range raw {
			name, ok := element.(string)
			if !ok {
				return nil, fmt.Errorf("smc_posterior: param_names[%d] must be a string", i)
			}
			names[i] = name
		}
		iteration.ParamNames = names
	}
	return iteration, nil
}

// buildFromHistory builds a FromHistoryIteration, whose only data field is the
// optional init_steps_taken offset (its Data is injected by the embedded-run
// machinery at runtime).
func buildFromHistory(fields map[string]interface{}) (simulator.Iteration, error) {
	iteration := &general.FromHistoryIteration{}
	for key, value := range fields {
		if key != "init_steps_taken" {
			return nil, fmt.Errorf("from_history: unknown field %q", key)
		}
		steps, ok := value.(int)
		if !ok {
			return nil, fmt.Errorf("from_history: init_steps_taken must be an integer, got %T", value)
		}
		iteration.InitStepsTaken = steps
	}
	return iteration, nil
}

func sortedFieldKeys(fields map[string]interface{}) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
