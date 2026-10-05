package api

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
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
//   - build is called once per connection and must return a generator with
//     fresh iteration instances, so concurrent clients never share state.
//   - allowedOrigins extends the default origin policy (see websocketOriginCheck).
func StepAndServeWebsocket(
	build func() *simulator.ConfigGenerator,
	stepDelay time.Duration,
	handle string,
	address string,
	allowedOrigins []string,
) {
	mux := http.NewServeMux()
	mux.Handle(handle, NewWebsocketHandler(build, stepDelay, allowedOrigins))
	log.Fatal(http.ListenAndServe(address, mux))
}

// NewWebsocketHandler returns the http.Handler behind StepAndServeWebsocket:
// each connection gets its own simulation, built fresh by build, stepped under
// the configured execution strategy and streamed until it terminates or the
// client disconnects.
//
// Building per connection is load-bearing. A ConfigGenerator hands out the same
// iteration instances on every GenerateConfigs call, so reusing one generator
// would have concurrent clients stepping one shared, mutable model.
func NewWebsocketHandler(
	build func() *simulator.ConfigGenerator,
	stepDelay time.Duration,
	allowedOrigins []string,
) http.Handler {
	upgrader := websocket.Upgrader{
		CheckOrigin: websocketOriginCheck(allowedOrigins),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Error upgrading to WebSocket:", err)
			return
		}
		defer connection.Close()

		// gorilla only processes control frames (including close) while
		// reading, so drain reads to learn when the client has gone away
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				if _, _, err := connection.ReadMessage(); err != nil {
					return
				}
			}
		}()

		var mutex sync.Mutex
		generator := build()
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
		for !coordinator.ReadyToTerminate() {
			select {
			case <-closed:
				return
			default:
			}
			stepper.Step()
			time.Sleep(stepDelay * time.Millisecond)
		}
	})
}

// websocketOriginCheck admits a websocket upgrade when the request carries no
// Origin header (a non-browser client), when the origin is the server's own
// host, when it is a loopback host on any port (the usual local dashboard
// setup), or when it is listed in allowed ("*" admits every origin). Any other
// browser origin is refused, so a page on an arbitrary site cannot drive a
// local simulation server.
func websocketOriginCheck(allowed []string) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		for _, entry := range allowed {
			if entry == "*" || strings.EqualFold(entry, origin) {
				return true
			}
		}
		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}
		if strings.EqualFold(parsed.Host, r.Host) {
			return true
		}
		switch parsed.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return true
		}
		return false
	}
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
// completion offline, emitting through the config's output_function.
// "ensemble" runs one seeded member per seed concurrently. Macros and ensembles
// have no output_function of their own, so Run prints what RunToStorage returns.
//
// A failure exits the process; a panic raised while running propagates. Use
// Execute (as the CLI does) for classified errors and exit codes instead.
func Run(config *ApiRunConfig, socket *SocketConfig) {
	if err := runChecked(config, socket); err != nil {
		log.Fatal(err)
	}
}

// runChecked is Run, returning its failure instead of exiting.
func runChecked(config *ApiRunConfig, socket *SocketConfig) error {
	if len(config.Macros) > 0 || config.Run.Mode == "ensemble" {
		result, err := RunToStorage(config)
		if err != nil {
			return err
		}
		if result.Members != nil {
			printEnsemble(result.Members)
		} else {
			printStorage(result.Storage)
		}
		return nil
	}
	generator, err := preparedMainGenerator(config)
	if err != nil {
		return err
	}
	switch config.Run.Mode {
	case "", "batch":
		runBatch(config, generator, socket)
		return nil
	default:
		return configError(unknownRunModeError(config.Run.Mode))
	}
}

// runE is runChecked with panics raised while running recovered and classified
// (see panicError), so every failure the engine can intercept comes back as an
// *Error. A panic inside a partition's worker goroutine cannot be intercepted
// and still crashes the process.
func runE(config *ApiRunConfig, socket *SocketConfig) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicError(recovered)
		}
	}()
	return runChecked(config, socket)
}

// Execute is the CLI's whole run: parse args (os.Args form), load the config and
// optional socket config, and run. It never exits or panics on a failure it can
// intercept; it returns an *Error whose kind ExitCode maps to the process exit
// status. Unlike ArgParse / LoadSocketConfigFromYaml, it writes nothing to
// stdout of its own, so stdout carries only the run's output.
func Execute(args []string) error {
	parsed, err := parseArgs(args)
	if err != nil {
		return err
	}
	LogRunProvenance(os.Stderr)
	config, err := LoadConfig(parsed.ConfigFile)
	if err != nil {
		return err
	}
	socket, err := loadSocketConfig(parsed.SocketFile)
	if err != nil {
		return err
	}
	return runE(config, socket)
}

// RunResult is what one run of a config produced. Exactly one field is set:
// Storage for a batch or macros run, Members for an ensemble run (index-aligned
// to run.seeds).
type RunResult struct {
	Storage *simulator.StateTimeStorage
	Members []simulator.EnsembleRun
}

// RunToStorage runs a config and returns what it produced instead of emitting it.
// It is the single programmatic entry point for every run shape — a macros: tier,
// a batch run, or a run: {mode: ensemble} — and it returns every failure as an
// error rather than exiting, so a library caller (a test, a downstream pipeline,
// an orchestrated step) can act on it.
//
// What is recorded is exactly what the config's output_condition selects, written
// to storage in place of the config's output_function, which is not invoked. This
// is the rule ensemble members already follow. Every partition is registered in
// the storage; one the condition never selects is present with no rows. The caller's config is not
// modified, so it can be run again (or passed to Run) afterwards.
//
// Serving a websocket is not a storage-returning run and is only reachable
// through Run.
func RunToStorage(config *ApiRunConfig) (*RunResult, error) {
	if len(config.Macros) > 0 {
		storage, err := runMacros(config)
		if err != nil {
			return nil, err
		}
		return &RunResult{Storage: storage}, nil
	}
	generator, err := preparedMainGenerator(config)
	if err != nil {
		return nil, err
	}
	switch config.Run.Mode {
	case "", "batch":
		return &RunResult{Storage: runBatchToStorage(generator)}, nil
	case "ensemble":
		members, err := ensembleRuns(config, generator.GetSimulation())
		if err != nil {
			return nil, configError(err)
		}
		return &RunResult{Members: members}, nil
	default:
		return nil, configError(unknownRunModeError(config.Run.Mode))
	}
}

// preparedMainGenerator validates a main:-path config and returns its generator,
// pre-flighted for within-step deadlocks and for wiring that cannot be built (an
// expression or upstream naming a partition that does not exist, an index out
// of range). Building the generator panics on those, so the panic is recovered
// here and returned as an ErrConfig error before anything runs.
func preparedMainGenerator(config *ApiRunConfig) (generator *simulator.ConfigGenerator, err error) {
	if err := validateMainContext(config); err != nil {
		return nil, configError(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			generator = nil
			err = configError(fmt.Errorf("invalid config wiring: %v", recovered))
		}
	}()
	generator = config.GetConfigGenerator()
	if err := CheckForDeadlock(generator); err != nil {
		return nil, configError(err)
	}
	generator.GenerateConfigs()
	return generator, nil
}

// runBatchToStorage runs the generator once to completion, recording what its
// output_condition selects into fresh storage. The simulation block is copied
// before its output_function is replaced: the generator holds a pointer to the
// config's own resolved block, so swapping it in place would rewrite the caller's
// config.
func runBatchToStorage(generator *simulator.ConfigGenerator) *simulator.StateTimeStorage {
	storage := simulator.NewStateTimeStorage()
	simulation := *generator.GetSimulation()
	simulation.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: storage}
	generator.SetSimulation(&simulation)
	simulator.NewPartitionCoordinator(generator.GenerateConfigs()).Run()
	return storage
}

func unknownRunModeError(mode string) error {
	return fmt.Errorf(
		"api: unknown run mode %q — expected \"batch\" or \"ensemble\"", mode)
}

// perConnectionBuild returns a builder that re-loads the config's source file, so
// every websocket connection gets fresh, non-shared iteration instances (the same
// mechanism ensemble mode uses for its members). A config built in memory has no
// file to re-load and is rejected.
func perConnectionBuild(
	config *ApiRunConfig,
) (func() *simulator.ConfigGenerator, error) {
	if config.sourcePath == "" {
		return nil, fmt.Errorf("api: serving a websocket requires a config " +
			"loaded from a file (each connection is rebuilt by re-loading it)")
	}
	return func() *simulator.ConfigGenerator {
		return LoadApiRunConfigFromYaml(config.sourcePath).GetConfigGenerator()
	}, nil
}

// validateMainContext rejects a data: block on a config with no macros:. Only the
// macros: tier reads data: — the main simulation never does — so on its own it
// looks load-bearing and does nothing.
func validateMainContext(config *ApiRunConfig) error {
	if config.Data != nil {
		return fmt.Errorf("api: a config sets data: but no macros:; data: is only " +
			"read by the macros: tier and the main simulation ignores it")
	}
	return nil
}

// runBatch serves a websocket when the socket is active, otherwise runs the
// simulation once to completion.
func runBatch(
	config *ApiRunConfig,
	generator *simulator.ConfigGenerator,
	socket *SocketConfig,
) {
	if socket.Active() {
		build, err := perConnectionBuild(config)
		if err != nil {
			log.Fatal(err)
		}
		StepAndServeWebsocket(
			build,
			time.Duration(socket.MillisecondDelay),
			socket.Handle,
			socket.Address,
			socket.AllowedOrigins,
		)
		return
	}
	coordinator := simulator.NewPartitionCoordinator(
		generator.GenerateConfigs(),
	)
	coordinator.Run()
}

// RunEnsembleToStorage runs the config's ensemble and returns each member's
// recorded storage, index-aligned to run.seeds, whatever run.mode says.
//
// The ensemble mechanism rebuilds each member by re-loading the source file for
// fresh, non-shared iteration instances, so: the config must have been loaded from
// a file; embedded runs are unsupported; every main partition must resolve a data
// iteration; and run.seeds must be non-empty.
//
// Deprecated: use RunToStorage with run: {mode: ensemble}, which returns the same
// members in RunResult.Members.
func RunEnsembleToStorage(
	config *ApiRunConfig,
) ([]simulator.EnsembleRun, error) {
	generator, err := preparedMainGenerator(config)
	if err != nil {
		return nil, err
	}
	return ensembleRuns(config, generator.GetSimulation())
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
// after re-loading from file. A partition with no iteration relies on an embedded
// run (rejected separately), so ensemble mode rejects it with a clear message
// rather than failing later inside GenerateConfigs.
func assertDataOnly(config *ApiRunConfig) error {
	generator := LoadApiRunConfigFromYaml(config.sourcePath).GetConfigGenerator()
	for _, name := range generator.PartitionNames() {
		if generator.GetPartition(name).Iteration == nil {
			return fmt.Errorf(
				"api: ensemble run mode requires every partition to resolve an "+
					"iteration; partition %q has none after loading",
				name,
			)
		}
	}
	return nil
}

// sortedNames returns a storage's partition names in a fixed order. GetNames
// reads a map, so printing in its order would make the CLI's output order differ
// from run to run.
func sortedNames(storage *simulator.StateTimeStorage) []string {
	names := storage.GetNames()
	sort.Strings(names)
	return names
}

// printStorage writes every recorded row of a StateTimeStorage to stdout in the
// StdoutOutputFunction format (<time> <partition> [values]), partitions in name
// order.
func printStorage(storage *simulator.StateTimeStorage) {
	times := storage.GetTimes()
	for _, name := range sortedNames(storage) {
		for step, row := range storage.GetValues(name) {
			fmt.Printf("%v %s %v\n", times[step], name, row)
		}
	}
}

// printEnsemble writes every recorded row of every member to stdout, matching the
// StdoutOutputFunction format (<time> <partition> [values]) with a member prefix,
// partitions in name order.
func printEnsemble(runs []simulator.EnsembleRun) {
	for member, run := range runs {
		times := run.Storage.GetTimes()
		for _, name := range sortedNames(run.Storage) {
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

// RunWithParsedArgs runs the configured simulation. The whole config is data
// (data-spec partitions and simulation, or expressions:, plus optional data:/macros:
// tiers), so it is resolved and run in-process with no Go toolchain.
func RunWithParsedArgs(args ParsedArgs) {
	// Stamp the run's provenance to stderr before anything else, so the very first
	// line of a job log ties whatever this run produces to the exact build (and,
	// when the orchestrator supplies it, the exact image) that produced it.
	LogRunProvenance(os.Stderr)

	Run(
		LoadApiRunConfigFromYaml(args.ConfigFile),
		LoadSocketConfigFromYaml(args.SocketFile),
	)
}
