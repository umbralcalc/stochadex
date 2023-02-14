package stochadex

type Iterator interface {
	Iterate(stateHistory *StateHistory) *State
}

type StateIterator struct {
	stateHistory *StateHistory
	iterator     Iterator
}

func (s *StateIterator) Iterate() {
	newState := s.iterator.Iterate(s.stateHistory)

}
