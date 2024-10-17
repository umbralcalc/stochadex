package simulator

import "fmt"

// Params is a type alias for the parameters needed to configure
// the stochastic process.
type Params struct {
	Map           map[string][]float64 `yaml:",inline"`
	partitionName string               `yaml:"-"`
}

// GetOk retrieves the desired parameter values given their name,
// returning the values and a boolean indicating if the name was found.
func (p *Params) GetOk(name string) ([]float64, bool) {
	values, ok := p.Map[name]
	return values, ok
}

// Get retrieves the desired parameter values given their name, or
// panics giving a useful error message.
func (p *Params) Get(name string) []float64 {
	if values, ok := p.Map[name]; ok {
		return values
	} else {
		panic("partition: " + p.partitionName +
			" does not have params set for: " + name)
	}
}

// GetIndex retrieves the desired parameter value given the params
// name and index of the value itself, or panics giving a useful
// error message.
func (p *Params) GetIndex(name string, index int) float64 {
	if values, ok := p.Map[name]; ok {
		return values[index]
	} else {
		panic("partition: " + p.partitionName +
			" does not have params set for: " + name)
	}
}

// Set creates or updates the desired parameter values given their name.
func (p *Params) Set(name string, values []float64) {
	p.Map[name] = values
}

// Set creates or updates the desired parameter value given the params
// name and index of the value itself, or panics giving a useful
// error message.
func (p *Params) SetIndex(name string, index int, value float64) {
	values, ok := p.Map[name]
	if !ok {
		panic("partition: " + p.partitionName +
			" does not have params set for: " + name)
	}
	if index < 0 || index >= len(values) {
		panic("partition: " + p.partitionName +
			", param: " + name +
			", index out of range: " + fmt.Sprint(index) +
			", valid range: 0-" + fmt.Sprint(len(values)-1))
	}
	p.Map[name][index] = value
}

// SetPartitionName sets the partition name that these params are
// associated to, mainly for providing more informative error messages.
func (p *Params) SetPartitionName(name string) {
	p.partitionName = name
}

// NewParams creates a new Params struct.
func NewParams(params map[string][]float64) Params {
	return Params{
		Map:           params,
		partitionName: "<name not set>",
	}
}
