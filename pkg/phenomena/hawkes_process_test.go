package phenomena

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// exponentialExcitingKernel weights the historic Hawkes process increments
// with an exponential function - this is just for testing.
type exponentialExcitingKernel struct{}

func (e *exponentialExcitingKernel) Evaluate(
	otherParams *simulator.OtherParams,
	currentTime float64,
	somePreviousTime float64,
	stateElement int,
) float64 {
	return math.Exp(
		-otherParams.FloatParams["exponential_decays"][stateElement] *
			(currentTime - somePreviousTime))
}

func TestHawkesProcess(t *testing.T) {
	t.Run(
		"test that the Hawkes process runs",
		func(t *testing.T) {
			settings := simulator.NewLoadSettingsConfigFromYaml(
				"hawkes_process_config.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			hawkesIteration := &HawkesProcessIteration{}
			hawkesIteration.Configure(0, settings)
			iterations = append(iterations, hawkesIteration)
			intensityIteration := &HawkesProcessIntensityIteration{
				excitingKernel: &exponentialExcitingKernel{},
			}
			intensityIteration.Configure(1, settings)
			iterations = append(iterations, intensityIteration)
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.VariableStoreOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 250,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			config := simulator.NewStochadexConfig(
				settings,
				implementations,
			)
			coordinator := simulator.NewPartitionCoordinator(config)
			coordinator.Run()
		},
	)
}
