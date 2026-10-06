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
	"gonum.org/v1/gonum/mat"
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
	if len(config.Macros) == 0 && config.Run.Mode == "ensemble" && config.outputViews != nil {
		// Members go to their own output views instead of being printed.
		_, err := RunWith(config, WithConfigOutputs())
		return err
	}
	if len(config.Macros) > 0 || config.Run.Mode == "ensemble" {
		result, err := RunToStorage(config)
		if err != nil {
			return err
		}
		switch {
		case result.Members != nil:
			printEnsemble(result.Members)
		case config.outputViews != nil:
			replayThroughViews(result.Storage, config.outputViews)
		default:
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

// RunResult is what one run of a config produced. Storage holds a batch run's
// default view (RunToStorage) or a macros run's result; Views holds the
// in-memory views a caller attached with CaptureView, by name; Members holds an
// ensemble's members, index-aligned to run.seeds.
type RunResult struct {
	Storage *simulator.StateTimeStorage
	Views   map[string]*simulator.StateTimeStorage
	Members []simulator.EnsembleRun
}

// RunOption configures RunWith.
type RunOption func(*runOptions)

type runOptions struct {
	captures         []captureView
	teeConfigOutputs bool
}

type captureView struct {
	name      string
	condition simulator.OutputCondition
	// mirror records with the config's own output condition (RunToStorage).
	mirror bool
}

// CaptureView attaches an in-memory view to a run: the rows condition selects
// (every step when nil) come back in RunResult.Views[name].
func CaptureView(name string, condition simulator.OutputCondition) RunOption {
	return func(o *runOptions) {
		o.captures = append(o.captures, captureView{name: name, condition: condition})
	}
}

// WithConfigOutputs also runs the config's own outputs — its outputs: views or
// output_function, and any nested run's sinks — alongside the captured views.
// Without it they are suppressed: nothing is written outside the process.
func WithConfigOutputs() RunOption {
	return func(o *runOptions) { o.teeConfigOutputs = true }
}

// RunWith runs a config, returning the in-memory views the caller attached with
// CaptureView. By default the config's own outputs are suppressed — including
// the sinks of nested (embedded) runs — so the run has no side effects outside
// the process; WithConfigOutputs writes them as well. The caller's config is
// not modified. Every failure is returned as an error, never an exit.
//
// A macros: config's result is also returned as RunResult.Storage, and each
// captured view receives it replayed as a live run would output it. An ensemble
// returns RunResult.Members; with WithConfigOutputs each member also writes its
// own outputs: views ({member} / {seed} substituted). Captured views do not yet
// apply to ensembles and are rejected.
func RunWith(config *ApiRunConfig, options ...RunOption) (*RunResult, error) {
	opts := runOptions{}
	for _, option := range options {
		option(&opts)
	}
	seen := make(map[string]bool, len(opts.captures))
	for _, capture := range opts.captures {
		if seen[capture.name] {
			return nil, &Error{Kind: ErrUsage, Err: fmt.Errorf(
				"api: CaptureView names view %q twice", capture.name)}
		}
		seen[capture.name] = true
	}
	// Captures the caller asked for, as opposed to RunToStorage's mirror view.
	requested := make([]captureView, 0, len(opts.captures))
	for _, capture := range opts.captures {
		if !capture.mirror {
			requested = append(requested, capture)
		}
	}
	if len(config.Macros) > 0 {
		storage, err := runMacros(config)
		if err != nil {
			return nil, err
		}
		// The macro result is already the full storage, so a mirror view is not
		// replayed; only requested views are.
		result := &RunResult{Storage: storage, Views: map[string]*simulator.StateTimeStorage{}}
		if len(requested) > 0 {
			replayThroughViews(storage, captureViews(requested, nil, result.Views))
		}
		if opts.teeConfigOutputs && config.outputViews != nil {
			replayThroughViews(storage, config.outputViews)
		}
		return result, nil
	}
	if config.Run.Mode == "ensemble" && len(requested) > 0 {
		return nil, &Error{Kind: ErrUsage, Err: fmt.Errorf(
			"api: captured views do not yet apply to ensemble runs; each member's " +
				"storage is returned in RunResult.Members")}
	}
	generator, err := preparedMainGeneratorWith(config, !opts.teeConfigOutputs)
	if err != nil {
		return nil, err
	}
	switch config.Run.Mode {
	case "", "batch":
		views := map[string]*simulator.StateTimeStorage{}
		simulation := *generator.GetSimulation()
		outputs := captureViews(opts.captures, simulation.OutputCondition, views)
		if opts.teeConfigOutputs {
			outputs.Views = append(outputs.Views, configViews(&simulation)...)
		}
		simulation.OutputFunction = outputs
		simulation.OutputCondition = &simulator.EveryStepOutputCondition{}
		generator.SetSimulation(&simulation)
		simulator.NewPartitionCoordinator(generator.GenerateConfigs()).Run()
		return &RunResult{Views: views}, nil
	case "ensemble":
		members, err := ensembleRuns(config, generator.GetSimulation(), opts.teeConfigOutputs)
		if err != nil {
			return nil, configError(err)
		}
		return &RunResult{Members: members}, nil
	default:
		return nil, configError(unknownRunModeError(config.Run.Mode))
	}
}

// captureViews builds one in-memory view per capture, registering each storage
// in into. A mirror capture uses configCondition.
func captureViews(
	captures []captureView,
	configCondition simulator.OutputCondition,
	into map[string]*simulator.StateTimeStorage,
) *simulator.OutputViews {
	views := &simulator.OutputViews{}
	for _, capture := range captures {
		condition := capture.condition
		switch {
		case capture.mirror && configCondition != nil:
			condition = configCondition
		case condition == nil:
			condition = &simulator.EveryStepOutputCondition{}
		}
		storage := simulator.NewStateTimeStorage()
		into[capture.name] = storage
		views.Views = append(views.Views, simulator.OutputView{
			Name: capture.name, Condition: condition,
			Function: &simulator.StateTimeStorageOutputFunction{Store: storage},
		})
	}
	return views
}

// configViews returns the config's own outputs as views: its outputs: views as
// they are, or its output_condition / output_function pair as one view.
func configViews(simulation *simulator.SimulationConfig) []simulator.OutputView {
	if views, ok := simulation.OutputFunction.(*simulator.OutputViews); ok {
		return views.Views
	}
	if simulation.OutputFunction == nil {
		return nil
	}
	return []simulator.OutputView{{
		Name: "config", Condition: simulation.OutputCondition, Function: simulation.OutputFunction,
	}}
}

// RunToStorage runs a config and returns what it produced instead of emitting it:
// RunWith with one in-memory view that records what the config's own
// output_condition selects (every step under outputs:), returned as
// RunResult.Storage, and the config's outputs — nested runs' sinks included —
// suppressed. A macros: config returns its result; an ensemble its members.
// Every partition is registered in the storage; one the condition never selects
// is present with no rows. The caller's config is not modified. Every failure is
// returned as an error rather than exiting.
//
// Serving a websocket is not a storage-returning run and is only reachable
// through Run.
func RunToStorage(config *ApiRunConfig) (*RunResult, error) {
	const defaultView = "\x00default"
	result, err := RunWith(config, func(o *runOptions) {
		o.captures = append(o.captures, captureView{name: defaultView, mirror: true})
	})
	if err != nil {
		return nil, err
	}
	if storage, ok := result.Views[defaultView]; ok && result.Storage == nil {
		result.Storage = storage
	}
	delete(result.Views, defaultView)
	return result, nil
}

// preparedMainGenerator validates a main:-path config and returns its generator,
// pre-flighted for within-step deadlocks and for wiring that cannot be built (an
// expression or upstream naming a partition that does not exist, an index out
// of range). Building the generator panics on those, so the panic is recovered
// here and returned as an ErrConfig error before anything runs.
func preparedMainGenerator(config *ApiRunConfig) (*simulator.ConfigGenerator, error) {
	return preparedMainGeneratorWith(config, false)
}

// preparedMainGeneratorWith is preparedMainGenerator, optionally building nested
// runs with their own sinks silenced (see ApiRunConfig.configGenerator).
func preparedMainGeneratorWith(
	config *ApiRunConfig,
	silenceNested bool,
) (generator *simulator.ConfigGenerator, err error) {
	if err := validateMainContext(config); err != nil {
		return nil, configError(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			generator = nil
			err = configError(fmt.Errorf("invalid config wiring: %v", recovered))
		}
	}()
	generator = config.configGenerator(silenceNested)
	if err := CheckForDeadlock(generator); err != nil {
		return nil, configError(err)
	}
	generator.GenerateConfigs()
	return generator, nil
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
	return ensembleRuns(config, generator.GetSimulation(), false)
}

// ensembleRuns validates the config for ensemble mode and runs one member per
// configured seed, returning the recorded members. It performs no output, so it
// is the testable core of runEnsemble.
func ensembleRuns(
	config *ApiRunConfig,
	resolvedSim *simulator.SimulationConfig,
	writeOutputs bool,
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
	if writeOutputs && len(config.Outputs) > 0 {
		// Each member re-loads the config (fresh iterations) and gets its own
		// views, with {member} / {seed} substituted, teed with its storage.
		build := func(member int, seed uint64) *simulator.ConfigGenerator {
			memberConfig := LoadApiRunConfigFromYaml(config.sourcePath)
			generator := memberConfig.GetConfigGenerator()
			simCopy := *resolvedSim
			simCopy.OutputFunction = memberConfig.memberOutputViews(member, seed)
			simCopy.OutputCondition = &simulator.EveryStepOutputCondition{}
			generator.SetSimulation(&simCopy)
			return generator
		}
		return simulator.RunSeededEnsembleMembers(
			build, config.Run.Seeds, config.Run.Concurrency), nil
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

// replayThroughViews sends a finished storage through output views as though
// each row had been output by a live run, so every view's condition selects
// exactly what it would have: row k carries step number k, and the history
// holds the previous row's time with the increment to row k's. Partitions go
// out in name order within a row. This is how a macros: run reaches outputs:
// until macros expand into the one live runtime (PLAN.md Phase 2).
func replayThroughViews(storage *simulator.StateTimeStorage, views *simulator.OutputViews) {
	names := sortedNames(storage)
	settings := &simulator.Settings{Iterations: make([]simulator.IterationSettings, len(names))}
	for i, name := range names {
		settings.Iterations[i].Name = name
	}
	views.Configure(settings)
	times := storage.GetTimes()
	for step, time := range times {
		previous := time
		if step > 0 {
			previous = times[step-1]
		}
		history := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{previous}),
			NextIncrement:     time - previous,
			CurrentStepNumber: step,
			StateHistoryDepth: 1,
		}
		for _, name := range names {
			rows := storage.GetValues(name)
			if step < len(rows) {
				views.OutputStep(name, rows[step], history, time)
			}
		}
	}
	views.Finalize()
}
