package macros

import (
	"maps"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PosteriorSamplerWiring is a posterior's sampler as data: its name, its first draw,
// and its distribution's params and wiring.
type PosteriorSamplerWiring struct {
	Name               string
	Default            []float64
	Params             map[string][]float64
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// PosteriorLayout is the data a posterior's partitions are wired with, all but its
// comparison, which a WindowLayout lays out: everything about them but their names
// and iterations. The constructors here and a config's expansion both read it.
type PosteriorLayout struct {
	LogNorm, Mean, Covariance, Sampler InnerLayout
}

// NewPosteriorLayout is the layout of a posterior whose log-likelihoods are element
// loglikeIndex of the comparison partition's state, discounted by pastDiscount a step
// into the past. With covariance.JustVariance it keeps a variance per element rather
// than a covariance matrix, and the sampler reads that variance in place of a matrix.
func NewPosteriorLayout(
	logNorm PosteriorLogNorm,
	mean PosteriorMean,
	covariance PosteriorCovariance,
	sampler PosteriorSamplerWiring,
	comparison string,
	loglikeIndex int,
	pastDiscount float64,
) PosteriorLayout {
	indices := []float64{float64(loglikeIndex)}
	loglikes := []string{comparison}
	weighted := map[string][]string{"loglike_partitions": loglikes, "param_partitions": {sampler.Name}}
	normalisation := simulator.NamedUpstreamConfig{Upstream: logNorm.Name}
	samplerAsPartitions := maps.Clone(sampler.ParamsAsPartitions)
	samplerUpstreams := maps.Clone(sampler.ParamsFromUpstream)
	if covariance.JustVariance {
		if samplerAsPartitions == nil {
			samplerAsPartitions = map[string][]string{}
		}
		samplerAsPartitions["variance_partition"] = []string{covariance.Name}
		delete(samplerUpstreams, "covariance_matrix")
	}
	return PosteriorLayout{
		LogNorm: InnerLayout{
			Params: map[string][]float64{"loglike_indices": indices,
				"past_discounting_factor": {pastDiscount}},
			ParamsAsPartitions: map[string][]string{"loglike_partitions": loglikes},
			InitStateValues:    []float64{logNorm.Default},
		},
		Mean: InnerLayout{
			Params:             map[string][]float64{"loglike_indices": indices},
			ParamsAsPartitions: weighted,
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"posterior_log_normalisation": normalisation},
			InitStateValues: mean.Default,
		},
		Covariance: InnerLayout{
			Params:             map[string][]float64{"loglike_indices": indices},
			ParamsAsPartitions: maps.Clone(weighted),
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"posterior_log_normalisation": normalisation, "mean": {Upstream: mean.Name}},
			InitStateValues: covariance.Default,
		},
		Sampler: InnerLayout{
			Params:             cloneOrEmpty(sampler.Params),
			ParamsAsPartitions: samplerAsPartitions,
			ParamsFromUpstream: samplerUpstreams,
			InitStateValues:    sampler.Default,
		},
	}
}

// PosteriorReadsSampler reports whether a posterior's comparison reads its sampler,
// through the model's upstreams or a window partition's outside upstreams. If it does
// not, every draw scores the same, the weights are uniform, and the posterior cannot
// converge.
func PosteriorReadsSampler(
	sampler string,
	model map[string]simulator.NamedUpstreamConfig,
	window []map[string]simulator.NamedUpstreamConfig,
) bool {
	for _, upstreams := range append([]map[string]simulator.NamedUpstreamConfig{model}, window...) {
		for _, upstream := range upstreams {
			if upstream.Upstream == sampler {
				return true
			}
		}
	}
	return false
}
