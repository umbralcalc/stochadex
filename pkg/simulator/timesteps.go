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

import (
	"math/rand/v2"

	"gonum.org/v1/gonum/stat/distuv"
)

// TimestepFunction computes the next time increment.
type TimestepFunction interface {
	NextIncrement(
		timestepsHistory *CumulativeTimestepsHistory,
	) float64
}

// ConstantTimestepFunction uses a fixed stepsize.
type ConstantTimestepFunction struct {
	Stepsize float64
}

func (t *ConstantTimestepFunction) NextIncrement(
	timestepsHistory *CumulativeTimestepsHistory,
) float64 {
	return t.Stepsize
}

// ExponentialDistributionTimestepFunction draws dt from an exponential
// distribution parameterised by Mean and Seed.
type ExponentialDistributionTimestepFunction struct {
	Mean         float64
	Seed         uint64
	distribution distuv.Exponential
}

func (t *ExponentialDistributionTimestepFunction) NextIncrement(
	timestepsHistory *CumulativeTimestepsHistory,
) float64 {
	return t.distribution.Rand()
}

// NewExponentialDistributionTimestepFunction constructs an exponential-dt
// timestep function given mean and seed.
func NewExponentialDistributionTimestepFunction(
	mean float64,
	seed uint64,
) *ExponentialDistributionTimestepFunction {
	return &ExponentialDistributionTimestepFunction{
		Mean: mean,
		Seed: seed,
		distribution: distuv.Exponential{
			Rate: 1.0 / mean,
			Src:  rand.NewPCG(seed, seed),
		},
	}
}
