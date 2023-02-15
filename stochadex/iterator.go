package stochadex

type Iteration interface {
	Iterate(stateHistory *StateHistory) *State
}

type StateIterator struct {
	StateHistory *StateHistory
	iteration    Iteration
}

func (s *StateIterator) Iterate() *State {
	return s.iteration.Iterate(s.StateHistory)
}

func (s *StateIterator) Broadcast(state *State) {

}
