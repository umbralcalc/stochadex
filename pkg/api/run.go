package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	return newStreamHandler(func(stream simulator.OutputFunction) (servedRun, error) {
		generator := build()
		simulation := generator.GetSimulation()
		simulation.OutputFunction = stream
		generator.SetSimulation(simulation)
		return servedRun{coordinator: simulator.NewPartitionCoordinator(
			generator.GenerateConfigs())}, nil
	}, stepDelay*time.Millisecond, allowedOrigins)
}

// servedRun is one connection's run: its coordinator, and the feeds that set
// its params between steps (nil without params_from_input).
type servedRun struct {
	coordinator *simulator.PartitionCoordinator
	feeds       *paramFeeds
	// outputs are the connection's own staged sinks, published when its run
	// ends cleanly (see staging.go).
	outputs *stagedOutputs
}

// newStreamHandler is the websocket handler for both serving paths. For each
// connection, build returns a fresh run whose output includes stream, the
// connection's websocket. The client's messages go to the run's served-client
// stream, if it has one. The run is stepped under its execution strategy, its
// feeds applied between steps, with pace after each, until it terminates or
// the client disconnects; its output is then finalized, so any sinks alongside
// the stream are flushed. If the run cannot be built, or fails, the client is
// sent a close frame giving the reason.
func newStreamHandler(
	build func(stream simulator.OutputFunction) (servedRun, error),
	pace time.Duration,
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

		var mutex sync.Mutex
		fail := func(err error) {
			log.Println("Error in a served run:", err)
			mutex.Lock()
			connection.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, closeReason(err)),
				time.Now().Add(time.Second))
			mutex.Unlock()
		}
		run, err := build(simulator.NewWebsocketOutputFunction(connection, &mutex))
		if err != nil {
			fail(err)
			return
		}

		// Read the client's messages: they go to the run's served-client
		// stream, if it has one. Reading also handles the close frame (gorilla
		// processes control frames only while reading), which is how the run
		// learns the client has gone.
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				_, message, err := connection.ReadMessage()
				if err != nil {
					return
				}
				if run.feeds != nil {
					run.feeds.deliverServed(message)
				}
			}
		}()

		// Step under the configured execution strategy, sleeping between steps
		// so the websocket streams state at a watchable rate. The client leaving
		// ends a session cleanly, so its outputs are published then too.
		if run.outputs != nil {
			defer run.outputs.discard()
		}
		err = runSteps(run.coordinator, run.feeds, pace, closed)
		if run.outputs != nil {
			err = run.outputs.finish(err)
		}
		if err != nil {
			fail(err)
		}
	})
}

// closeReason is err's text cut to fit a websocket close frame, whose reason
// must be valid UTF-8 within 123 bytes: whole characters, at most 120 bytes.
func closeReason(err error) string {
	reason := err.Error()
	for len(reason) > 120 {
		_, size := utf8.DecodeLastRuneInString(reason)
		reason = reason[:len(reason)-size]
	}
	return reason
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
// run: block. The default (empty or "batch") runs once to completion offline;
// "ensemble" runs one seeded member per seed concurrently, each writing its own
// views; "serve" serves a websocket, one fresh run per connection, until the
// server fails. A macros: run's result is replayed through the views. Every
// mode writes through the config's outputs: views, which hold the shorthand
// output pair or, when no output is declared, every step to stdout.
//
// An active socket config is a deprecated alias for run: {mode: serve}: it
// switches a batch config to serving with the socket's address, handle,
// origins and delay.
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
	config, err := withSocketAlias(config, socket, os.Stderr)
	if err != nil {
		return err
	}
	if len(config.Macros) > 0 || config.Run.Mode == "ensemble" {
		// A macros run's result is replayed through its views; each ensemble
		// member writes its own.
		_, err := RunWith(config, WithConfigOutputs())
		return err
	}
	prepared, err := preparedMainGenerator(config)
	if err != nil {
		return err
	}
	switch config.Run.Mode {
	case "", "batch":
		outputs := stagedOutputs{feeds: prepared.feeds}
		outputs.stageNested(config)
		outputs.stage(prepared.implementations.OutputFunction)
		defer outputs.discard()
		return outputs.finish(runCoordinator(
			simulator.NewPartitionCoordinator(prepared.settings, prepared.implementations),
			prepared.feeds))
	case "serve":
		return runServe(config)
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
	options, err := setOptions(parsed.Sets)
	if err != nil {
		return err
	}
	if parsed.SeedRange != "" {
		shard, err := seedRangeOption(parsed.SeedRange)
		if err != nil {
			return err
		}
		options = append(options, shard)
	}
	for _, spec := range parsed.DebugEmbedded {
		debug, err := debugEmbeddedOption(spec)
		if err != nil {
			return err
		}
		options = append(options, debug)
	}
	config, err := LoadConfig(parsed.ConfigFile, options...)
	if err != nil {
		return err
	}
	for _, notice := range config.Deprecations() {
		fmt.Fprintln(os.Stderr, "stochadex: "+notice)
	}
	if parsed.SeedRange != "" {
		warnSharedMemberNames(config, os.Stderr)
	}
	if parsed.InspectIO {
		return printJSON(Manifest(config))
	}
	if parsed.InspectProvenance {
		provenance, err := ComputeProvenance(config)
		if err != nil {
			return err
		}
		return printJSON(provenance)
	}
	socket, err := loadSocketConfig(parsed.SocketFile)
	if err != nil {
		return err
	}
	if parsed.Check {
		if config, err = withSocketAlias(config, socket, io.Discard); err != nil {
			return err
		}
		if err := Check(config); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "stochadex: %s is valid\n", parsed.ConfigFile)
		return nil
	}
	if !parsed.Provenance {
		return runE(config, socket)
	}
	// Fingerprinted before the run, from the inputs it is about to read.
	provenance, err := ComputeProvenance(config)
	if err != nil {
		return err
	}
	targets := sidecarTargets(config)
	if parsed.SkipIfUnchanged {
		current, why := upToDate(provenance, targets)
		if current {
			fmt.Fprintf(os.Stderr, "stochadex: skipped: every output already carries key %s\n",
				provenance.Key)
			return nil
		}
		fmt.Fprintf(os.Stderr, "stochadex: running: %s\n", why)
	}
	if err := runE(config, socket); err != nil {
		return err
	}
	return writeSidecars(provenance, targets)
}

// printJSON prints v to stdout as indented JSON.
func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", data)
	return err
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
			var outputs stagedOutputs
			outputs.stage(config.outputViews)
			defer outputs.discard()
			replayThroughViews(storage, config.outputViews)
			if err := outputs.finish(nil); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	if config.Run.Mode == "serve" {
		return nil, &Error{Kind: ErrUsage, Err: fmt.Errorf(
			"api: a serve config runs once per websocket connection; serve it with " +
				"Run / Execute, or run it once with run: {mode: batch}")}
	}
	if config.Run.Mode == "ensemble" && len(requested) > 0 {
		return nil, &Error{Kind: ErrUsage, Err: fmt.Errorf(
			"api: captured views do not yet apply to ensemble runs; each member's " +
				"storage is returned in RunResult.Members")}
	}
	prepared, err := preparedMainGeneratorWith(config, !opts.teeConfigOutputs)
	if err != nil {
		return nil, err
	}
	switch config.Run.Mode {
	case "", "batch":
		views := map[string]*simulator.StateTimeStorage{}
		simulation := prepared.generator.GetSimulation()
		outputs := captureViews(opts.captures, simulation.OutputCondition, views)
		if opts.teeConfigOutputs {
			outputs.Views = append(outputs.Views, configViews(simulation)...)
		}
		// Swap the output on the already-generated run rather than generating
		// it again: generating reconfigures every iteration.
		implementations := *prepared.implementations
		implementations.OutputFunction = outputs
		implementations.OutputCondition = &simulator.EveryStepOutputCondition{}
		staged := stagedOutputs{feeds: prepared.feeds}
		if opts.teeConfigOutputs {
			// Nested runs' sinks run only when the config's outputs do.
			staged.stageNested(config)
		}
		staged.stage(outputs)
		defer staged.discard()
		if err := staged.finish(runCoordinator(
			simulator.NewPartitionCoordinator(prepared.settings, &implementations),
			prepared.feeds)); err != nil {
			return nil, err
		}
		return &RunResult{Views: views}, nil
	case "ensemble":
		members, err := ensembleRuns(config, prepared.generator.GetSimulation(), opts.teeConfigOutputs)
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

// preparedRun is a main:-path config made ready to run: its generator, and the
// settings and implementations it generated, which the run uses as they are.
type preparedRun struct {
	generator       *simulator.ConfigGenerator
	settings        *simulator.Settings
	implementations *simulator.Implementations
	// feeds set params from inputs between steps (nil without
	// params_from_input; see streams.go).
	feeds *paramFeeds
}

// preparedMainGenerator validates a main:-path config and returns it ready to
// run, pre-flighted for within-step deadlocks and for wiring that cannot be
// built (an expression or upstream naming a partition that does not exist, an
// index out of range). Generating the configs panics on those, so the panic is
// recovered here and returned as an ErrConfig error before anything runs. The
// run uses the configs generated here: generating again would reconfigure every
// iteration, re-creating each one's random source.
func preparedMainGenerator(config *ApiRunConfig) (preparedRun, error) {
	return preparedMainGeneratorWith(config, false)
}

// preparedMainGeneratorWith is preparedMainGenerator, optionally building nested
// runs with their own sinks silenced (see ApiRunConfig.configGenerator).
func preparedMainGeneratorWith(
	config *ApiRunConfig,
	silenceNested bool,
) (prepared preparedRun, err error) {
	if err := validateMainContext(config); err != nil {
		return preparedRun{}, configError(err)
	}
	loader := inputLoader{config: config}
	if err := bindInputsWith(config, &loader); err != nil {
		return preparedRun{}, err
	}
	feeds, err := newParamFeeds(config, &loader)
	if err != nil {
		return preparedRun{}, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			prepared = preparedRun{}
			err = configError(fmt.Errorf("invalid config wiring: %v", recovered))
		}
	}()
	generator := config.configGenerator(silenceNested)
	if err := CheckForDeadlock(generator); err != nil {
		return preparedRun{}, configError(err)
	}
	settings, implementations := generator.GenerateConfigs()
	return preparedRun{generator: generator, settings: settings,
		implementations: implementations, feeds: feeds}, nil
}

func unknownRunModeError(mode string) error {
	return fmt.Errorf(
		"api: unknown run mode %q — expected \"batch\", \"ensemble\" or \"serve\"", mode)
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
	prepared, err := preparedMainGenerator(config)
	if err != nil {
		return nil, err
	}
	return ensembleRuns(config, prepared.generator.GetSimulation(), false)
}

// ensembleRuns validates the config for ensemble mode and runs one member per
// configured seed, returning the recorded members. It performs no output, so it
// is the testable core of runEnsemble.
func ensembleRuns(
	config *ApiRunConfig,
	resolvedSim *simulator.SimulationConfig,
	writeOutputs bool,
) ([]simulator.EnsembleRun, error) {
	if err := validateEnsemble(config); err != nil {
		return nil, err
	}
	if writeOutputs {
		// Each member re-loads the config (fresh iterations) and gets its own
		// views, with {member} / {seed} substituted, teed with its storage.
		// They are published together, once every member has finished.
		// Members are built concurrently, so each writes only its own slot.
		views := make([]*simulator.OutputViews, len(config.Run.Seeds))
		build := func(member int, seed uint64) *simulator.ConfigGenerator {
			memberConfig := mustReload(config)
			if err := bindInputs(memberConfig); err != nil {
				panic(err)
			}
			generator := memberConfig.GetConfigGenerator()
			simCopy := *resolvedSim
			memberViews := memberConfig.memberOutputViews(member, seed)
			memberViews.Stage()
			views[member] = memberViews
			simCopy.OutputFunction = memberViews
			simCopy.OutputCondition = &simulator.EveryStepOutputCondition{}
			generator.SetSimulation(&simCopy)
			return generator
		}
		outputs := &stagedOutputs{}
		defer outputs.discard()
		members := simulator.RunSeededEnsembleMembers(
			build, config.Run.Seeds, config.Run.Concurrency)
		for _, memberViews := range views {
			// Already staged as each member was built.
			outputs.add(memberViews)
		}
		return members, outputs.finish(nil)
	}
	build := func() *simulator.ConfigGenerator {
		memberConfig := mustReload(config)
		if err := bindInputs(memberConfig); err != nil {
			panic(err)
		}
		generator := memberConfig.GetConfigGenerator()
		simCopy := *resolvedSim
		generator.SetSimulation(&simCopy)
		return generator
	}
	return simulator.RunSeededEnsemble(
		build, config.Run.Seeds, config.Run.Concurrency,
	), nil
}

// validateEnsemble checks what an ensemble run needs before building members.
func validateEnsemble(config *ApiRunConfig) error {
	if len(config.Run.Seeds) == 0 {
		return fmt.Errorf("api: ensemble run mode requires a non-empty run.seeds")
	}
	if config.sourcePath == "" {
		return fmt.Errorf(
			"api: ensemble run mode requires a config loaded from a file " +
				"(members are rebuilt by re-loading it)",
		)
	}
	if len(config.Embedded) > 0 {
		return fmt.Errorf(
			"api: ensemble run mode does not yet support embedded runs (their " +
				"simulation blocks cannot be rebuilt by a plain re-load)",
		)
	}
	return assertDataOnly(config)
}

// mustReload is reload for an ensemble member's build, which cannot return an
// error. The document already loaded once, so a failure here is a bug.
func mustReload(config *ApiRunConfig) *ApiRunConfig {
	reloaded, err := config.reload()
	if err != nil {
		panic(err)
	}
	return reloaded
}

// assertDataOnly reports an error unless every main partition has an iteration
// after re-loading from file. A partition with no iteration relies on an embedded
// run (rejected separately), so ensemble mode rejects it with a clear message
// rather than failing later inside GenerateConfigs.
func assertDataOnly(config *ApiRunConfig) error {
	generator := mustReload(config).GetConfigGenerator()
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

// RunWithParsedArgs runs the configured simulation. The whole config is data
// (data-spec partitions and simulation, or expressions:, plus optional data:/macros:
// tiers), so it is resolved and run in-process with no Go toolchain.
func RunWithParsedArgs(args ParsedArgs) {
	// Stamp the run's provenance to stderr before anything else, so the very first
	// line of a job log ties whatever this run produces to the exact build (and,
	// when the orchestrator supplies it, the exact image) that produced it.
	LogRunProvenance(os.Stderr)

	options, err := setOptions(args.Sets)
	if err != nil {
		panic(err)
	}
	if args.SeedRange != "" {
		shard, err := seedRangeOption(args.SeedRange)
		if err != nil {
			panic(err)
		}
		options = append(options, shard)
	}
	config := LoadApiRunConfigFromYaml(args.ConfigFile, options...)
	for _, notice := range config.Deprecations() {
		fmt.Fprintln(os.Stderr, "stochadex: "+notice)
	}
	Run(
		config,
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
