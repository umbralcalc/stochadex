package macros

import (
	"fmt"
	"sync"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SMCParticleEvaluationIteration scores every particle by running one model per
// particle and reading its log-likelihood.
//
// The model is stated once and instantiated per particle, so the particle count
// is a property of the run rather than of the configuration. Seeds are derived
// here, per particle and per round, so no config can leave two particles sharing
// a random stream. Particles are evaluated concurrently and synchronise once per
// round.
//
// # Row layout
//
//	row[p] = the log-likelihood particle p accumulated over the model run.
//
// Params:
//   - particle_params: the flat N*d proposal vector, normally wired from the
//     proposal partition via params_from_upstream.
type SMCParticleEvaluationIteration struct {
	// Simulations holds one independent model per particle. They are separate
	// values because they are evaluated concurrently and a ReentrantSimulation
	// is not safe to share.
	Simulations []*simulator.ReentrantSimulation
	// Forwarding says how a particle's parameter vector reaches its model.
	Forwarding []SMCParamForwarding
	// LoglikeIndex is the partition whose row[0] carries the log-likelihood.
	LoglikeIndex int
	// NumParams is each particle's parameter count.
	NumParams int

	seed uint64
	out  []float64
}

// SMCParamForwarding routes part of a particle's parameter vector into one of
// its model's params.
type SMCParamForwarding struct {
	// Partition and Param name the destination.
	Partition int
	Param     string
	// Indices selects which of the particle's d parameters to send, in order.
	Indices []int
}

// Configure implements simulator.Iteration.
func (s *SMCParticleEvaluationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	if len(s.Simulations) == 0 {
		panic("macros.SMCParticleEvaluationIteration: no particle simulations")
	}
	if s.NumParams <= 0 {
		panic("macros.SMCParticleEvaluationIteration: NumParams must be > 0")
	}
	s.seed = settings.Iterations[partitionIndex].Seed
	s.out = make([]float64, len(s.Simulations))
}

// Iterate implements simulator.Iteration: one round of particle evaluation.
func (s *SMCParticleEvaluationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	particleParams := params.Get("particle_params")
	round := uint64(timestepsHistory.CurrentStepNumber)

	var group sync.WaitGroup
	for particle := range s.Simulations {
		group.Add(1)
		go func(particle int) {
			defer group.Done()
			simulation := s.Simulations[particle]
			offset := particle * s.NumParams
			for _, forward := range s.Forwarding {
				values := make([]float64, len(forward.Indices))
				for i, index := range forward.Indices {
					if slot := offset + index; slot < len(particleParams) {
						values[i] = particleParams[slot]
					}
				}
				simulation.SetParam(forward.Partition, forward.Param, values)
			}
			// Vary the seed by both round and particle: sharing either would
			// put particles on one noise realisation, or repeat the same one
			// every round.
			seed := simulator.DeriveSeed(
				simulator.DeriveSeed(s.seed, int(round)), particle)
			rows := simulation.Run(simulator.ReentrantRun{Seed: &seed})
			s.out[particle] = rows[s.LoglikeIndex][0]
		}(particle)
	}
	group.Wait()
	return s.out
}

// NewSMCParticleEvaluationIteration builds the per-particle evaluator from a
// single-particle model, instantiating it once per particle.
func NewSMCParticleEvaluationIteration(
	build func() *SMCInnerSimConfig,
	numParticles int,
	numParams int,
) *SMCParticleEvaluationIteration {
	simulations := make([]*simulator.ReentrantSimulation, numParticles)
	var forwarding []SMCParamForwarding
	loglikeIndex := -1

	for particle := 0; particle < numParticles; particle++ {
		config := build()
		generator := simulator.NewConfigGenerator()
		generator.SetSimulation(config.Simulation)
		for _, partition := range config.Partitions {
			generator.SetPartition(partition)
		}
		settings, implementations := generator.GenerateConfigs()
		simulations[particle] = simulator.NewReentrantSimulation(settings, implementations)

		if particle > 0 {
			continue
		}
		// The model has the same shape for every particle, so resolve names to
		// indices once against the first instance.
		index, ok := simulations[0].PartitionIndex(config.LoglikePartition)
		if !ok {
			panic(fmt.Sprintf(
				"macros: SMC loglike partition %q is not in the particle model",
				config.LoglikePartition))
		}
		loglikeIndex = index
		forwarding = make([]SMCParamForwarding, 0, len(config.ParamForwarding))
		for target, indices := range config.ParamForwarding {
			partitionName, paramName, ok := splitForwardingTarget(target)
			if !ok {
				panic(fmt.Sprintf(
					"macros: SMC param forwarding key %q must be "+
						"\"partition_name/param_name\"", target))
			}
			partitionIndex, ok := simulations[0].PartitionIndex(partitionName)
			if !ok {
				panic(fmt.Sprintf(
					"macros: SMC param forwarding names partition %q, which is not "+
						"in the particle model", partitionName))
			}
			forwarding = append(forwarding, SMCParamForwarding{
				Partition: partitionIndex,
				Param:     paramName,
				Indices:   indices,
			})
		}
	}

	return &SMCParticleEvaluationIteration{
		Simulations:  simulations,
		Forwarding:   forwarding,
		LoglikeIndex: loglikeIndex,
		NumParams:    numParams,
	}
}

// splitForwardingTarget splits "partition_name/param_name" at the last slash.
func splitForwardingTarget(target string) (partition, param string, ok bool) {
	for i := len(target) - 1; i >= 0; i-- {
		if target[i] == '/' {
			return target[:i], target[i+1:], i > 0 && i < len(target)-1
		}
	}
	return "", "", false
}
