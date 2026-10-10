package simulator

import "errors"

// OutputView is one named output: a sink gated by its own condition. A config's
// outputs: list is a set of views; the classic output_condition/output_function
// pair is a single unnamed view.
type OutputView struct {
	Name      string
	Condition OutputCondition
	Function  OutputFunction
}

// OutputViews tees every output step to each view whose own condition selects
// it, so one run can feed several sinks with different filters (a full log, a
// dashboard of a few partitions, every tenth step to a database).
//
// Set it as a simulation's OutputFunction. The coordinator still holds a single
// OutputCondition; it is not consulted for OutputViews, which decide per view.
type OutputViews struct {
	Views []OutputView
}

// Configure configures every view's sink.
func (o *OutputViews) Configure(settings *Settings) {
	for _, view := range o.Views {
		view.Function.Configure(settings)
	}
}

// Output sends to every view unconditionally. It is the path for a caller
// driving the sink by hand; a coordinator uses OutputStep, which applies each
// view's condition.
func (o *OutputViews) Output(partitionName string, state []float64, cumulativeTimesteps float64) {
	for _, view := range o.Views {
		view.Function.Output(partitionName, state, cumulativeTimesteps)
	}
}

// OutputStep sends one step's output to each view whose condition selects it.
func (o *OutputViews) OutputStep(
	partitionName string,
	state []float64,
	timestepsHistory *CumulativeTimestepsHistory,
	cumulativeTimesteps float64,
) {
	for _, view := range o.Views {
		if view.Condition.IsOutputStep(partitionName, state, timestepsHistory) {
			view.Function.Output(partitionName, state, cumulativeTimesteps)
		}
	}
}

// Finalize finalizes every view's sink that holds resources.
func (o *OutputViews) Finalize() {
	for _, view := range o.Views {
		if finalizing, ok := view.Function.(FinalizingOutputFunction); ok {
			finalizing.Finalize()
		}
	}
}

// Stage stages every view's sink that can be staged (see StagedOutputFunction).
func (o *OutputViews) Stage() {
	for _, view := range o.Views {
		if staged, ok := view.Function.(StagedOutputFunction); ok {
			staged.Stage()
		}
	}
}

// Commit commits every staged view's sink, returning every failure.
func (o *OutputViews) Commit() error {
	var failures []error
	for _, view := range o.Views {
		if staged, ok := view.Function.(StagedOutputFunction); ok {
			failures = append(failures, staged.Commit())
		}
	}
	return errors.Join(failures...)
}

// Abort aborts every staged view's sink.
func (o *OutputViews) Abort() {
	for _, view := range o.Views {
		if staged, ok := view.Function.(StagedOutputFunction); ok {
			staged.Abort()
		}
	}
}

// emitOutput sends one partition's state for a step to the simulation's output:
// OutputViews decides per view; anything else is gated by the
// simulation's single condition. StateIterator.Iterate repeats this logic
// inline on the per-step path; keep the two in step.
func emitOutput(
	condition OutputCondition,
	function OutputFunction,
	partitionName string,
	state []float64,
	timestepsHistory *CumulativeTimestepsHistory,
	cumulativeTimesteps float64,
) {
	// A concrete type check is one pointer comparison per output; an interface
	// check costs a lookup, which showed up in the step benchmarks.
	if views, ok := function.(*OutputViews); ok {
		views.OutputStep(partitionName, state, timestepsHistory, cumulativeTimesteps)
		return
	}
	if condition.IsOutputStep(partitionName, state, timestepsHistory) {
		function.Output(partitionName, state, cumulativeTimesteps)
	}
}
