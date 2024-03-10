package params

import (
	"github.com/mkmik/argsort"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ParamsMapping is a struct which ensures that the configured params of the
// simulator can be correctly mapped to and from a generic flattened format.
type ParamsMapping struct {
	Names            []string
	ParamIndices     []int
	FlattenedIndices []int
}

func (p *ParamsMapping) SortByOptimiserIndices() {
	sortedIndices := argsort.SortSlice(
		p.FlattenedIndices,
		func(i, j int) bool {
			return p.FlattenedIndices[i] < p.FlattenedIndices[j]
		},
	)
	sortedNames := p.Names
	sortedParamIndices := p.ParamIndices
	sortedFlattenedIndices := p.FlattenedIndices
	for i, sortedIndex := range sortedIndices {
		sortedNames[i] = p.Names[sortedIndex]
		sortedParamIndices[i] = p.ParamIndices[sortedIndex]
		sortedFlattenedIndices[i] = p.FlattenedIndices[sortedIndex]
	}
	p.Names = sortedNames
	p.ParamIndices = sortedParamIndices
	p.FlattenedIndices = sortedFlattenedIndices
}

// ParamsMappings provides a ParamsMapping for both .FloatParams and
// .IntParams in the simulator.OtherParams struct.
type ParamsMappings struct {
	FloatParams *ParamsMapping
	IntParams   *ParamsMapping
}

// GetParamsFlattened is a convenience function which returns the params
// from the stochadex where the mask has been applied to them in a flattened
// single slice format.
func (p *ParamsMappings) GetParamsFlattened(
	params *simulator.OtherParams,
) []float64 {
	flattenedParams := make([]float64, 0)
	for i, name := range p.FloatParams.Names {
		flattenedParams = append(
			flattenedParams,
			params.FloatParams[name][p.FloatParams.ParamIndices[i]],
		)
	}
	for i, name := range p.IntParams.Names {
		flattenedParams = append(
			flattenedParams,
			float64(
				params.IntParams[name][p.IntParams.ParamIndices[i]],
			),
		)
	}
	return flattenedParams
}

// UpdateParamsFromFlattened is a convenience function which updates the
// input params from the stochadex which have been retrieved from the flattened
// slice format.
func (p *ParamsMappings) UpdateParamsFromFlattened(
	fromFlattened []float64,
	params *simulator.OtherParams,
) *simulator.OtherParams {
	largestFloatParamIndex :=
		p.FloatParams.FlattenedIndices[len(p.FloatParams.FlattenedIndices)-1]
	for flattenedIndex, param := range fromFlattened {
		if flattenedIndex <= largestFloatParamIndex {
			name := p.FloatParams.Names[flattenedIndex]
			index := p.FloatParams.ParamIndices[flattenedIndex]
			params.FloatParams[name][index] = param
		} else {
			name := p.IntParams.Names[flattenedIndex]
			index := p.IntParams.ParamIndices[flattenedIndex]
			params.IntParams[name][index] = int64(param)
		}
	}
	return params
}

// NewParamsMappings creates a new ParamsMappings.
func NewParamsMappings(params *simulator.OtherParams) *ParamsMappings {
	flattenedParamsIndex := 0
	floatParamsMapping := &ParamsMapping{
		Names:            make([]string, 0),
		FlattenedIndices: make([]int, 0),
	}
	for name, paramSlice := range params.FloatParams {
		_, ok := params.FloatParamsMask[name]
		if !ok {
			continue
		}
		for i := range paramSlice {
			if params.FloatParamsMask[name][i] {
				floatParamsMapping.Names = append(floatParamsMapping.Names, name)
				floatParamsMapping.ParamIndices = append(
					floatParamsMapping.ParamIndices,
					i,
				)
				floatParamsMapping.FlattenedIndices = append(
					floatParamsMapping.FlattenedIndices,
					flattenedParamsIndex,
				)
				flattenedParamsIndex += 1
			}
		}
	}
	intParamsMapping := &ParamsMapping{
		Names:            make([]string, 0),
		FlattenedIndices: make([]int, 0),
	}
	for name, paramSlice := range params.IntParams {
		_, ok := params.IntParamsMask[name]
		if !ok {
			continue
		}
		for i := range paramSlice {
			if params.IntParamsMask[name][i] {
				intParamsMapping.Names = append(intParamsMapping.Names, name)
				intParamsMapping.ParamIndices = append(
					intParamsMapping.ParamIndices,
					i,
				)
				intParamsMapping.FlattenedIndices = append(
					intParamsMapping.FlattenedIndices,
					flattenedParamsIndex,
				)
				flattenedParamsIndex += 1
			}
		}
	}
	floatParamsMapping.SortByOptimiserIndices()
	intParamsMapping.SortByOptimiserIndices()
	return &ParamsMappings{
		FloatParams: floatParamsMapping,
		IntParams:   intParamsMapping,
	}
}
