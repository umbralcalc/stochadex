package rugby

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Substitution covariate dimensions (lifted from the downstream data layer).
const (
	SubCovWidth       = 8 // 4 position groups × 2 teams
	NumPositionGroups = 4 // front row, back row, halves, outside backs
)

// NewSubCovariatesReplayPartition creates a partition that replays
// observed substitution covariates from pre-computed data.
func NewSubCovariatesReplayPartition(covariates [][]float64) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name: "sub_covariates",
		Iteration: &general.FromStorageIteration{
			Data: covariates,
		},
		Params:            simulator.NewParams(make(map[string][]float64)),
		InitStateValues:   make([]float64, SubCovWidth),
		StateHistoryDepth: 1,
		Seed:              0,
	}
}

// NewSubCovariatesConstantPartition creates a partition that returns
// fixed covariate values at every time step. Pass make([]float64, SubCovWidth)
// for a "no substitutions" baseline.
func NewSubCovariatesConstantPartition(values []float64) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name:      "sub_covariates",
		Iteration: &general.ParamValuesIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"param_values": values,
		}),
		InitStateValues:   make([]float64, SubCovWidth),
		StateHistoryDepth: 1,
		Seed:              0,
	}
}

// SubstitutionStrategy defines when each position group gets substituted.
// Each entry is the minute at which to substitute that group.
// A value of 0 means "don't substitute this group".
type SubstitutionStrategy struct {
	// HomeSubs[i] = minute at which to sub home position group i.
	// [front_row, back_row, halves, outside_backs]
	HomeSubs [NumPositionGroups]int
	// AwaySubs[i] = minute at which to sub away position group i.
	AwaySubs [NumPositionGroups]int
}

// TotalSubs returns the total number of substitutions in this strategy
// (entries > 0).
func (s *SubstitutionStrategy) TotalSubs() int {
	n := 0
	for _, m := range s.HomeSubs {
		if m > 0 {
			n++
		}
	}
	for _, m := range s.AwaySubs {
		if m > 0 {
			n++
		}
	}
	return n
}

// GenerateCovariates converts the strategy into a per-minute binary
// covariate matrix for nSteps minutes (rows 0..nSteps-1).
// Layout matches BuildSubstitutionCovariates:
// [home_front_row, home_back_row, home_halves, home_outside_backs,
//
//	away_front_row, away_back_row, away_halves, away_outside_backs]
func (s *SubstitutionStrategy) GenerateCovariates(nSteps int) [][]float64 {
	covariates := make([][]float64, nSteps)
	for t := 0; t < nSteps; t++ {
		row := make([]float64, SubCovWidth)
		for g := 0; g < NumPositionGroups; g++ {
			if s.HomeSubs[g] > 0 && t >= s.HomeSubs[g] {
				row[g] = 1.0
			}
			if s.AwaySubs[g] > 0 && t >= s.AwaySubs[g] {
				row[NumPositionGroups+g] = 1.0
			}
		}
		covariates[t] = row
	}
	return covariates
}

// NewSubCovariatesFromStrategyPartition creates a partition that replays
// covariates generated from a SubstitutionStrategy. The covariate matrix
// has nSteps+1 rows to cover the initial state plus all simulation steps.
func NewSubCovariatesFromStrategyPartition(
	strategy *SubstitutionStrategy,
	nSteps int,
) *simulator.PartitionConfig {
	return NewSubCovariatesReplayPartition(strategy.GenerateCovariates(nSteps + 1))
}
