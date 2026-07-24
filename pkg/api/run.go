package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/graph"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StepAndServeWebsocket steps a simulation and streams state updates over a
// websocket using simulator.WebsocketOutputFunction.
//
// Usage hints:
//   - The HTTP server mounts the websocket at handle and listens on address.
//   - stepDelay controls the delay between steps in milliseconds.
func StepAndServeWebsocket(
	generator *simulator.ConfigGenerator,
	stepDelay time.Duration,
	handle string,
	address string,
) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	http.HandleFunc(
		handle,
		func(w http.ResponseWriter, r *http.Request) {
			connection, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Println("Error upgrading to WebSocket:", err)
				return
			}
			defer connection.Close()

			var mutex sync.Mutex
			simulationConfig := generator.GetSimulation()
			simulationConfig.OutputFunction =
				simulator.NewWebsocketOutputFunction(connection, &mutex)
			generator.SetSimulation(simulationConfig)
			coordinator := simulator.NewPartitionCoordinator(
				generator.GenerateConfigs(),
			)

			// step under the configured execution strategy, sleeping between
			// steps so the websocket streams state at a watchable rate
			stepper := coordinator.NewStepper()
			defer stepper.Close()
			// terminate the for loop if the condition has been met
			for !coordinator.ReadyToTerminate() {
				stepper.Step()
				time.Sleep(stepDelay * time.Millisecond)
			}
		},
	)
	log.Fatal(http.ListenAndServe(address, nil))
}

// CheckForDeadlock reports whether the generator's within-step wiring
// (params_from_upstream) contains a dependency cycle that would deadlock the
// channel-based execution strategies (the default and persistent-worker
// strategies), returning a descriptive error naming the partitions in each
// cycle. It runs no simulation. Without this pre-flight check such a cycle
// surfaces only as an opaque runtime "all goroutines are asleep - deadlock!"
// with no indication of which partitions are at fault. See pkg/graph.
func CheckForDeadlock(generator *simulator.ConfigGenerator) error {
	cycles := graph.Build(generator).InjectCycles()
	if len(cycles) == 0 {
		return nil
	}
	names := generator.PartitionNames()
	groups := make([]string, len(cycles))
	for i, cycle := range cycles {
		members := make([]string, len(cycle))
		for j, index := range cycle {
			members[j] = names[index]
		}
		groups[i] = "[" + strings.Join(members, " ") + "]"
	}
	return fmt.Errorf(
		"api: simulation wiring will deadlock — params_from_upstream forms a "+
			"within-step dependency cycle among partitions %s. Break each cycle "+
			"by making at least one direction a lag-1 read (a state-history read "+
			"via params_as_partitions) instead of params_from_upstream",
		strings.Join(groups, ", "),
	)
}

// Run executes the configured simulation under the mode named by the config's
// run: block. The default (empty or "batch") preserves pre-run:-tier behaviour:
// serve a websocket when a socket config is active, otherwise run once to
// completion offline. "ensemble" runs one seeded member per seed concurrently.
func Run(config *ApiRunConfig, socket *SocketConfig) {
	// The macros: tier is its own run context — build storage, expand macros, run
	// them against storage, emit the result — with no main partitions or coordinator.
	if len(config.Macros) > 0 {
		storage, err := runMacros(config)
		if err != nil {
			log.Fatal(err)
		}
		printStorage(storage)
		return
	}
	generator := config.GetConfigGenerator()
	if err := CheckForDeadlock(generator); err != nil {
		log.Fatal(err)
	}
	switch config.Run.Mode {
	case "", "batch":
		runBatch(generator, socket)
	case "ensemble":
		if err := runEnsemble(config, generator.GetSimulation()); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf(
			"api: unknown run mode %q — expected \"batch\" or \"ensemble\"",
			config.Run.Mode,
		)
	}
}

// runBatch serves a websocket when the socket is active, otherwise runs the
// simulation once to completion.
func runBatch(generator *simulator.ConfigGenerator, socket *SocketConfig) {
	if socket.Active() {
		StepAndServeWebsocket(
			generator,
			time.Duration(socket.MillisecondDelay),
			socket.Handle,
			socket.Address,
		)
		return
	}
	coordinator := simulator.NewPartitionCoordinator(
		generator.GenerateConfigs(),
	)
	coordinator.Run()
}

// runEnsemble runs one member per configured seed via simulator.RunSeededEnsemble
// and writes each member's recorded trajectory to stdout, prefixed with its member
// index and seed.
//
// Members are rebuilt by re-loading the source file so each gets fresh, non-shared
// iteration instances (required by RunSeededEnsemble). Re-loading recovers the
// partitions but not the simulation block — its components are Go-expression
// strings today (yaml:"-"), resolved only by the code-generation step — so the
// already-resolved sim from the in-memory config is injected into each member. A
// shallow copy per member keeps the (stateless) components shared but gives each
// its own SimulationConfig struct. Once 2.5's registry gives the simulation
// components a data spelling, the injection can be dropped and the re-load becomes
// self-contained.
func runEnsemble(
	config *ApiRunConfig,
	resolvedSim *simulator.SimulationConfig,
) error {
	runs, err := ensembleRuns(config, resolvedSim)
	if err != nil {
		return err
	}
	printEnsemble(runs)
	return nil
}

// ensembleRuns validates the config for ensemble mode and runs one member per
// configured seed, returning the recorded members. It performs no output, so it
// is the testable core of runEnsemble.
func ensembleRuns(
	config *ApiRunConfig,
	resolvedSim *simulator.SimulationConfig,
) ([]simulator.EnsembleRun, error) {
	if len(config.Run.Seeds) == 0 {
		return nil, fmt.Errorf("api: ensemble run mode requires a non-empty run.seeds")
	}
	if config.sourcePath == "" {
		return nil, fmt.Errorf(
			"api: ensemble run mode requires a config loaded from a file " +
				"(members are rebuilt by re-loading it)",
		)
	}
	if len(config.Embedded) > 0 {
		return nil, fmt.Errorf(
			"api: ensemble run mode does not yet support embedded runs (their " +
				"simulation blocks cannot be rebuilt by a plain re-load)",
		)
	}
	if err := assertDataOnly(config); err != nil {
		return nil, err
	}
	build := func() *simulator.ConfigGenerator {
		generator := LoadApiRunConfigFromYaml(config.sourcePath).GetConfigGenerator()
		simCopy := *resolvedSim
		generator.SetSimulation(&simCopy)
		return generator
	}
	return simulator.RunSeededEnsemble(
		build, config.Run.Seeds, config.Run.Concurrency,
	), nil
}

// assertDataOnly reports an error unless every main partition has an iteration
// after re-loading from file — i.e. the config's partitions are fully specified by
// expressions, with no Go iteration: string that only ExtraCode would fill. Such a
// config cannot be rebuilt by a plain re-load, so ensemble mode rejects it with a
// clear message rather than failing later inside GenerateConfigs.
func assertDataOnly(config *ApiRunConfig) error {
	generator := LoadApiRunConfigFromYaml(config.sourcePath).GetConfigGenerator()
	for _, name := range generator.PartitionNames() {
		if generator.GetPartition(name).Iteration == nil {
			return fmt.Errorf(
				"api: ensemble run mode currently supports data-only configs; "+
					"partition %q has no iteration after loading (it relies on a "+
					"Go iteration: string, which ensemble cannot rebuild)",
				name,
			)
		}
	}
	return nil
}

// printStorage writes every recorded row of a StateTimeStorage to stdout in the
// StdoutOutputFunction format (<time> <partition> [values]).
func printStorage(storage *simulator.StateTimeStorage) {
	times := storage.GetTimes()
	for _, name := range storage.GetNames() {
		for step, row := range storage.GetValues(name) {
			fmt.Printf("%v %s %v\n", times[step], name, row)
		}
	}
}

// printEnsemble writes every recorded row of every member to stdout, matching the
// StdoutOutputFunction format (<time> <partition> [values]) with a member prefix.
func printEnsemble(runs []simulator.EnsembleRun) {
	for member, run := range runs {
		times := run.Storage.GetTimes()
		for _, name := range run.Storage.GetNames() {
			values := run.Storage.GetValues(name)
			for step, row := range values {
				fmt.Printf(
					"member=%d seed=%d %v %s %v\n",
					member, run.Seed, times[step], name, row,
				)
			}
		}
	}
}

// GoToolchainMissingMessage is what a user sees when a config names Go
// expressions on a machine (or in an image) with no Go toolchain. It names the
// way out in both directions, because either can be the right fix: install Go,
// or move the config onto the data surface that needs no toolchain at all.
const GoToolchainMissingMessage = "this config names Go expressions, which " +
	"requires a Go toolchain — the run is executed by generating a program and " +
	"calling `go run`. Either install Go, or state the config purely as data " +
	"({type: ...} iterations, expressions:, and {type: ...} simulation " +
	"components), which resolves and runs in-process with no toolchain. The " +
	"published container image carries the data path only."

// checkGoToolchain reports whether the Go toolchain needed by the code-generation
// path is available. lookPath is injected so the failure is testable without
// manipulating PATH.
func checkGoToolchain(lookPath func(string) (string, error)) error {
	if _, err := lookPath("go"); err != nil {
		return errors.New(GoToolchainMissingMessage)
	}
	return nil
}

// RunWithParsedArgs runs the configured simulation. A fully-data config (data-spec
// partitions and simulation, no Go anywhere) is resolved and run in-process with no
// toolchain; any config that names Go is run by generating a temporary main program
// and executing it via go run, which is what enables dynamic iteration wiring.
func RunWithParsedArgs(args ParsedArgs) {
	// Stamp the run's provenance to stderr before anything else, so the very first
	// line of a job log ties whatever this run produces to the exact build (and,
	// when the orchestrator supplies it, the exact image) that produced it. It is
	// emitted once per invocation here — the codegen path below execs a freshly
	// compiled program that drives the engine directly, not through this function,
	// so there is no double stamp.
	LogRunProvenance(os.Stderr)

	// A fully-data config, or any macros: config (analysis runs are always
	// in-process and use pkg/analysis, which the generated program does not import),
	// runs directly.
	if args.ConfigStrings.IsFullyData() || len(args.ConfigStrings.Macros) > 0 {
		Run(
			LoadApiRunConfigFromYaml(args.ConfigFile),
			LoadSocketConfigFromYaml(args.SocketFile),
		)
		return
	}

	// This config names Go, so it can only run by code generation. Check for the
	// toolchain before generating anything: without this the failure surfaces as an
	// opaque `exec: "go": executable file not found in $PATH` panic from the middle
	// of a run, which says nothing about which half of the config surface the user
	// is on. Distributions that deliberately omit the toolchain land here — the
	// container image most of all, since it carries the data path only.
	if err := checkGoToolchain(exec.LookPath); err != nil {
		fmt.Fprintln(os.Stderr, "\nerror: "+err.Error())
		os.Exit(1)
	}

	// hydrate the template code and write it to a /tmp/*main.go
	fileName := WriteMainProgram(args)
	defer os.Remove(fileName)

	// execute the code
	runCmd := exec.Command("go", "run", fileName)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		panic(err)
	}
}
