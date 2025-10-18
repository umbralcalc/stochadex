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
	"gonum.org/v1/gonum/mat"
)

// StateHistory is a rolling window of state vectors.
//
// Usage hints:
//   - Values holds rows of state (row 0 is most recent by convention).
//   - Use GetNextStateRowToUpdate when updating in multi-row histories.
type StateHistory struct {
	// each row is a different state in the history, by convention,
	// starting with the most recent at index = 0
	Values *mat.Dense
	// should be of length = StateWidth
	NextValues        []float64
	StateWidth        int
	StateHistoryDepth int
}

// CopyStateRow copies a row from the state history given the index.
func (s *StateHistory) CopyStateRow(index int) []float64 {
	valuesCopy := make([]float64, s.StateWidth)
	copy(valuesCopy, s.Values.RawRowView(index))
	return valuesCopy
}

// GetNextStateRowToUpdate determines whether or not it is necessary
// to copy the previous row or simply expose it based on whether a history
// longer than 1 is needed.
func (s *StateHistory) GetNextStateRowToUpdate() []float64 {
	if s.StateHistoryDepth == 1 {
		return s.Values.RawRowView(0)
	}
	return s.CopyStateRow(0)
}

// CumulativeTimestepsHistory is a rolling window of cumulative timesteps with
// NextIncrement and CurrentStepNumber.
type CumulativeTimestepsHistory struct {
	NextIncrement     float64
	Values            *mat.VecDense
	CurrentStepNumber int
	StateHistoryDepth int
}

// IteratorInputMessage carries shared histories into iterator jobs.
type IteratorInputMessage struct {
	StateHistories   []*StateHistory
	TimestepsHistory *CumulativeTimestepsHistory
}
