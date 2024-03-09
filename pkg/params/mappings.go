package params

import (
	"github.com/mkmik/argsort"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// OptimiserParamsMapping is a struct which ensures that the configured
// params of the simulator can be correctly mapped to and from a generic
// optimisation algorithm.
type OptimiserParamsMapping struct {
	Names            []string
	ParamIndices     []int
	PartitionIndices []int
	OptimiserIndices []int
}

func (o *OptimiserParamsMapping) SortByOptimiserIndices() {
	sortedIndices := argsort.SortSlice(
		o.OptimiserIndices,
		func(i, j int) bool {
			return o.OptimiserIndices[i] < o.OptimiserIndices[j]
		},
	)
	sortedNames := o.Names
	sortedParamIndices := o.ParamIndices
	sortedPartitionIndices := o.PartitionIndices
	sortedOptimiserIndices := o.OptimiserIndices
	for i, sortedIndex := range sortedIndices {
		sortedNames[i] = o.Names[sortedIndex]
		sortedParamIndices[i] = o.ParamIndices[sortedIndex]
		sortedPartitionIndices[i] = o.PartitionIndices[sortedIndex]
		sortedOptimiserIndices[i] = o.OptimiserIndices[sortedIndex]
	}
	o.Names = sortedNames
	o.ParamIndices = sortedParamIndices
	o.PartitionIndices = sortedPartitionIndices
	o.OptimiserIndices = sortedOptimiserIndices
}

// OptimiserParamsMappings provides a OptimiserParamsMapping for both
// .FloatParams and .IntParams in the simulator.OtherParams struct.
type OptimiserParamsMappings struct {
	FloatParams *OptimiserParamsMapping
	IntParams   *OptimiserParamsMapping
}

// GetParamsForOptimiser is a convenience function which returns the params
// from the stochadex where the mask has been applied to them in a flattened
// single slice format ready to input into an optimiser.
func (o *OptimiserParamsMappings) GetParamsForOptimiser(
	params []*simulator.OtherParams,
) []float64 {
	paramsForOptimiser := make([]float64, 0)
	for i, name := range o.FloatParams.Names {
		paramsForOptimiser = append(
			paramsForOptimiser,
			params[o.FloatParams.PartitionIndices[i]].
				FloatParams[name][o.FloatParams.ParamIndices[i]],
		)
	}
	for i, name := range o.IntParams.Names {
		paramsForOptimiser = append(
			paramsForOptimiser,
			float64(
				params[o.IntParams.PartitionIndices[i]].
					IntParams[name][o.IntParams.ParamIndices[i]],
			),
		)
	}
	return paramsForOptimiser
}

// UpdateParamsFromOptimiser is a convenience function which updates the input params
// from the stochadex which have been retrieved from the flattened slice format that
// is typically used in an optimiser package.
func (o *OptimiserParamsMappings) UpdateParamsFromOptimiser(
	fromOptimiser []float64,
	params []*simulator.OtherParams,
) []*simulator.OtherParams {
	largestFloatParamIndex :=
		o.FloatParams.OptimiserIndices[len(o.FloatParams.OptimiserIndices)-1]
	for optimiserIndex, param := range fromOptimiser {
		if optimiserIndex <= largestFloatParamIndex {
			partition := o.FloatParams.PartitionIndices[optimiserIndex]
			name := o.FloatParams.Names[optimiserIndex]
			index := o.FloatParams.ParamIndices[optimiserIndex]
			params[partition].FloatParams[name][index] = param
		} else {
			partition := o.IntParams.PartitionIndices[optimiserIndex]
			name := o.IntParams.Names[optimiserIndex]
			index := o.IntParams.ParamIndices[optimiserIndex]
			params[partition].IntParams[name][index] = int64(param)
		}
	}
	return params
}

// NewOptimiserParamsMappings creates a new OptimiserParamsMappings.
func NewOptimiserParamsMappings(
	params []*simulator.OtherParams,
) *OptimiserParamsMappings {
	optimiserParamsIndex := 0
	floatParamsMapping := &OptimiserParamsMapping{
		Names:            make([]string, 0),
		PartitionIndices: make([]int, 0),
		OptimiserIndices: make([]int, 0),
	}
	for index, partitionParams := range params {
		for name, paramSlice := range partitionParams.FloatParams {
			_, ok := partitionParams.FloatParamsMask[name]
			if !ok {
				continue
			}
			for i := range paramSlice {
				if partitionParams.FloatParamsMask[name][i] {
					floatParamsMapping.Names = append(floatParamsMapping.Names, name)
					floatParamsMapping.ParamIndices = append(
						floatParamsMapping.ParamIndices,
						i,
					)
					floatParamsMapping.PartitionIndices = append(
						floatParamsMapping.PartitionIndices,
						index,
					)
					floatParamsMapping.OptimiserIndices = append(
						floatParamsMapping.OptimiserIndices,
						optimiserParamsIndex,
					)
					optimiserParamsIndex += 1
				}
			}
		}
	}
	intParamsMapping := &OptimiserParamsMapping{
		Names:            make([]string, 0),
		PartitionIndices: make([]int, 0),
		OptimiserIndices: make([]int, 0),
	}
	for index, partitionParams := range params {
		for name, paramSlice := range partitionParams.IntParams {
			_, ok := partitionParams.IntParamsMask[name]
			if !ok {
				continue
			}
			for i := range paramSlice {
				if partitionParams.IntParamsMask[name][i] {
					intParamsMapping.Names = append(intParamsMapping.Names, name)
					intParamsMapping.ParamIndices = append(
						intParamsMapping.ParamIndices,
						i,
					)
					intParamsMapping.PartitionIndices = append(
						intParamsMapping.PartitionIndices,
						index,
					)
					intParamsMapping.OptimiserIndices = append(
						intParamsMapping.OptimiserIndices,
						optimiserParamsIndex,
					)
					optimiserParamsIndex += 1
				}
			}
		}
	}
	floatParamsMapping.SortByOptimiserIndices()
	intParamsMapping.SortByOptimiserIndices()
	return &OptimiserParamsMappings{
		FloatParams: floatParamsMapping,
		IntParams:   intParamsMapping,
	}
}
