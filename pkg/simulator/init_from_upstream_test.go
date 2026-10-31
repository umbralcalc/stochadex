package simulator

import (
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// runningSumIteration adds its "v" param, its upstream's value this step, to
// its own state: a running sum that never reads the upstream's initial row.
type runningSumIteration struct{}

func (r *runningSumIteration) Configure(int, *Settings) {}

func (r *runningSumIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	sum := stateHistories[partitionIndex].CopyStateRow(0)
	floats.Add(sum, params.Get("v"))
	return sum
}

// sumGenerator is a seeded walk "data" summed by "sum", which reads indices of
// it and starts from its initial state when fromUpstream.
func sumGenerator(indices []int, width int, fromUpstream bool) *ConfigGenerator {
	generator := ensembleBuilder(1, 10)()
	data := generator.GetPartition("walk_0")
	data.InitStateValues = []float64{3.5, -2.0}
	sum := &PartitionConfig{
		Name:               "sum",
		Iteration:          &runningSumIteration{},
		Params:             NewParams(map[string][]float64{"v": make([]float64, width)}),
		ParamsFromUpstream: map[string]NamedUpstreamConfig{"v": {Upstream: "walk_0", Indices: indices}},
		InitStateValues:    make([]float64, width),
		StateHistoryDepth:  1,
	}
	if fromUpstream {
		sum.InitStateFromUpstream = "v"
	}
	generator.SetPartition(sum)
	return generator
}

// recordedRun runs a generator to a storage.
func recordedRun(generator *ConfigGenerator) *StateTimeStorage {
	store := NewStateTimeStorage()
	simulation := *generator.GetSimulation()
	simulation.OutputFunction = &StateTimeStorageOutputFunction{Store: store}
	generator.SetSimulation(&simulation)
	settings, implementations := generator.GenerateConfigs()
	NewPartitionCoordinator(settings, implementations).Run()
	return store
}

// generateError is the panic GenerateConfigs raises, as text, or "".
func generateError(generator *ConfigGenerator) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message = recovered.(string)
		}
	}()
	generator.GenerateConfigs()
	return ""
}

func TestInitStateFromUpstream(t *testing.T) {
	t.Run("a running sum started from its upstream counts every row, the initial one included", func(t *testing.T) {
		store := recordedRun(sumGenerator(nil, 2, true))
		data, sums := store.GetValues("walk_0"), store.GetValues("sum")
		want := make([]float64, 2)
		for row := range data {
			floats.Add(want, data[row])
			if !floats.EqualApprox(sums[row], want, 1e-12) {
				t.Fatalf("row %d: sum %v, want the sum of rows 0..%d, %v", row, sums[row], row, want)
			}
		}
	})

	t.Run("without it the running sum misses the upstream's initial row", func(t *testing.T) {
		store := recordedRun(sumGenerator(nil, 2, false))
		data, sums := store.GetValues("walk_0"), store.GetValues("sum")
		// Every row but the first.
		want := make([]float64, 2)
		for _, row := range data[1:] {
			floats.Add(want, row)
		}
		if got := sums[len(sums)-1]; !floats.EqualApprox(got, want, 1e-12) {
			t.Fatalf("final sum %v, want %v, the rows after the initial one", got, want)
		}
	})

	t.Run("it takes the upstream key's indices, in their order", func(t *testing.T) {
		settings, _ := sumGenerator([]int{1, 0, 1}, 3, true).GenerateConfigs()
		got := settings.Iterations[1].InitStateValues
		if want := []float64{-2.0, 3.5, -2.0}; !floats.Equal(got, want) {
			t.Fatalf("initial state %v, want %v", got, want)
		}
	})

	t.Run("it resolves through a chain of partitions that start from their upstreams", func(t *testing.T) {
		generator := sumGenerator(nil, 2, true)
		generator.SetPartition(&PartitionConfig{
			Name:                  "second",
			Iteration:             &runningSumIteration{},
			Params:                NewParams(map[string][]float64{"v": {0}}),
			ParamsFromUpstream:    map[string]NamedUpstreamConfig{"v": {Upstream: "sum", Indices: []int{1}}},
			InitStateValues:       []float64{0},
			InitStateFromUpstream: "v",
			StateHistoryDepth:     1,
		})
		settings, _ := generator.GenerateConfigs()
		if got := settings.Iterations[2].InitStateValues; !floats.Equal(got, []float64{-2.0}) {
			t.Fatalf("the chain's end starts from %v, want [-2], its upstream's own upstream's value", got)
		}
	})

	t.Run("the configs it generates leave the partition config as it was", func(t *testing.T) {
		generator := sumGenerator(nil, 2, true)
		generator.GenerateConfigs()
		if got := generator.GetPartition("sum").InitStateValues; !floats.Equal(got, []float64{0, 0}) {
			t.Fatalf("the partition config's init_state_values became %v", got)
		}
	})

	t.Run("it passes the harnesses", func(t *testing.T) {
		settings, implementations := sumGenerator(nil, 2, true).GenerateConfigs()
		if err := RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	for _, c := range []struct {
		name  string
		setup func(*ConfigGenerator)
		want  string
	}{
		{"a key params_from_upstream does not have", func(g *ConfigGenerator) {
			g.GetPartition("sum").InitStateFromUpstream = "nope"
		}, `init_state_from_upstream "nope" names no params_from_upstream key`},
		{"a width other than the partition's", func(g *ConfigGenerator) {
			g.GetPartition("sum").InitStateValues = []float64{0}
		}, `gives 2 values, but its init_state_values give it a width of 1`},
		{"an index past the upstream's width", func(g *ConfigGenerator) {
			g.GetPartition("sum").ParamsFromUpstream["v"] = NamedUpstreamConfig{Upstream: "walk_0", Indices: []int{0, 5}}
		}, `index 5 out of range (state width 2)`},
		{"an upstream that does not exist", func(g *ConfigGenerator) {
			g.GetPartition("sum").ParamsFromUpstream["v"] = NamedUpstreamConfig{Upstream: "nope"}
		}, `no partition by that name`},
		{"partitions that start from each other", func(g *ConfigGenerator) {
			data := g.GetPartition("walk_0")
			data.ParamsFromUpstream = map[string]NamedUpstreamConfig{"v": {Upstream: "sum"}}
			data.InitStateFromUpstream = "v"
		}, `reads its own initial state back through its upstreams`},
	} {
		t.Run("it refuses "+c.name, func(t *testing.T) {
			generator := sumGenerator(nil, 2, true)
			c.setup(generator)
			if got := generateError(generator); !strings.Contains(got, c.want) {
				t.Fatalf("GenerateConfigs panicked with %q, want it to contain %q", got, c.want)
			}
		})
	}
}
