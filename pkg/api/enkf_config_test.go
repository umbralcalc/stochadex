package api

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestEnKFConfigFilters is the config-surface behaviour test for the shipped EnKF
// recipe (cfg/example_enkf_config.yaml): it runs the full assimilation loop expressed
// entirely as config and checks the analysis ensemble mean tracks the hidden truth
// with lower error than the raw noisy observations carry. This validates the config
// WIRING — the forecast<->enkf cycle broken by the lag-1 read, and the within-step
// observation of the current truth — beyond merely running. The update maths and the
// steady-state variance are pinned separately in pkg/inference.
func TestEnKFConfigFilters(t *testing.T) {
	config := LoadApiRunConfigFromYaml("../../cfg/example_enkf_config.yaml")
	generator := config.GetConfigGenerator()

	// Redirect output to a storage sink so the run is inspectable (the shipped config
	// prints to stdout for the CLI).
	store := simulator.NewStateTimeStorage()
	sim := generator.GetSimulation()
	sim.OutputCondition = &simulator.EveryStepOutputCondition{}
	sim.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}

	settings, implementations := generator.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, implementations).Run()

	truth := store.GetValues("truth")
	obs := store.GetValues("observations")
	enkf := store.GetValues("enkf")
	if len(truth) == 0 || len(enkf) == 0 {
		t.Fatal("no output recorded")
	}

	const burnIn = 30
	var analysisSqErr, obsSqErr float64
	var counted int
	for s := burnIn; s < len(truth); s++ {
		// Analysis estimate: mean over the ensemble members.
		mean := 0.0
		for _, v := range enkf[s] {
			mean += v
		}
		mean /= float64(len(enkf[s]))
		analysisSqErr += (mean - truth[s][0]) * (mean - truth[s][0])
		obsSqErr += (obs[s][0] - truth[s][0]) * (obs[s][0] - truth[s][0])
		counted++
	}
	analysisRMSE := math.Sqrt(analysisSqErr / float64(counted))
	obsRMSE := math.Sqrt(obsSqErr / float64(counted))
	t.Logf("config EnKF analysis RMSE=%.4f  observation RMSE=%.4f", analysisRMSE, obsRMSE)

	if analysisRMSE >= obsRMSE {
		t.Errorf("analysis RMSE %.4f not below observation RMSE %.4f — the config wiring "+
			"is not assimilating", analysisRMSE, obsRMSE)
	}
}
