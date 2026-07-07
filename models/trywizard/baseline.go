package rugby

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NewBaselineRatesConstantPartition creates a partition that returns zero
// baseline rates at every step. When the rate functions see a zero baseline
// they fall back to the pure exp(intercept + covariates) model, so this is the
// data-free stand-in for the downstream's adaptive-bandwidth kernel-smoothed,
// time-varying baseline (which is a data-fitting concern and stays downstream).
func NewBaselineRatesConstantPartition() *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name:      "baseline_rates",
		Iteration: &general.ParamValuesIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"param_values": make([]float64, RateEventWidth),
		}),
		InitStateValues:   make([]float64, RateEventWidth),
		StateHistoryDepth: 1,
		Seed:              0,
	}
}
