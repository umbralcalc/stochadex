package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// This file is Phase B: composition. A composable iteration has an interface- or
// func-typed field (a kernel, likelihood, jump distribution, prior, nested
// iteration, value function, aggregation or transform). Its data spec nests the
// sub-spec (recursively for interface values) or names a framework-shipped
// function, and the resolver here builds the whole tree with no Go toolchain.

// specReader reads a data spec's fields with strict key checking and recursive
// resolution of nested interface specs and named function values. It accumulates
// the first error; callers check done() once at the end.
type specReader struct {
	what   string
	fields map[string]interface{}
	used   map[string]bool
	err    error
}

func newSpecReader(what string, fields map[string]interface{}) *specReader {
	return &specReader{
		what:   what,
		fields: fields,
		used:   make(map[string]bool, len(fields)),
	}
}

func (r *specReader) fail(format string, args ...interface{}) {
	if r.err == nil {
		r.err = fmt.Errorf(r.what+": "+format, args...)
	}
}

func (r *specReader) value(key string, required bool) (interface{}, bool) {
	r.used[key] = true
	value, ok := r.fields[key]
	if !ok && required {
		r.fail("missing required field %q", key)
	}
	return value, ok
}

func (r *specReader) optBool(key string) bool {
	value, ok := r.value(key, false)
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	if !ok {
		r.fail("field %q must be a bool, got %T", key, value)
	}
	return typed
}

func (r *specReader) intField(key string) int {
	value, ok := r.value(key, true)
	if !ok {
		return 0
	}
	typed, ok := value.(int)
	if !ok {
		r.fail("field %q must be an integer, got %T", key, value)
	}
	return typed
}

// nestedSpec coerces a field's value (a YAML mapping, which yaml.v2 delivers as
// map[interface{}]interface{}) into a ComponentSpec for recursive resolution.
func (r *specReader) nestedSpec(key string, required bool) (simulator.ComponentSpec, bool) {
	value, ok := r.value(key, required)
	if !ok {
		return simulator.ComponentSpec{}, false
	}
	spec, err := toComponentSpec(value)
	if err != nil {
		r.fail("field %q: %v", key, err)
		return simulator.ComponentSpec{}, false
	}
	return spec, true
}

func (r *specReader) kernel(key string) kernels.IntegrationKernel {
	spec, ok := r.nestedSpec(key, true)
	if !ok {
		return nil
	}
	resolved, err := resolveKernel(spec)
	if err != nil {
		r.fail("field %q: %v", key, err)
	}
	return resolved
}

func (r *specReader) likelihood(key string) inference.LikelihoodDistribution {
	spec, ok := r.nestedSpec(key, true)
	if !ok {
		return nil
	}
	resolved, err := resolveLikelihood(spec)
	if err != nil {
		r.fail("field %q: %v", key, err)
	}
	return resolved
}

func (r *specReader) jump(key string) continuous.JumpDistribution {
	spec, ok := r.nestedSpec(key, true)
	if !ok {
		return nil
	}
	resolved, err := resolveJump(spec)
	if err != nil {
		r.fail("field %q: %v", key, err)
	}
	return resolved
}

func (r *specReader) iteration(key string) simulator.Iteration {
	spec, ok := r.nestedSpec(key, true)
	if !ok {
		return nil
	}
	resolved, err := ResolveIteration(spec)
	if err != nil {
		r.fail("field %q: %v", key, err)
	}
	return resolved
}

func (r *specReader) priorList(key string) []inference.Prior {
	value, ok := r.value(key, true)
	if !ok {
		return nil
	}
	raw, ok := value.([]interface{})
	if !ok {
		r.fail("field %q must be a list of prior specs, got %T", key, value)
		return nil
	}
	priors := make([]inference.Prior, len(raw))
	for i, element := range raw {
		spec, err := toComponentSpec(element)
		if err != nil {
			r.fail("field %q[%d]: %v", key, i, err)
			return nil
		}
		prior, err := resolvePrior(spec)
		if err != nil {
			r.fail("field %q[%d]: %v", key, i, err)
			return nil
		}
		priors[i] = prior
	}
	return priors
}

// namedFunc looks a framework-shipped function value up by name in a typed
// registry, failing if the name is unknown.
func namedFunc[T any](r *specReader, key string, registry map[string]T) T {
	var zero T
	value, ok := r.value(key, true)
	if !ok {
		return zero
	}
	name, ok := value.(string)
	if !ok {
		r.fail("field %q must be a function name (string), got %T", key, value)
		return zero
	}
	fn, ok := registry[name]
	if !ok {
		r.fail("field %q: unknown function %q", key, name)
		return zero
	}
	return fn
}

func (r *specReader) done() error {
	if r.err != nil {
		return r.err
	}
	for key := range r.fields {
		if !r.used[key] {
			return fmt.Errorf("%s: unknown field %q", r.what, key)
		}
	}
	return nil
}

// toComponentSpec coerces a nested YAML value into a ComponentSpec. yaml.v2
// delivers a nested mapping as map[interface{}]interface{}, so both that and
// map[string]interface{} are accepted.
func toComponentSpec(value interface{}) (simulator.ComponentSpec, error) {
	fields := make(map[string]interface{})
	switch typed := value.(type) {
	case map[string]interface{}:
		for k, v := range typed {
			fields[k] = v
		}
	case map[interface{}]interface{}:
		for k, v := range typed {
			key, ok := k.(string)
			if !ok {
				return simulator.ComponentSpec{}, fmt.Errorf("spec key %v is not a string", k)
			}
			fields[key] = v
		}
	default:
		return simulator.ComponentSpec{}, fmt.Errorf(
			"expected a {type: ...} mapping, got %T", value)
	}
	kind, ok := fields["type"].(string)
	if !ok || kind == "" {
		return simulator.ComponentSpec{}, fmt.Errorf("spec needs a string 'type', got %v", fields)
	}
	delete(fields, "type")
	return simulator.ComponentSpec{Type: kind, Fields: fields}, nil
}

// ---- interface sub-registries -------------------------------------------------

// resolveKernel builds an IntegrationKernel, recursively for the product kernel.
func resolveKernel(spec simulator.ComponentSpec) (kernels.IntegrationKernel, error) {
	reader := newSpecReader("kernel "+spec.Type, spec.Fields)
	var result kernels.IntegrationKernel
	switch spec.Type {
	case "exponential":
		result = &kernels.ExponentialIntegrationKernel{}
	case "periodic":
		result = &kernels.PeriodicIntegrationKernel{}
	case "gaussian_state":
		result = &kernels.GaussianStateIntegrationKernel{}
	case "t_distribution_state":
		result = &kernels.TDistributionStateIntegrationKernel{}
	case "binned":
		result = &kernels.BinnedIntegrationKernel{}
	case "instantaneous":
		result = &kernels.InstantaneousIntegrationKernel{}
	case "constant":
		result = &kernels.ConstantIntegrationKernel{}
	case "product":
		result = &kernels.ProductIntegrationKernel{
			KernelA: reader.kernel("kernel_a"),
			KernelB: reader.kernel("kernel_b"),
		}
	default:
		return nil, fmt.Errorf("kernel: unknown type %q", spec.Type)
	}
	return result, reader.done()
}

// resolveLikelihood builds a LikelihoodDistribution. Only normal carries a data
// field; the rest configure from params.
func resolveLikelihood(spec simulator.ComponentSpec) (inference.LikelihoodDistribution, error) {
	reader := newSpecReader("likelihood "+spec.Type, spec.Fields)
	var result inference.LikelihoodDistribution
	switch spec.Type {
	case "normal":
		result = &inference.NormalLikelihoodDistribution{
			AllowDefaultCovarianceFallback: reader.optBool("allow_default_covariance_fallback"),
		}
	case "t_distribution":
		result = &inference.TLikelihoodDistribution{}
	case "wishart":
		result = &inference.WishartLikelihoodDistribution{}
	case "beta":
		result = &inference.BetaLikelihoodDistribution{}
	case "poisson":
		result = &inference.PoissonLikelihoodDistribution{}
	case "gamma":
		result = &inference.GammaLikelihoodDistribution{}
	case "negative_binomial":
		result = &inference.NegativeBinomialLikelihoodDistribution{}
	default:
		return nil, fmt.Errorf("likelihood: unknown type %q", spec.Type)
	}
	return result, reader.done()
}

// resolveJump builds a JumpDistribution (defined in pkg/continuous).
func resolveJump(spec simulator.ComponentSpec) (continuous.JumpDistribution, error) {
	reader := newSpecReader("jump "+spec.Type, spec.Fields)
	var result continuous.JumpDistribution
	switch spec.Type {
	case "gamma_jump":
		result = &continuous.GammaJumpDistribution{}
	default:
		return nil, fmt.Errorf("jump: unknown type %q", spec.Type)
	}
	return result, reader.done()
}

// resolvePrior builds a Prior for the SMC proposal.
func resolvePrior(spec simulator.ComponentSpec) (inference.Prior, error) {
	reader := newSpecReader("prior "+spec.Type, spec.Fields)
	var result inference.Prior
	switch spec.Type {
	case "uniform":
		result = &inference.UniformPrior{Lo: reader.floatField("lo"), Hi: reader.floatField("hi")}
	case "truncated_normal":
		result = &inference.TruncatedNormalPrior{
			Mu: reader.floatField("mu"), Sigma: reader.floatField("sigma"),
			Lo: reader.floatField("lo"), Hi: reader.floatField("hi"),
		}
	case "half_normal":
		result = &inference.HalfNormalPrior{Sigma: reader.floatField("sigma")}
	case "log_normal":
		result = &inference.LogNormalPrior{Mu: reader.floatField("mu"), Sigma: reader.floatField("sigma")}
	default:
		return nil, fmt.Errorf("prior: unknown type %q", spec.Type)
	}
	return result, reader.done()
}

func (r *specReader) floatField(key string) float64 {
	value, ok := r.value(key, true)
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		r.fail("field %q must be a number, got %T", key, value)
		return 0
	}
}

// ---- named function registries ------------------------------------------------

// valueFunc is the Function field type of the vector mean/covariance iterations.
type valueFunc = func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64

var valueFunctions = map[string]valueFunc{
	"data_values":                  general.DataValuesFunction,
	"data_values_variance":         general.DataValuesVarianceFunction,
	"other_values":                 general.OtherValuesFunction,
	"past_discounted_data_values":  general.PastDiscountedDataValuesFunction,
	"past_discounted_other_values": general.PastDiscountedOtherValuesFunction,
	"unit_value":                   general.UnitValueFunction,
}

// aggregationFunc is the Aggregation field type of the grouped aggregation iteration.
type aggregationFunc = func(
	defaultValues []float64,
	outputIndexByGroup map[string]int,
	groupings map[string][]float64,
	weightings map[string][]float64,
) []float64

var aggregationFunctions = map[string]aggregationFunc{
	"count": general.CountAggregation,
	"sum":   general.SumAggregation,
	"mean":  general.MeanAggregation,
	"max":   general.MaxAggregation,
	"min":   general.MinAggregation,
}

// posteriorTransform is the Transform field type of PosteriorMeanIteration.
type posteriorTransform = func(params *simulator.Params, values mat.Vector) mat.Vector

var posteriorTransforms = map[string]posteriorTransform{
	"mean":     inference.MeanTransform,
	"variance": inference.VarianceTransform,
}

// ---- composable iteration builders --------------------------------------------

func registerComposableIterations() {
	iterationBuilders["compound_poisson_process"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("compound_poisson_process", f)
		it := &continuous.CompoundPoissonProcessIteration{JumpDist: r.jump("jump_dist")}
		return it, r.done()
	}
	iterationBuilders["drift_jump_diffusion"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("drift_jump_diffusion", f)
		it := &continuous.DriftJumpDiffusionIteration{JumpDist: r.jump("jump_dist")}
		return it, r.done()
	}
	iterationBuilders["values_function_vector_mean"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("values_function_vector_mean", f)
		it := &general.ValuesFunctionVectorMeanIteration{
			Function: namedFunc(r, "function", valueFunctions),
			Kernel:   r.kernel("kernel"),
		}
		return it, r.done()
	}
	iterationBuilders["values_function_vector_covariance"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("values_function_vector_covariance", f)
		it := &general.ValuesFunctionVectorCovarianceIteration{
			Function: namedFunc(r, "function", valueFunctions),
			Kernel:   r.kernel("kernel"),
		}
		return it, r.done()
	}
	iterationBuilders["values_grouped_aggregation"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("values_grouped_aggregation", f)
		it := &general.ValuesGroupedAggregationIteration{
			Aggregation: namedFunc(r, "aggregation", aggregationFunctions),
			Kernel:      r.kernel("kernel"),
		}
		return it, r.done()
	}
	iterationBuilders["cumulative"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("cumulative", f)
		it := &general.CumulativeIteration{Iteration: r.iteration("iteration")}
		return it, r.done()
	}
	iterationBuilders["discounted_cumulative"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("discounted_cumulative", f)
		it := &general.DiscountedCumulativeIteration{Iteration: r.iteration("iteration")}
		return it, r.done()
	}
	iterationBuilders["data_generation"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("data_generation", f)
		it := &inference.DataGenerationIteration{Likelihood: r.likelihood("likelihood")}
		return it, r.done()
	}
	iterationBuilders["data_comparison"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("data_comparison", f)
		it := &inference.DataComparisonIteration{Likelihood: r.likelihood("likelihood")}
		return it, r.done()
	}
	iterationBuilders["posterior_mean"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("posterior_mean", f)
		it := &inference.PosteriorMeanIteration{Transform: namedFunc(r, "transform", posteriorTransforms)}
		return it, r.done()
	}
	iterationBuilders["smc_proposal"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("smc_proposal", f)
		it := &inference.SMCProposalIteration{Priors: r.priorList("priors")}
		return it, r.done()
	}
	iterationBuilders["values_function"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("values_function", f)
		transform := namedFunc(r, "transform", valuesTransforms)
		reduce := namedFunc(r, "reduce", valuesReduces)
		it := &general.ValuesFunctionIteration{}
		if r.err == nil {
			it.Function = general.NewTransformReduceFunction(transform, reduce)
		}
		return it, r.done()
	}
	iterationBuilders["values_collection"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("values_collection", f)
		it := &general.ValuesCollectionIteration{
			PopIndex: namedFunc(r, "pop_index", popIndexFunctions),
			Push:     namedFunc(r, "push", pushFunctions),
		}
		return it, r.done()
	}
	iterationBuilders["values_sorting_collection"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("values_sorting_collection", f)
		it := &general.ValuesSortingCollectionIteration{
			PushAndSort: namedFunc(r, "push_and_sort", pushAndSortFunctions),
		}
		return it, r.done()
	}
}

// Collection iteration func-field types and their framework-shipped values.
type popIndexFunc = func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) (int, bool)

type pushFunc = func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) ([]float64, bool)

type pushAndSortFunc = func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) (general.SortingValues, bool)

var popIndexFunctions = map[string]popIndexFunc{
	"next_non_empty": general.NextNonEmptyPopIndexFunction,
}

var pushFunctions = map[string]pushFunc{
	"other_partition": general.OtherPartitionPushFunction,
	"pop_from_other":  general.PopFromOtherCollectionPushFunction,
	"param_values":    general.ParamValuesPushFunction,
}

var pushAndSortFunctions = map[string]pushAndSortFunc{
	"other_partitions": general.OtherPartitionsPushAndSortFunction,
	"param_values":     general.ParamValuesPushAndSortFunction,
}

// valuesTransform / valuesReduce are the building-block types composed by
// general.NewTransformReduceFunction into a ValuesFunctionIteration.Function.
type valuesTransform = func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) map[string][]float64

type valuesReduce = func(values map[string][]float64) []float64

var valuesTransforms = map[string]valuesTransform{
	"params": general.ParamsTransform,
}

var valuesReduces = map[string]valuesReduce{
	"sum": general.SumReduce,
}

func init() {
	registerComposableIterations()
	registerDownstreamComponents()
}

// registerDownstreamComponents registers simulation components that live in
// packages downstream of simulator (so simulator cannot name them itself) into
// the simulator component registry.
func registerDownstreamComponents() {
	// from_history: the embedded-window timestep function whose Data is injected by
	// the embedded-run machinery at runtime, exactly like the Go form.
	simulator.RegisterComponent(
		"timestep_function", "from_history",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			if len(spec.Fields) > 0 {
				return nil, fmt.Errorf("timestep_function from_history takes no fields")
			}
			return &general.FromHistoryTimestepFunction{}, nil
		},
	)
}
