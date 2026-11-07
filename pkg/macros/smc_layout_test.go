package macros

import (
	"reflect"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestSMCLayout(t *testing.T) {
	priors := []inference.Prior{
		&inference.UniformPrior{Lo: 1, Hi: 3},
		&inference.HalfNormalPrior{Sigma: 0.5},
	}
	for _, c := range []struct {
		name    string
		verbose bool
		flag    float64
	}{{"a verbose run", true, 1}, {"a quiet run", false, 0}} {
		t.Run(c.name, func(t *testing.T) {
			layout := NewSMCLayout("proposal", "sim", "posterior", 3, priors, c.verbose, 7)
			want := SMCLayout{
				Proposal: InnerLayout{
					Params: map[string][]float64{
						"verbose":       {c.flag},
						"num_particles": {3},
						"prior_types":   {0, 2},
						"prior_params":  {1, 3, 0, 0, 0.5, 0, 0, 0},
					},
					ParamsAsPartitions: map[string][]string{"posterior_partition": {"posterior"}},
					InitStateValues:    []float64{0, 0, 0, 0, 0, 0},
				},
				Evaluation: InnerLayout{
					Params: map[string][]float64{"replica_params": {0, 0, 0, 0, 0, 0}},
					ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
						"replica_params": {Upstream: "proposal"}},
					InitStateValues: []float64{0, 0, 0},
				},
				Posterior: InnerLayout{
					Params: map[string][]float64{
						"verbose":           {c.flag},
						"num_particles":     {3},
						"num_params":        {2},
						"particle_loglikes": {0, 0, 0},
						"particle_params":   {0, 0, 0, 0, 0, 0},
					},
					ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
						"particle_loglikes": {Upstream: "sim"},
						"particle_params":   {Upstream: "proposal"},
					},
					// A zero mean, the identity covariance, then the log normalisation.
					InitStateValues: []float64{0, 0, 1, 0, 0, 1, 0},
				},
				HistoryDepth:   2,
				ProposalSeed:   7,
				EvaluationSeed: 107,
			}
			if !reflect.DeepEqual(layout, want) {
				t.Errorf("got  %+v\nwant %+v", layout, want)
			}
		})
	}

	t.Run("the partitions read it", func(t *testing.T) {
		applied := AppliedSMCInference{
			ProposalName: "proposal", SimName: "sim", PosteriorName: "posterior",
			NumParticles: 3, Priors: priors, Seed: 7,
			Model: SMCParticleModel{Build: func(int) *SMCInnerSimConfig {
				return &SMCInnerSimConfig{
					Partitions: []*simulator.PartitionConfig{{Name: "loglike",
						Iteration: &general.ConstantValuesIteration{},
						Params:    simulator.NewParams(map[string][]float64{}), InitStateValues: []float64{0},
						StateHistoryDepth: 1}},
					Simulation: &simulator.SimulationConfig{
						OutputCondition:      &simulator.NilOutputCondition{},
						OutputFunction:       &simulator.NilOutputFunction{},
						TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
						TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1},
					},
					LoglikePartition: "loglike",
				}
			}},
		}
		layout := NewSMCLayout("proposal", "sim", "posterior", 3, priors, false, 7)
		partitions := NewSMCInferencePartitions(applied)
		for i, inner := range []InnerLayout{layout.Proposal, layout.Evaluation, layout.Posterior} {
			partition := partitions[i]
			if !reflect.DeepEqual(partition.Params.Map, inner.Params) ||
				!reflect.DeepEqual(partition.ParamsFromUpstream, inner.ParamsFromUpstream) ||
				!reflect.DeepEqual(partition.InitStateValues, inner.InitStateValues) ||
				partition.StateHistoryDepth != 2 {
				t.Errorf("partition %s does not read its layout", partition.Name)
			}
		}
		if partitions[0].Seed != 7 || partitions[1].Seed != 107 || partitions[2].Seed != 0 {
			t.Errorf("seeds %d, %d, %d", partitions[0].Seed, partitions[1].Seed, partitions[2].Seed)
		}
	})
}
