// Package twins runs a catalogue model's builds and compares them: the Go stub, its
// declarative twin (every partition an expression) and its building-block twin (built-in
// iterations wherever one states a partition's update). Builds that draw the same
// streams in the same order agree step for step, so a twin is checked against another by
// comparing whole runs.
package twins

import (
	"fmt"
	"math"
	"sort"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Run runs a build to termination, recording every step of every partition.
func Run(gen *simulator.ConfigGenerator) *simulator.StateTimeStorage {
	storage := simulator.NewStateTimeStorage()
	simulation := gen.GetSimulation()
	simulation.OutputCondition = &simulator.EveryStepOutputCondition{}
	simulation.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: storage}
	gen.SetSimulation(simulation)
	simulator.NewPartitionCoordinator(gen.GenerateConfigs()).Run()
	return storage
}

// WithStepsize sets a build's clock to a constant stepsize, and returns the build.
func WithStepsize(gen *simulator.ConfigGenerator, stepsize float64) *simulator.ConfigGenerator {
	simulation := gen.GetSimulation()
	simulation.TimestepFunction = &simulator.ConstantTimestepFunction{Stepsize: stepsize}
	gen.SetSimulation(simulation)
	return gen
}

// RunQuietly runs a build to termination, recording nothing, as a benchmark of its
// arithmetic wants.
func RunQuietly(gen *simulator.ConfigGenerator) {
	simulation := gen.GetSimulation()
	simulation.OutputCondition = &simulator.NilOutputCondition{}
	simulation.OutputFunction = &simulator.NilOutputFunction{}
	gen.SetSimulation(simulation)
	simulator.NewPartitionCoordinator(gen.GenerateConfigs()).Run()
}

// Compare reports how got differs from want: a partition or step missing, or a value
// further than tolerance from want's, relative to its size (or absolute below 1). It also
// reports want's state never moving, since agreement then shows nothing.
func Compare(got, want *simulator.StateTimeStorage, tolerance float64) error {
	names := want.GetNames()
	sort.Strings(names)
	moved := false
	for _, name := range names {
		wantRows, gotRows := want.GetValues(name), got.GetValues(name)
		if len(gotRows) != len(wantRows) {
			return fmt.Errorf("partition %s: %d rows, want %d", name, len(gotRows), len(wantRows))
		}
		for step, wantRow := range wantRows {
			if len(gotRows[step]) != len(wantRow) {
				return fmt.Errorf("partition %s, step %d: width %d, want %d",
					name, step, len(gotRows[step]), len(wantRow))
			}
			for i, value := range wantRow {
				if math.Abs(gotRows[step][i]-value) > tolerance*math.Max(1, math.Abs(value)) {
					return fmt.Errorf("partition %s, step %d, element %d: %v, want %v",
						name, step, i, gotRows[step][i], value)
				}
				moved = moved || value != wantRows[0][i]
			}
		}
	}
	if !moved {
		return fmt.Errorf("no partition's state moved, so the builds' agreement shows nothing")
	}
	return nil
}
