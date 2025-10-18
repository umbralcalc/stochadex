// Package simulator provides the core simulation engine and infrastructure
// for stochadex simulations. It includes the main simulation loop, state management,
// partition coordination, and execution control mechanisms.
//
// Key Features:
//   - Partition-based simulation architecture
//   - Concurrent execution with goroutine coordination
//   - State history management and time tracking
//   - Configurable termination and output conditions
//   - Flexible timestep control
//   - Thread-safe state storage and communication
//
// Architecture Overview:
// The simulator uses a partition-based architecture where each partition
// represents a component of the simulation state. Partitions can communicate
// through upstream/downstream channels, enabling complex multi-component
// simulations with dependencies between components.
//
// Core Components:
//   - PartitionCoordinator: Orchestrates execution across all partitions
//   - StateIterator: Manages individual partition execution and communication
//   - StateTimeStorage: Thread-safe storage for simulation results
//   - ConfigGenerator: Creates simulation configurations from settings
//   - TerminationCondition: Controls when simulations stop
//   - OutputFunction: Handles result collection and storage
//
// Design Philosophy:
// The simulator emphasizes modularity, concurrency, and flexibility. It provides
// a robust foundation for building complex simulations while maintaining good
// performance characteristics and thread safety.
//
// Usage Patterns:
//   - Multi-component system simulation
//   - Agent-based modeling with interactions
//   - Monte Carlo simulations with multiple sources of randomness
//   - Time-series analysis and forecasting
//   - Parameter estimation and optimization
package simulator

import "fmt"

// Params stores per-partition parameter values.
//
// Usage hints:
//   - Use Get/GetIndex helpers to retrieve, Set/SetIndex to update.
//   - SetPartitionName improves error messages for missing params.
type Params struct {
	Map           map[string][]float64 `yaml:",inline"`
	partitionName string               `yaml:"-"`
}

// GetOk returns parameter values if present along with a boolean flag.
func (p *Params) GetOk(name string) ([]float64, bool) {
	values, ok := p.Map[name]
	return values, ok
}

// GetCopyOk returns a copy of parameter values if present along with a flag.
func (p *Params) GetCopyOk(name string) ([]float64, bool) {
	if values, ok := p.Map[name]; ok {
		valuesCopy := make([]float64, len(values))
		copy(valuesCopy, values)
		return valuesCopy, ok
	} else {
		return nil, ok
	}
}

// Get returns parameter values or panics with a helpful message.
func (p *Params) Get(name string) []float64 {
	if values, ok := p.Map[name]; ok {
		return values
	} else {
		panic("partition: " + p.partitionName +
			" does not have params set for: " + name)
	}
}

// GetCopy returns a copy of parameter values or panics with a helpful message.
func (p *Params) GetCopy(name string) []float64 {
	if values, ok := p.Map[name]; ok {
		valuesCopy := make([]float64, len(values))
		copy(valuesCopy, values)
		return valuesCopy
	} else {
		panic("partition: " + p.partitionName +
			" does not have params set for: " + name)
	}
}

// GetIndex returns a single parameter value or panics.
func (p *Params) GetIndex(name string, index int) float64 {
	if values, ok := p.Map[name]; ok {
		return values[index]
	} else {
		panic("partition: " + p.partitionName +
			" does not have params set for: " + name)
	}
}

// Set creates or updates parameter values by name.
func (p *Params) Set(name string, values []float64) {
	p.Map[name] = values
}

// SetIndex updates a single parameter value or panics on invalid index.
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

// SetPartitionName attaches the owning partition name for better errors.
func (p *Params) SetPartitionName(name string) {
	p.partitionName = name
}

// NewParams constructs a Params instance.
func NewParams(params map[string][]float64) Params {
	return Params{
		Map:           params,
		partitionName: "<name not set>",
	}
}
