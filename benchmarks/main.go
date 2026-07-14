// Command benchmarks measures stochadex's fair, CPU-to-CPU performance claims and
// writes the results as JSON (consumed by plot.py to render the committed plots).
//
// Deliberately NOT a peak-FLOPs race against GPU frameworks (JAX/Julia) — those win
// on their own hardware and problem shapes, and comparing them on a laptop CPU would
// be apples-to-oranges. These are the systems claims that are actually stochadex's:
//
//  1. partition-scaling — throughput vs partition count (goroutine concurrency scaling)
//  2. cold-start        — time to first result (warmup-free, no JIT/interpreter)
//  3. vectorized-ops    — gonum vector-op throughput (compared against NumPy by numpy_ops.py)
//
// Run: `go run ./benchmarks` (from the repo root) or `go run .` from here. Writes
// benchmarks/results/*.json. Numbers are machine-specific — record the machine in the
// README when committing results.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gonum.org/v1/gonum/blas/blas64"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

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
	coord := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coord.ReadyToTerminate() {
		coord.Step(&wg)
	}
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
	Repeats             int     `json:"repeats"`
	MedianMicroseconds  float64 `json:"median_microseconds"`
	Note                string  `json:"note"`
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

type opPoint struct {
	Size        int     `json:"size"`
	Op          string  `json:"op"`
	BestSeconds float64 `json:"best_seconds"`
	GFLOPs      float64 `json:"gflops"`
}

// benchmarkVectorizedOps measures gonum (BLAS-backed) throughput for the elementwise
// and reduction vector ops a partition does on its state — AXPY (y += a·x) and DOT.
// numpy_ops.py runs the identical ops so the plot can show CPU-to-CPU parity.
func benchmarkVectorizedOps() []opPoint {
	sizes := []int{1_000, 10_000, 100_000, 1_000_000, 10_000_000}
	const repeats = 20
	var out []opPoint
	for _, n := range sizes {
		x := make([]float64, n)
		y := make([]float64, n)
		for i := range x {
			x[i], y[i] = float64(i%7)+0.5, float64(i%5)+0.5
		}
		vx := blas64.Vector{N: n, Inc: 1, Data: x}
		vy := blas64.Vector{N: n, Inc: 1, Data: y}

		// AXPY: 2n flops.
		best := time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			start := time.Now()
			blas64.Axpy(1.0000001, vx, vy)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		out = append(out, opPoint{Size: n, Op: "axpy", BestSeconds: best.Seconds(),
			GFLOPs: 2 * float64(n) / best.Seconds() / 1e9})

		// DOT: 2n flops.
		best = time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			start := time.Now()
			_ = blas64.Dot(vx, vy)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		out = append(out, opPoint{Size: n, Op: "dot", BestSeconds: best.Seconds(),
			GFLOPs: 2 * float64(n) / best.Seconds() / 1e9})
		fmt.Printf("  vec n=%-9d  axpy %.2f GFLOP/s  dot %.2f GFLOP/s\n",
			n, out[len(out)-2].GFLOPs, out[len(out)-1].GFLOPs)
	}
	return out
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
			coord := simulator.NewPartitionCoordinator(settings, impl)
			var w sync.WaitGroup
			for !coord.ReadyToTerminate() {
				coord.Step(&w)
			}
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

	fmt.Println("ensemble scaling (independent simulations, RunSeededEnsemble):")
	scaling := benchmarkEnsembleScaling()
	fmt.Println("cold start:")
	cold := benchmarkColdStart()
	fmt.Println("vectorized ops (gonum):")
	ops := benchmarkVectorizedOps()
	fmt.Println("whole-process simulation across execution models (engine vs NumPy):")
	procs := benchmarkProcesses()
	fmt.Println("coupled OU-chain across execution models (engine's home turf):")
	coupled := benchmarkCoupled()

	writeJSON("meta.json", meta)
	writeJSON("ensemble_scaling.json", scaling)
	writeJSON("cold_start.json", cold)
	writeJSON("vectorized_ops_go.json", ops)
	writeJSON("processes_go.json", procs)
	writeJSON("coupled_go.json", coupled)
	fmt.Println("wrote benchmarks/results/*.json")
}
