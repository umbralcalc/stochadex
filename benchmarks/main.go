// Command benchmarks measures stochadex's fair, CPU-to-CPU performance claims and
// writes the results as JSON (consumed by plot.py to render the committed plots).
//
// Deliberately NOT a peak-FLOPs race against GPU frameworks (JAX/Julia) — those win
// on their own hardware and problem shapes, and comparing them on a laptop CPU would
// be apples-to-oranges. These are the systems claims that are actually stochadex's:
//
//   - ensemble scaling, cold start
//   - whole-process, coupled, and branching-coupled simulation vs NumPy
//   - execution strategies across regimes (where each shines)
//
// The BLAS vector-op micro-benchmark (README §5) lives in ./benchmarks/vectorops — the
// only benchmark that touches BLAS, so the only one the `cblas` backend changes.
//
// Run: `go run ./benchmarks` (from the repo root) or `go run .` from here. Writes
// benchmarks/results/*.json. Numbers are machine-specific — record the machine in the
// README when committing results.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gonum.org/v1/gonum/stat/distuv"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// busyIteration does a controllable amount of allocation-free compute per element per
// step ("ops" transcendental iterations) — the knob that decides whether within-step
// parallelism (spawn/persistent) can beat serial inline. Edge-free, so a coordinator
// can run its partitions concurrently within a step.
type busyIteration struct{}

func (b *busyIteration) Configure(partitionIndex int, settings *simulator.Settings) {}

func (b *busyIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	values := stateHistories[partitionIndex].GetNextStateRowToUpdate()
	ops := int(params.GetIndex("ops", 0))
	for i := range values {
		x := values[i]
		for k := 0; k < ops; k++ {
			x = math.Sin(x) + 1.0 // compute-heavy, allocation-free
		}
		values[i] = x
	}
	return values
}

// branchResponder is a bespoke coupled iteration: it decays each step, and ONLY when
// its upstream driver crosses a threshold (a rare, per-path condition) does it do
// expensive work — a sum of `terms` gamma draws. This is the coupling that is hard to
// vectorize: SIMD over paths must either compute the expensive branch for every path
// and mask, or gather the (few) triggered paths; a scalar per-path loop just takes the
// branch. Reads the driver's current-step values via ParamsFromUpstream ("driver").
type branchResponder struct {
	gamma *distuv.Gamma
}

func (b *branchResponder) Configure(partitionIndex int, settings *simulator.Settings) {
	seed := settings.Iterations[partitionIndex].Seed
	b.gamma = &distuv.Gamma{Alpha: 2.0, Beta: 1.0, Src: rand.NewPCG(seed, seed)}
}

func (b *branchResponder) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	values := stateHistories[partitionIndex].GetNextStateRowToUpdate()
	driver := params.Map["driver"]
	threshold := params.GetIndex("threshold", 0)
	decay := params.GetIndex("decay", 0)
	terms := int(params.GetIndex("terms", 0))
	for i := range values {
		values[i] *= decay
		if driver[i] > threshold { // rare per-path branch -> expensive work only here
			sum := 0.0
			for k := 0; k < terms; k++ {
				sum += b.gamma.Rand()
			}
			values[i] += sum
		}
	}
	return values
}

// stateWidth is the per-partition state dimension used for the scaling benchmark —
// a modest vector so each partition does a little real work per step.
const stateWidth = 8

// buildGen returns a fresh ConfigGenerator for a simulation of numPartitions
// Wiener processes run for numSteps under the given execution strategy (nil = the
// default). A fresh generator per call is required by RunSeededEnsemble (each member
// needs its own Iteration instances). Output is discarded for the within-sim path.
func buildGen(numPartitions, numSteps int, strategy simulator.ExecutionStrategy) *simulator.ConfigGenerator {
	gen := simulator.NewConfigGenerator()
	variances := make([]float64, stateWidth)
	init := make([]float64, stateWidth)
	for i := range variances {
		variances[i] = 1.0
	}
	for i := 0; i < numPartitions; i++ {
		gen.SetPartition(&simulator.PartitionConfig{
			Name:              fmt.Sprintf("p%d", i),
			Iteration:         &continuous.WienerProcessIteration{},
			Params:            simulator.NewParams(map[string][]float64{"variances": variances}),
			InitStateValues:   append([]float64(nil), init...),
			StateHistoryDepth: 1,
			Seed:              uint64(i + 1),
		})
	}
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: numSteps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		ExecutionStrategy:    strategy,
	})
	return gen
}

// buildSim is buildGen + GenerateConfigs, for the direct (non-ensemble) runs.
func buildSim(numPartitions, numSteps int, strategy simulator.ExecutionStrategy) (*simulator.Settings, *simulator.Implementations) {
	return buildGen(numPartitions, numSteps, strategy).GenerateConfigs()
}

// runSim runs a prebuilt simulation to completion (the timed hot loop).
func runSim(settings *simulator.Settings, implementations *simulator.Implementations) {
	// Run() applies the configured ExecutionStrategy; a raw Step() loop would bypass it.
	simulator.NewPartitionCoordinator(settings, implementations).Run()
}

type scalingPoint struct {
	MaxConcurrency int     `json:"max_concurrency"`
	Members        int     `json:"members"`
	BestSeconds    float64 `json:"best_seconds"`
	MembersPerSec  float64 `json:"members_per_sec"`
	SpeedupVs1     float64 `json:"speedup_vs_1"`
}

// benchmarkEnsembleScaling measures the engine's real parallelism claim: running an
// ensemble of INDEPENDENT simulations (RunSeededEnsemble) is embarrassingly parallel —
// there is no per-step barrier between members — so throughput should scale ~linearly
// with maxConcurrency up to the core count. (This is the right place to measure
// concurrency; partitions WITHIN one simulation are step-synchronised for coupled
// components and are not the parallelism story.) Each member is one small Wiener
// simulation run to termination.
func benchmarkEnsembleScaling() []scalingPoint {
	const members, memberSteps, repeats = 512, 3000, 5
	// Each member is one edge-free partition, so it runs inline (no per-step
	// goroutine spawn); the parallelism is at the ensemble level, across members.
	build := func() *simulator.ConfigGenerator { return buildGen(1, memberSteps, &simulator.InlineExecution{}) }
	seeds := make([]uint64, members)
	for i := range seeds {
		seeds[i] = uint64(i + 1)
	}

	concs := []int{1, 2, 3, 4, 6, 8, runtime.NumCPU()}
	// de-dup / sort not needed; NumCPU may equal a listed value — harmless.
	var out []scalingPoint
	var base float64
	for _, c := range concs {
		simulator.RunSeededEnsemble(build, seeds[:32], c) // warm
		best := time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			start := time.Now()
			_ = simulator.RunSeededEnsemble(build, seeds, c)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		sec := best.Seconds()
		if c == 1 {
			base = sec
		}
		out = append(out, scalingPoint{
			MaxConcurrency: c,
			Members:        members,
			BestSeconds:    sec,
			MembersPerSec:  float64(members) / sec,
			SpeedupVs1:     base / sec,
		})
		fmt.Printf("  maxConcurrency=%-3d  %.3fs  %.0f members/s  %.2fx\n",
			c, sec, float64(members)/sec, base/sec)
	}
	return out
}

type coldStart struct {
	Repeats            int     `json:"repeats"`
	MedianMicroseconds float64 `json:"median_microseconds"`
	Note               string  `json:"note"`
}

// benchmarkColdStart measures the time from an unbuilt simulation to the first
// produced result: config assembly + one step. A statically-compiled Go binary has
// no interpreter or JIT to warm up, so this is ~immediate and stable run-to-run —
// the warmup-free, single-binary deployment property, stated as an absolute.
func benchmarkColdStart() coldStart {
	const repeats = 201
	samples := make([]float64, repeats)
	for r := 0; r < repeats; r++ {
		start := time.Now()
		settings, impl := buildSim(1, 1, nil)
		runSim(settings, impl)
		samples[r] = float64(time.Since(start).Microseconds())
	}
	// median
	for i := 0; i < len(samples); i++ {
		for j := i + 1; j < len(samples); j++ {
			if samples[j] < samples[i] {
				samples[i], samples[j] = samples[j], samples[i]
			}
		}
	}
	med := samples[len(samples)/2]
	fmt.Printf("  cold-start (config + first step): median %.1f µs\n", med)
	return coldStart{
		Repeats:            repeats,
		MedianMicroseconds: med,
		Note:               "config assembly + first step; statically compiled, no JIT/interpreter warmup",
	}
}

func fill(n int, v float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = v
	}
	return s
}

// newProcessIteration returns a fresh iteration + params + init state for one
// stochastic process, with `width` independent paths in the state vector.
func newProcessIteration(kind string, width int) (simulator.Iteration, map[string][]float64, []float64) {
	params := map[string][]float64{}
	init := make([]float64, width)
	var iter simulator.Iteration
	switch kind {
	case "gbm":
		iter = &continuous.GeometricBrownianMotionIteration{}
		params["variances"] = fill(width, 0.04)
		for i := range init {
			init[i] = 1.0
		}
	case "ou":
		iter = &continuous.OrnsteinUhlenbeckIteration{}
		params["thetas"], params["mus"], params["sigmas"] = fill(width, 0.5), fill(width, 0.0), fill(width, 0.3)
	case "compound_poisson":
		iter = &continuous.CompoundPoissonProcessIteration{JumpDist: &continuous.GammaJumpDistribution{}}
		params["rates"], params["gamma_alphas"], params["gamma_betas"] = fill(width, 5.0), fill(width, 2.0), fill(width, 1.0)
	default:
		panic("unknown process " + kind)
	}
	return iter, params, init
}

// processGen builds a simulation of `numPartitions` independent process partitions,
// each carrying `width` paths, run for `steps` under the given execution strategy.
// Output is discarded (NilOutputFunction) so we time the simulation compute, not
// history storage — the same thing the NumPy comparison does. Varying
// (numPartitions, width, strategy) and whether it is run as one sim or an ensemble
// is exactly the execution-model freedom the benchmark surfaces.
func processGen(kind string, numPartitions, width, steps int, strategy simulator.ExecutionStrategy) func() *simulator.ConfigGenerator {
	return func() *simulator.ConfigGenerator {
		gen := simulator.NewConfigGenerator()
		for p := 0; p < numPartitions; p++ {
			iter, params, init := newProcessIteration(kind, width)
			gen.SetPartition(&simulator.PartitionConfig{
				Name:              fmt.Sprintf("proc%d", p),
				Iteration:         iter,
				Params:            simulator.NewParams(params),
				InitStateValues:   init,
				StateHistoryDepth: 1,
				Seed:              uint64(p + 1),
			})
		}
		gen.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 0.01},
			ExecutionStrategy:    strategy,
		})
		return gen
	}
}

// runProcessEnsemble runs `members` independent simulations (built by gen) to
// termination, up to maxConc at once, with output discarded, and returns the wall
// time. This is a fair parallel ensemble runner (no per-step storage overhead).
func runProcessEnsemble(gen func() *simulator.ConfigGenerator, members, maxConc int) time.Duration {
	sem := make(chan struct{}, maxConc)
	var wg sync.WaitGroup
	start := time.Now()
	for m := 0; m < members; m++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(seed uint64) {
			defer wg.Done()
			defer func() { <-sem }()
			g := gen()
			g.SetGlobalSeed(seed)
			settings, impl := g.GenerateConfigs()
			simulator.NewPartitionCoordinator(settings, impl).Run()
		}(uint64(m + 1))
	}
	wg.Wait()
	return time.Since(start)
}

type processResult struct {
	Process    string             `json:"process"`
	TotalPaths int                `json:"total_paths"`
	Steps      int                `json:"steps"`
	Seconds    map[string]float64 `json:"seconds"` // execution model -> best wall seconds
}

// benchmarkProcesses times whole-process simulation across every stochadex execution
// model, so it is explicit which one wins, why, and that the user is free to tune it
// (NumPy, added by numpy_processes.py, offers one way). All configs simulate the same
// totalPaths × steps of the same process; the engine (coordinator + iteration +
// ensemble) is fully in the loop, not just gonum primitives.
//
// The models, in order:
//   - single wide inline partition, 1 core        — the optimal serial config
//   - one sim, N partitions, spawn-per-step        — within-sim (default); the per-step
//   - one sim, N partitions, persistent-worker         barrier limits decoupled work,
//   - one sim, N partitions, inline (serial)           so these barely beat serial
//   - ensemble of N inline members, all cores      — decoupled, no barrier: the winner
func benchmarkProcesses() []processResult {
	const totalPaths, steps, repeats = 10000, 2000, 3
	nc := runtime.NumCPU()
	width := totalPaths / nc
	var out []processResult
	for _, kind := range []string{"gbm", "ou", "compound_poisson"} {
		configs := []struct {
			name             string
			gen              func() *simulator.ConfigGenerator
			members, maxConc int
		}{
			{"single wide inline partition (1 core)",
				processGen(kind, 1, totalPaths, steps, &simulator.InlineExecution{}), 1, 1},
			{"one sim, N partitions, spawn-per-step",
				processGen(kind, nc, width, steps, &simulator.SpawnPerStepExecution{}), 1, 1},
			{"one sim, N partitions, persistent-worker",
				processGen(kind, nc, width, steps, &simulator.PersistentWorkerExecution{}), 1, 1},
			{"one sim, N partitions, inline (serial)",
				processGen(kind, nc, width, steps, &simulator.InlineExecution{}), 1, 1},
			{"ensemble, N inline members (all cores)",
				processGen(kind, 1, width, steps, &simulator.InlineExecution{}), nc, nc},
		}
		seconds := map[string]float64{}
		for _, c := range configs {
			runProcessEnsemble(c.gen, c.members, c.maxConc) // warm
			best := time.Duration(1 << 62)
			for r := 0; r < repeats; r++ {
				if d := runProcessEnsemble(c.gen, c.members, c.maxConc); d < best {
					best = d
				}
			}
			seconds[c.name] = best.Seconds()
			fmt.Printf("  %-16s  %-42s %.3fs\n", kind, c.name, best.Seconds())
		}
		out = append(out, processResult{Process: kind, TotalPaths: totalPaths, Steps: steps, Seconds: seconds})
	}
	return out
}

// coupledGen builds `numChains` independent coupled chains of OU components. Within a
// chain, component j mean-reverts toward component j-1's CURRENT-step value (a
// within-step `ParamsFromUpstream` edge on "mus") — the coordinator resolves the
// ordering. This is the regime the engine is designed for: NumPy must hand-order the
// same cross-dependencies by writing them in the right sequence each step.
func coupledGen(numChains, width, steps int, strategy simulator.ExecutionStrategy) func() *simulator.ConfigGenerator {
	const chainLen = 4
	return func() *simulator.ConfigGenerator {
		gen := simulator.NewConfigGenerator()
		for c := 0; c < numChains; c++ {
			for j := 0; j < chainLen; j++ {
				pc := &simulator.PartitionConfig{
					Name:      fmt.Sprintf("c%d_%d", c, j),
					Iteration: &continuous.OrnsteinUhlenbeckIteration{},
					Params: simulator.NewParams(map[string][]float64{
						"thetas": fill(width, 1.0),
						"mus":    fill(width, 0.0),
						"sigmas": fill(width, 0.3),
					}),
					InitStateValues:   make([]float64, width),
					StateHistoryDepth: 1,
					Seed:              uint64(c*chainLen + j + 1),
				}
				if j > 0 { // component j tracks component j-1's current-step value
					pc.ParamsFromUpstream = map[string]simulator.NamedUpstreamConfig{
						"mus": {Upstream: fmt.Sprintf("c%d_%d", c, j-1)},
					}
				}
				gen.SetPartition(pc)
			}
		}
		gen.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 0.01},
			ExecutionStrategy:    strategy,
		})
		return gen
	}
}

// benchmarkCoupled runs the coupled OU-chain system (chainLen=4 within-step-coupled
// components) through the same execution-model matrix as benchmarkProcesses. Coupled
// systems are the engine's home turf — each "unit" here is a chain, not a lone process.
func benchmarkCoupled() []processResult {
	const totalPaths, steps, repeats = 10000, 2000, 3
	nc := runtime.NumCPU()
	width := totalPaths / nc
	configs := []struct {
		name             string
		gen              func() *simulator.ConfigGenerator
		members, maxConc int
	}{
		{"single wide inline chain (1 core)",
			coupledGen(1, totalPaths, steps, &simulator.InlineExecution{}), 1, 1},
		{"one sim, N chains, spawn-per-step",
			coupledGen(nc, width, steps, &simulator.SpawnPerStepExecution{}), 1, 1},
		{"one sim, N chains, persistent-worker",
			coupledGen(nc, width, steps, &simulator.PersistentWorkerExecution{}), 1, 1},
		{"one sim, N chains, inline (serial)",
			coupledGen(nc, width, steps, &simulator.InlineExecution{}), 1, 1},
		{"ensemble, N inline chains (all cores)",
			coupledGen(1, width, steps, &simulator.InlineExecution{}), nc, nc},
	}
	seconds := map[string]float64{}
	for _, c := range configs {
		runProcessEnsemble(c.gen, c.members, c.maxConc) // warm
		best := time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			if d := runProcessEnsemble(c.gen, c.members, c.maxConc); d < best {
				best = d
			}
		}
		seconds[c.name] = best.Seconds()
		fmt.Printf("  %-42s %.3fs\n", c.name, best.Seconds())
	}
	return []processResult{{Process: "coupled_ou_chain_len4", TotalPaths: totalPaths, Steps: steps, Seconds: seconds}}
}

// branchCoupledGen builds `numSystems` independent 2-partition systems: an OU driver
// and a branchResponder that reads the driver's current value and does expensive work
// only when it crosses a threshold. The threshold/σ are set so ~7% of path-steps
// trigger — the rare-branch regime where SIMD-over-paths wastes work.
func branchCoupledGen(numSystems, width, steps int, strategy simulator.ExecutionStrategy) func() *simulator.ConfigGenerator {
	return func() *simulator.ConfigGenerator {
		gen := simulator.NewConfigGenerator()
		for s := 0; s < numSystems; s++ {
			aName, bName := fmt.Sprintf("a%d", s), fmt.Sprintf("b%d", s)
			gen.SetPartition(&simulator.PartitionConfig{
				Name:      aName,
				Iteration: &continuous.OrnsteinUhlenbeckIteration{},
				Params: simulator.NewParams(map[string][]float64{
					"thetas": fill(width, 0.5), "mus": fill(width, 0.0), "sigmas": fill(width, 1.0),
				}),
				InitStateValues: make([]float64, width), StateHistoryDepth: 1, Seed: uint64(2*s + 1),
			})
			gen.SetPartition(&simulator.PartitionConfig{
				Name:      bName,
				Iteration: &branchResponder{},
				Params: simulator.NewParams(map[string][]float64{
					"threshold": {1.5}, "decay": {0.99}, "terms": {30},
				}),
				ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{"driver": {Upstream: aName}},
				InitStateValues:    make([]float64, width), StateHistoryDepth: 1, Seed: uint64(2*s + 2),
			})
		}
		gen.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 0.01},
			ExecutionStrategy:    strategy,
		})
		return gen
	}
}

// benchmarkBranchCoupled runs the threshold-triggered branching-coupled system through
// the execution-model matrix — the case where the coupling is hard to vectorize.
func benchmarkBranchCoupled() []processResult {
	const totalPaths, steps, repeats = 10000, 2000, 3
	nc := runtime.NumCPU()
	width := totalPaths / nc
	configs := []struct {
		name             string
		gen              func() *simulator.ConfigGenerator
		members, maxConc int
	}{
		{"single wide inline (1 core)",
			branchCoupledGen(1, totalPaths, steps, &simulator.InlineExecution{}), 1, 1},
		{"one sim, N systems, spawn-per-step",
			branchCoupledGen(nc, width, steps, &simulator.SpawnPerStepExecution{}), 1, 1},
		{"one sim, N systems, inline (serial)",
			branchCoupledGen(nc, width, steps, &simulator.InlineExecution{}), 1, 1},
		{"ensemble, N inline systems (all cores)",
			branchCoupledGen(1, width, steps, &simulator.InlineExecution{}), nc, nc},
	}
	seconds := map[string]float64{}
	for _, c := range configs {
		runProcessEnsemble(c.gen, c.members, c.maxConc) // warm
		best := time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			if d := runProcessEnsemble(c.gen, c.members, c.maxConc); d < best {
				best = d
			}
		}
		seconds[c.name] = best.Seconds()
		fmt.Printf("  %-42s %.3fs\n", c.name, best.Seconds())
	}
	return []processResult{{Process: "branch_coupled", TotalPaths: totalPaths, Steps: steps, Seconds: seconds}}
}

// tunedOUIteration is a hand-optimized Ornstein–Uhlenbeck step: the SAME math as
// continuous.OrnsteinUhlenbeckIteration, rewritten to shed the two per-element costs that
// make the stock iteration slow on a single core (see README §3 discussion):
//
//   - it hoists the "thetas"/"mus"/"sigmas" param SLICES out of the loop, replacing three
//     string-keyed map lookups per path per step (params.GetIndex) with plain slice reads;
//   - it owns ONE math/rand/v2 generator (created once in Configure) and draws from it
//     directly, instead of distuv.Normal.Rand() allocating a fresh rand wrapper every draw.
//
// It stays pure-Go and WASM-clean — no cgo, no assembly. This is the "tuned iteration"
// companion to the stock one: it shows how much of the single-core gap vs NumPy is a
// straightforward optimization of the Iteration body, not a limit of the engine.
type tunedOUIteration struct {
	rng *rand.Rand
}

func (o *tunedOUIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	seed := settings.Iterations[partitionIndex].Seed
	o.rng = rand.New(rand.NewPCG(seed, seed))
}

func (o *tunedOUIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	values := stateHistories[partitionIndex].GetNextStateRowToUpdate()
	thetas, mus, sigmas := params.Map["thetas"], params.Map["mus"], params.Map["sigmas"]
	dt := timestepsHistory.NextIncrement
	sqrtDt := math.Sqrt(dt)
	for i := range values {
		values[i] += thetas[i]*(mus[i]-values[i])*dt + sigmas[i]*sqrtDt*o.rng.NormFloat64()
	}
	return values
}

// ouGen builds a single wide inline OU partition (1 core) using the iteration built by
// makeIter — the shared config for the stock-vs-tuned single-core comparison. Params match
// newProcessIteration("ou", ...) exactly so it is the identical workload.
func ouGen(makeIter func() simulator.Iteration, width, steps int) func() *simulator.ConfigGenerator {
	return func() *simulator.ConfigGenerator {
		gen := simulator.NewConfigGenerator()
		gen.SetPartition(&simulator.PartitionConfig{
			Name:      "ou",
			Iteration: makeIter(),
			Params: simulator.NewParams(map[string][]float64{
				"thetas": fill(width, 0.5), "mus": fill(width, 0.0), "sigmas": fill(width, 0.3),
			}),
			InitStateValues:   make([]float64, width),
			StateHistoryDepth: 1,
			Seed:              1,
		})
		gen.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 0.01},
			ExecutionStrategy:    &simulator.InlineExecution{},
		})
		return gen
	}
}

// benchmarkTunedOU compares, on ONE core (single wide inline partition), the stock
// continuous.OrnsteinUhlenbeckIteration against tunedOUIteration — the same OU workload as
// benchmarkProcesses' "ou" case, so its single-core NumPy number (from numpy_processes.py)
// is the reference. It quantifies how much of the single-core gap is recoverable in pure Go.
func benchmarkTunedOU() []processResult {
	const totalPaths, steps, repeats = 10000, 2000, 5
	configs := []struct {
		name string
		gen  func() *simulator.ConfigGenerator
	}{
		{"stock OU iteration (1 core inline)",
			ouGen(func() simulator.Iteration { return &continuous.OrnsteinUhlenbeckIteration{} }, totalPaths, steps)},
		{"tuned OU iteration (1 core inline)",
			ouGen(func() simulator.Iteration { return &tunedOUIteration{} }, totalPaths, steps)},
	}
	seconds := map[string]float64{}
	for _, c := range configs {
		runProcessEnsemble(c.gen, 1, 1) // warm
		best := time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			if d := runProcessEnsemble(c.gen, 1, 1); d < best {
				best = d
			}
		}
		seconds[c.name] = best.Seconds()
		fmt.Printf("  %-40s %.3fs\n", c.name, best.Seconds())
	}
	return []processResult{{Process: "tuned_ou", TotalPaths: totalPaths, Steps: steps, Seconds: seconds}}
}

// tunedBranchResponder is the hand-optimized counterpart to branchResponder — identical
// behaviour (decay each step; on a threshold crossing add a sum of `terms` Gamma(2,1)
// draws), but it owns ONE math/rand/v2 generator and samples gamma inline via
// Marsaglia–Tsang (the exact algorithm distuv.Gamma uses for alpha≥1/3), instead of
// distuv.Gamma.Rand() which allocates a fresh rand wrapper on every one of the 30 draws
// per triggered path. The Marsaglia–Tsang constants (d, c) are precomputed in Configure.
// Pure-Go/WASM-clean. This is the branching-coupled analogue of tunedOUIteration.
type tunedBranchResponder struct {
	rng    *rand.Rand
	gammaD float64 // Marsaglia–Tsang d = alpha - 1/3
	gammaC float64 // 1 / (3·sqrt(d))
	gammaB float64 // rate (Beta)
}

func (b *tunedBranchResponder) Configure(partitionIndex int, settings *simulator.Settings) {
	seed := settings.Iterations[partitionIndex].Seed
	b.rng = rand.New(rand.NewPCG(seed, seed))
	const alpha, beta = 2.0, 1.0 // matches branchResponder's distuv.Gamma{Alpha:2, Beta:1}
	b.gammaD = alpha - 1.0/3
	b.gammaC = 1 / (3 * math.Sqrt(b.gammaD))
	b.gammaB = beta
}

// gammaRand draws Gamma(alpha, rate=Beta) via Marsaglia–Tsang from the owned generator —
// the same math as distuv.Gamma for alpha≥1, minus the per-call rand.New(Src) allocation.
func (b *tunedBranchResponder) gammaRand() float64 {
	for {
		x := b.rng.NormFloat64()
		v := 1 + x*b.gammaC
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := b.rng.Float64()
		if u < 1.0-0.0331*(x*x)*(x*x) {
			return b.gammaD * v / b.gammaB
		}
		if math.Log(u) < 0.5*x*x+b.gammaD*(1-v+math.Log(v)) {
			return b.gammaD * v / b.gammaB
		}
	}
}

func (b *tunedBranchResponder) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	values := stateHistories[partitionIndex].GetNextStateRowToUpdate()
	driver := params.Map["driver"]
	threshold := params.GetIndex("threshold", 0)
	decay := params.GetIndex("decay", 0)
	terms := int(params.GetIndex("terms", 0))
	for i := range values {
		values[i] *= decay
		if driver[i] > threshold {
			sum := 0.0
			for k := 0; k < terms; k++ {
				sum += b.gammaRand()
			}
			values[i] += sum
		}
	}
	return values
}

// branchCoupledGenTuned mirrors branchCoupledGen but wires the tuned iterations — a tuned
// OU driver and a tunedBranchResponder — so the branching-coupled system can be timed with
// both hot loops hand-optimized (still pure-Go).
func branchCoupledGenTuned(numSystems, width, steps int, strategy simulator.ExecutionStrategy) func() *simulator.ConfigGenerator {
	return func() *simulator.ConfigGenerator {
		gen := simulator.NewConfigGenerator()
		for s := 0; s < numSystems; s++ {
			aName, bName := fmt.Sprintf("a%d", s), fmt.Sprintf("b%d", s)
			gen.SetPartition(&simulator.PartitionConfig{
				Name:      aName,
				Iteration: &tunedOUIteration{},
				Params: simulator.NewParams(map[string][]float64{
					"thetas": fill(width, 0.5), "mus": fill(width, 0.0), "sigmas": fill(width, 1.0),
				}),
				InitStateValues: make([]float64, width), StateHistoryDepth: 1, Seed: uint64(2*s + 1),
			})
			gen.SetPartition(&simulator.PartitionConfig{
				Name:      bName,
				Iteration: &tunedBranchResponder{},
				Params: simulator.NewParams(map[string][]float64{
					"threshold": {1.5}, "decay": {0.99}, "terms": {30},
				}),
				ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{"driver": {Upstream: aName}},
				InitStateValues:    make([]float64, width), StateHistoryDepth: 1, Seed: uint64(2*s + 2),
			})
		}
		gen.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 0.01},
			ExecutionStrategy:    strategy,
		})
		return gen
	}
}

// benchmarkTunedBranch compares, on ONE core (single wide inline system), the stock
// branching-coupled system (stock OU driver + branchResponder) against the fully-tuned one
// (tuned OU driver + tunedBranchResponder), against NumPy's optimized gather path (§3c) as
// reference — the case where single-core stock stochadex lost to hand-optimized NumPy.
func benchmarkTunedBranch() []processResult {
	const totalPaths, steps, repeats = 10000, 2000, 5
	configs := []struct {
		name string
		gen  func() *simulator.ConfigGenerator
	}{
		{"stock branch system (1 core inline)",
			branchCoupledGen(1, totalPaths, steps, &simulator.InlineExecution{})},
		{"tuned branch system (1 core inline)",
			branchCoupledGenTuned(1, totalPaths, steps, &simulator.InlineExecution{})},
	}
	seconds := map[string]float64{}
	for _, c := range configs {
		runProcessEnsemble(c.gen, 1, 1) // warm
		best := time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			if d := runProcessEnsemble(c.gen, 1, 1); d < best {
				best = d
			}
		}
		seconds[c.name] = best.Seconds()
		fmt.Printf("  %-40s %.3fs\n", c.name, best.Seconds())
	}
	return []processResult{{Process: "tuned_branch", TotalPaths: totalPaths, Steps: steps, Seconds: seconds}}
}

// stratGen builds one simulation of numPartitions independent busy partitions.
func stratGen(numPartitions, width, ops, steps int, strategy simulator.ExecutionStrategy) func() *simulator.ConfigGenerator {
	return func() *simulator.ConfigGenerator {
		gen := simulator.NewConfigGenerator()
		for p := 0; p < numPartitions; p++ {
			gen.SetPartition(&simulator.PartitionConfig{
				Name:              fmt.Sprintf("p%d", p),
				Iteration:         &busyIteration{},
				Params:            simulator.NewParams(map[string][]float64{"ops": {float64(ops)}}),
				InitStateValues:   fill(width, 0.5),
				StateHistoryDepth: 1,
				Seed:              uint64(p + 1),
			})
		}
		gen.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			ExecutionStrategy:    strategy,
		})
		return gen
	}
}

// timeSteps builds a coordinator (setup excluded from timing — e.g. the persistent
// worker pool is created once here) and times only its step loop.
func timeSteps(gen func() *simulator.ConfigGenerator) time.Duration {
	settings, impl := gen().GenerateConfigs()
	coord := simulator.NewPartitionCoordinator(settings, impl)
	start := time.Now()
	coord.Run() // applies the configured ExecutionStrategy
	return time.Since(start)
}

// allocsSteps returns the number of heap allocations during one step loop.
func allocsSteps(gen func() *simulator.ConfigGenerator) uint64 {
	settings, impl := gen().GenerateConfigs()
	coord := simulator.NewPartitionCoordinator(settings, impl)
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	coord.Run() // applies the configured ExecutionStrategy
	runtime.ReadMemStats(&m1)
	return m1.Mallocs - m0.Mallocs
}

type strategyResult struct {
	Regime  string             `json:"regime"`
	Detail  string             `json:"detail"`
	Seconds map[string]float64 `json:"seconds"`  // strategy -> best step-loop seconds
	AllocsK map[string]float64 `json:"allocs_k"` // strategy -> heap allocs during the run (thousands)
}

// benchmarkStrategies sweeps regimes designed so a different execution strategy wins
// each, reporting both wall-clock and allocations — the "pick the right strategy" knob.
func benchmarkStrategies() []strategyResult {
	const repeats = 5
	strategies := []struct {
		name string
		make func() simulator.ExecutionStrategy
	}{
		{"inline", func() simulator.ExecutionStrategy { return &simulator.InlineExecution{} }},
		{"spawn-per-step", func() simulator.ExecutionStrategy { return &simulator.SpawnPerStepExecution{} }},
		{"persistent-worker", func() simulator.ExecutionStrategy { return &simulator.PersistentWorkerExecution{} }},
	}
	regimes := []struct {
		name, detail            string
		parts, width, ops, step int
	}{
		{"few partitions, light work, many steps", "1 partition, width 8, ops 1, 8000 steps", 1, 8, 1, 8000},
		{"many partitions, light work, many steps", "24 partitions, width 8, ops 1, 8000 steps", 24, 8, 1, 8000},
		{"many partitions, heavy work", "24 partitions, width 64, ops 400, 400 steps", 24, 64, 400, 400},
	}
	var out []strategyResult
	for _, rg := range regimes {
		res := strategyResult{Regime: rg.name, Detail: rg.detail, Seconds: map[string]float64{}, AllocsK: map[string]float64{}}
		for _, s := range strategies {
			gen := stratGen(rg.parts, rg.width, rg.ops, rg.step, s.make())
			timeSteps(gen) // warm
			best := time.Duration(1 << 62)
			for r := 0; r < repeats; r++ {
				if d := timeSteps(gen); d < best {
					best = d
				}
			}
			res.Seconds[s.name] = best.Seconds()
			res.AllocsK[s.name] = float64(allocsSteps(gen)) / 1000
			fmt.Printf("  %-40s %-18s %.3fs  %.0fk allocs\n", rg.name, s.name, best.Seconds(), res.AllocsK[s.name])
		}
		out = append(out, res)
	}
	return out
}

func writeJSON(name string, v any) {
	dir := "benchmarks/results"
	if _, err := os.Stat("results"); err == nil {
		dir = "results" // running from inside benchmarks/
	}
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
}

func main() {
	meta := map[string]any{
		"go_version": runtime.Version(),
		"goarch":     runtime.GOARCH,
		"goos":       runtime.GOOS,
		"num_cpu":    runtime.NumCPU(),
		"gomaxprocs": runtime.GOMAXPROCS(0),
	}
	fmt.Printf("machine: %s/%s, %d CPU, Go %s\n", meta["goos"], meta["goarch"], meta["num_cpu"], meta["go_version"])

	// `go run ./benchmarks tuned` regenerates only the stock-vs-tuned OU result (§3
	// addendum) without re-running the heavy engine suite.
	if len(os.Args) > 1 && os.Args[1] == "tuned" {
		fmt.Println("stock vs tuned OU iteration (single-core, pure-Go):")
		writeJSON("tuned_ou_go.json", benchmarkTunedOU())
		fmt.Println("stock vs tuned branching-coupled system (single-core, pure-Go):")
		writeJSON("tuned_branch_go.json", benchmarkTunedBranch())
		fmt.Println("wrote benchmarks/results/tuned_{ou,branch}_go.json")
		return
	}

	fmt.Println("ensemble scaling (independent simulations, RunSeededEnsemble):")
	scaling := benchmarkEnsembleScaling()
	fmt.Println("cold start:")
	cold := benchmarkColdStart()
	fmt.Println("whole-process simulation across execution models (engine vs NumPy):")
	procs := benchmarkProcesses()
	fmt.Println("coupled OU-chain across execution models (engine's home turf):")
	coupled := benchmarkCoupled()
	fmt.Println("branching-coupled (hard to vectorize) across execution models:")
	branch := benchmarkBranchCoupled()
	fmt.Println("execution strategies across regimes (where each shines):")
	strats := benchmarkStrategies()
	fmt.Println("stock vs tuned OU iteration (single-core, pure-Go):")
	tuned := benchmarkTunedOU()
	fmt.Println("stock vs tuned branching-coupled system (single-core, pure-Go):")
	tunedBranch := benchmarkTunedBranch()

	writeJSON("meta.json", meta)
	writeJSON("tuned_ou_go.json", tuned)
	writeJSON("tuned_branch_go.json", tunedBranch)
	writeJSON("strategies.json", strats)
	writeJSON("ensemble_scaling.json", scaling)
	writeJSON("cold_start.json", cold)
	writeJSON("processes_go.json", procs)
	writeJSON("coupled_go.json", coupled)
	writeJSON("branch_coupled_go.json", branch)
	fmt.Println("wrote benchmarks/results/*.json")
}
