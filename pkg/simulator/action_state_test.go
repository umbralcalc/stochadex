package simulator

import (
	"testing"

	"gonum.org/v1/gonum/floats"
	"google.golang.org/protobuf/proto"
)

func TestActionStateRoundTrip(t *testing.T) {
	// The wire format dexetera's clients send: field numbers must not change.
	sent := &ActionState{
		Values:     []float64{1.5, -2},
		Partitions: map[string]*ActionValues{"a": {Values: []float64{3}}},
	}
	data, err := proto.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}
	received := &ActionState{}
	if err := proto.Unmarshal(data, received); err != nil {
		t.Fatal(err)
	}
	if !floats.Equal(received.GetValues(), []float64{1.5, -2}) ||
		!floats.Equal(received.GetPartitions()["a"].GetValues(), []float64{3}) {
		t.Errorf("round trip gave %v", received)
	}
	// Field 1 (values) is packed doubles: tag 0x0a, then length 16.
	if data[0] != 0x0a || data[1] != 16 {
		t.Errorf("values should be field 1, packed: got leading bytes %x", data[:2])
	}
	if received.String() == "" || (&ActionValues{}).String() != "" {
		t.Error("String should describe a set message and be empty for an unset one")
	}
}
