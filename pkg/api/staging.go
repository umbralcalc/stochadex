package api

import (
	"errors"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// All-or-nothing outputs (PLAN.md O.4). Every sink a config run writes through
// is staged, so what it writes is published only when the whole run has ended
// cleanly: a file is written under a ".partial" name and renamed into place,
// and an upload or database ingest waits until then. A run that fails, panics
// or is killed leaves nothing at its outputs' destinations, and any earlier
// run's output there intact. Sinks that stream as the run goes (stdout, a
// served connection, a websocket push, Postgres) cannot be held back; they are
// not staged, and say so in their docs.
//
// "Ended cleanly" means the run reached its termination condition, or, for a
// served connection, that the client left: that is how a served session ends.

// stagedOutputs are the staged sinks of one run. Its zero value is ready to
// use, and a run that keeps it as a local value allocates nothing for it: the
// first sinks are held in place (a slice into its own array would move it to
// the heap).
type stagedOutputs struct {
	first [2]simulator.StagedOutputFunction
	count int
	more  []simulator.StagedOutputFunction
	// feeds' stream recorders are staged as they open, so they are collected
	// when the run ends.
	feeds *paramFeeds
	done  bool
}

// stage stages output, if it can be staged. Call it before the run configures
// it.
func (s *stagedOutputs) stage(output simulator.OutputFunction) {
	if staged, ok := output.(simulator.StagedOutputFunction); ok {
		staged.Stage()
		s.add(staged)
	}
}

// add adds a sink that is already staged.
func (s *stagedOutputs) add(staged simulator.StagedOutputFunction) {
	if s.count < len(s.first) {
		s.first[s.count] = staged
		s.count++
		return
	}
	s.more = append(s.more, staged)
}

// stageNested stages the outputs of config's embedded runs, written once per
// outer step.
func (s *stagedOutputs) stageNested(config *ApiRunConfig) {
	for _, embedded := range config.Embedded {
		s.stage(embedded.Run.Simulation.OutputFunction)
	}
}

// size and at walk every staged sink, the feeds' recorders included.
func (s *stagedOutputs) size() int {
	size := s.count + len(s.more)
	if s.feeds != nil {
		size += len(s.feeds.recorders)
	}
	return size
}

func (s *stagedOutputs) at(index int) simulator.StagedOutputFunction {
	switch {
	case index < s.count:
		return s.first[index]
	case index < s.count+len(s.more):
		return s.more[index-s.count]
	}
	return s.feeds.recorders[index-s.count-len(s.more)]
}

// finish publishes every sink's output if the run ended cleanly (err is nil),
// and discards it otherwise, returning err, or why publishing failed.
func (s *stagedOutputs) finish(err error) error {
	s.done = true
	if err != nil {
		for index := range s.size() {
			s.at(index).Abort()
		}
		return err
	}
	var failed error
	for index := range s.size() {
		failed = errors.Join(failed, s.at(index).Commit())
	}
	if failed != nil {
		return withKind(ErrUnavailable, failed)
	}
	return nil
}

// discard discards every sink's output unless finish has run. Deferred, it
// cleans up after a run that panicked.
func (s *stagedOutputs) discard() {
	if !s.done {
		s.finish(errors.New("the run did not finish"))
	}
}
