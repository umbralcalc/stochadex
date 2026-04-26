package agentstest

import "fmt"

// TTTState is a tic-tac-toe position. cells holds 0=empty, 1=X, 2=O.
type TTTState struct {
	Cells   [9]int8
	Current int // 0=X to move, 1=O to move
	Done    bool
	Winner  int // -1=draw or in-progress, 0=X won, 1=O won
}

// TTTAction is a cell index 0..8.
type TTTAction int

// TTTKey is a stable string key for an action, useful for any aggregation
// or lookup that needs a string-typed action identifier.
func TTTKey(a TTTAction) string { return fmt.Sprintf("%d", a) }

// WinLines is the eight three-in-a-row patterns.
var WinLines = [...][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
	{0, 4, 8}, {2, 4, 6},
}

// TTTGame implements agents.Environment[TTTState, TTTAction].
//
// We do not import mcts here to avoid an import cycle from
// pkg/mcts/mctstest → pkg/mcts → ... → pkg/mcts/agentstest. Callers in
// _test.go files will satisfy agents.Environment by this struct's method
// set via duck typing at the call site.
type TTTGame struct{}

// Legal returns the empty cells; nil if the game is over.
func (g *TTTGame) Legal(s TTTState) []TTTAction {
	if s.Done {
		return nil
	}
	out := make([]TTTAction, 0, 9)
	for i, c := range s.Cells {
		if c == 0 {
			out = append(out, TTTAction(i))
		}
	}
	return out
}

// Apply places the current player's mark on cell a and updates Done/Winner.
func (g *TTTGame) Apply(s TTTState, a TTTAction) (TTTState, error) {
	if a < 0 || a >= 9 {
		return s, fmt.Errorf("ttt: bad action %d", a)
	}
	if s.Cells[a] != 0 {
		return s, fmt.Errorf("ttt: cell %d occupied", a)
	}
	out := s
	out.Cells[a] = int8(s.Current + 1)
	for _, line := range WinLines {
		v1, v2, v3 := out.Cells[line[0]], out.Cells[line[1]], out.Cells[line[2]]
		if v1 != 0 && v1 == v2 && v2 == v3 {
			out.Done = true
			out.Winner = int(v1) - 1
			return out, nil
		}
	}
	full := true
	for _, c := range out.Cells {
		if c == 0 {
			full = false
			break
		}
	}
	if full {
		out.Done = true
		out.Winner = -1
	} else {
		out.Current = 1 - out.Current
	}
	return out, nil
}

// Terminal returns the per-player [0,1] score vector and a done flag.
// Draw splits 0.5/0.5; a winner takes 1, the other 0.
func (g *TTTGame) Terminal(s TTTState) ([]float64, bool) {
	if !s.Done {
		return nil, false
	}
	scores := []float64{0, 0}
	switch s.Winner {
	case 0:
		scores[0] = 1
	case 1:
		scores[1] = 1
	default:
		scores[0] = 0.5
		scores[1] = 0.5
	}
	return scores, true
}

func (g *TTTGame) Actor(s TTTState) int   { return s.Current }
func (g *TTTGame) Players(s TTTState) int { return 2 }

// TTTWidth is the encoded row width: 9 cells + current player.
const TTTWidth = 10

// TTTEncode produces the []float64 row representation of a TTTState.
// Done/Winner are derivable from the cells so they are not encoded.
func TTTEncode(s TTTState) []float64 {
	out := make([]float64, TTTWidth)
	for i, c := range s.Cells {
		out[i] = float64(c)
	}
	out[9] = float64(s.Current)
	return out
}

// TTTDecode rebuilds a TTTState from its row representation, recomputing
// Done/Winner so callers can spell positions declaratively.
func TTTDecode(v []float64) (TTTState, error) {
	if len(v) != TTTWidth {
		return TTTState{}, fmt.Errorf("ttt: bad encoded width %d", len(v))
	}
	var s TTTState
	for i := 0; i < 9; i++ {
		s.Cells[i] = int8(v[i])
	}
	s.Current = int(v[9])
	s.Winner = -1
	for _, line := range WinLines {
		v1, v2, v3 := s.Cells[line[0]], s.Cells[line[1]], s.Cells[line[2]]
		if v1 != 0 && v1 == v2 && v2 == v3 {
			s.Done = true
			s.Winner = int(v1) - 1
			return s, nil
		}
	}
	full := true
	for _, c := range s.Cells {
		if c == 0 {
			full = false
			break
		}
	}
	if full {
		s.Done = true
	}
	return s, nil
}

// TTTFromGrid builds a TTTState from a literal cell grid plus the player
// to move. Done/Winner are derived. Useful for spelling test positions
// inline.
func TTTFromGrid(grid [9]int8, currentPlayer int) TTTState {
	s := TTTState{Cells: grid, Current: currentPlayer, Winner: -1}
	for _, line := range WinLines {
		v1, v2, v3 := s.Cells[line[0]], s.Cells[line[1]], s.Cells[line[2]]
		if v1 != 0 && v1 == v2 && v2 == v3 {
			s.Done = true
			s.Winner = int(v1) - 1
			return s
		}
	}
	return s
}
