package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AggDataRef
type AggDataRef struct {
	PartitionName string
	ValueIndex    int
}

// AggregationConfig
type AggregationConfig struct {
	Name string
	// can be "sum", "count", "mean", "cov", "max", "min"
	Type        string
	WindowDepth int
	Data        []AggDataRef
	GroupBy     []AggDataRef
}

func (a *AggregationConfig) Iteration() simulator.Iteration {
	switch a.Type {
	case "sum":
		return &general.ValuesGroupedAggregationIteration{
			AggFunction: general.SumAggFunction,
		}
	case "count":
		return &general.ValuesGroupedAggregationIteration{
			AggFunction: general.CountAggFunction,
		}
	case "mean":
		return &general.ValuesFunctionWindowedWeightedMeanIteration{
			Function: general.DataValuesFunction,
		}
	case "cov":
		return &general.ValuesFunctionWindowedWeightedCovarianceIteration{
			Function: general.DataValuesFunction,
		}
	case "max":
		return &general.ValuesGroupedAggregationIteration{
			AggFunction: general.MaxAggFunction,
		}
	case "min":
		return &general.ValuesGroupedAggregationIteration{
			AggFunction: general.MinAggFunction,
		}
	}
	panic("iteration not found based on AggregationConfig")
}

// NewAggregationPartition
func NewAggregationPartition(
	config *AggregationConfig,
) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{}
}
