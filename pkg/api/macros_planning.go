package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// mcts_planning searches over the forward model itself: the dynamics are already
// partitions, so an action is a params injection, a transition is one step of the
// model, and the reward is a partition of it. Nothing has to be named from
// outside, unlike mcts_self_play, whose environment is Go decision rules reached
// through the environment registry.
//
// It is a sibling of evolution_strategy_optimisation, not a replacement: that
// optimises a policy parameterisation once and globally, this optimises an action
// sequence from the current state and replans every step.
//
// A planned return is one scenario's outcome, not a forecast. Vary scenario_seed
// and aggregate — run: ensemble is the easy way.
type mctsPlanningSpec struct {
	macroTypeField `yaml:",inline"`
	// Name prefixes the generated partitions. "<name>_apply" carries the model
	// state as it is driven, which is normally what you read output from.
	Name string `yaml:"name"`
	// Steps is the number of decisions to take.
	Steps int `yaml:"steps"`
	// The environment: the forward model and the decision problem over it.
	simulationEnvironmentSpec `yaml:",inline"`
	// PinnedNoise turns chance nodes OFF, reverting to a search that commits to
	// the first successor drawn for each action.
	//
	// Chance nodes are the default because a plan's value is usually going to be
	// believed, and without them the search plans as though it knew which way the
	// dice would fall: on the shipped example its over-promise runs to tens of
	// percent, and *grows* with the simulation budget, since more search means
	// more exploitation of the one realisation it can see. Pinning is faster and
	// costs nothing when the model is deterministic — which is the case worth
	// setting this for.
	PinnedNoise bool `yaml:"pinned_noise,omitempty"`
	// Search hyperparameters; unset values fall back to the agents defaults.
	SimsPerDecision int     `yaml:"sims_per_decision,omitempty"`
	Exploration     float64 `yaml:"exploration,omitempty"`
	MaxTreeDepth    int     `yaml:"max_tree_depth,omitempty"`
	RolloutMaxSteps int     `yaml:"rollout_max_steps,omitempty"`
	Seed            uint64  `yaml:"seed,omitempty"`
	Timestep        float64 `yaml:"timestep,omitempty"`
}

// simulationEnvironmentSpec is a decision problem over a forward model: an
// action is a params injection, a transition is one step of the model, and the
// reward is a partition of it. It is mcts_planning's environment, and the
// {type: simulation} environment of the mcts_* iterations.
type simulationEnvironmentSpec struct {
	// Horizon is how far each search looks ahead, in model steps.
	Horizon int `yaml:"horizon"`
	// Actions is the discrete action set: each entry is the params value written
	// to ActionParam on ActionPartition.
	Actions         [][]float64 `yaml:"actions"`
	ActionPartition string      `yaml:"action_partition"`
	ActionParam     string      `yaml:"action_param"`
	// RewardPartition names the model partition whose state[0] is the reward
	// earned by the step just taken. Writing it as an {type: expression}
	// partition is the usual way, which keeps the reward in the expressions DSL
	// rather than inventing a second one.
	RewardPartition string `yaml:"reward_partition"`
	// ProgressPartition optionally names a model partition whose state[0] scores
	// the current position in [0,1] — a win probability, a normalised margin.
	// It is what a rollout is scored by when it hits its step limit before the
	// horizon, which is the common case once the horizon exceeds
	// rollout_max_steps. Unset falls back to the reward banked so far.
	ProgressPartition string `yaml:"progress_partition,omitempty"`
	// Discount is the per-step discount factor (default 1, undiscounted).
	Discount float64 `yaml:"discount,omitempty"`
	// ReturnRange bounds the achievable return as [min, max]. UCB1 needs a
	// bounded value scale, so returns are normalised into [0,1] against it and
	// clamped outside it — widen the range rather than leaving it clamped, since
	// a saturated score gives the search no gradient.
	ReturnRange []float64 `yaml:"return_range"`
	// ScenarioSeed pins which noise realisation Apply returns, and so which
	// scenario a pinned-noise search plans against.
	ScenarioSeed uint64 `yaml:"scenario_seed,omitempty"`
	// Parameters turns the run into posterior-predictive planning: the search
	// averages over draws of the model's uncertain parameters instead of
	// committing to a point estimate.
	Parameters *planningParametersSpec `yaml:"parameters,omitempty"`
	// Model is the forward model being planned over.
	Model planningModelSpec `yaml:"model"`
}

// planningParametersSpec supplies the posterior the search plans over. Samples
// come either inline or from a partition recorded in the data: tier — one row
// per draw — which is how a calibration's own output feeds the planner.
type planningParametersSpec struct {
	Samples [][]float64 `yaml:"samples,omitempty"`
	// SamplesFrom reads one draw per recorded row. posterior_estimation's sampler
	// partition already draws from the running posterior each step, so its
	// recorded rows are the posterior sample set — no further sampling step is
	// needed, and an SMC proposal partition works the same way.
	//
	// Set BurnIn past the rows produced while the estimate was still moving.
	// Those carry the convergence transient, not the posterior, and a planner
	// reading them plans against a spread the inference has already ruled out.
	//
	// Pointing this at a posterior MEAN partition is a mistake that runs: those
	// rows are a trajectory of point estimates, not draws.
	SamplesFrom *planningSamplesRef   `yaml:"samples_from,omitempty"`
	Targets     []planningParamTarget `yaml:"targets"`
	// Weights is how much credence each sample carries, one per sample. This is
	// what an inference tier hands a planner when its samples are NOT already
	// distributed as the posterior — SMC particles, say, whose weights are the
	// posterior. Omit when the samples were drawn from the posterior itself.
	Weights []float64 `yaml:"weights,omitempty"`
	// WeightsFrom reads those weights from a recorded partition's final row,
	// so a calibration's own output can be handed straight over.
	WeightsFrom *planningRowsRef `yaml:"weights_from,omitempty"`
	// Belief turns on in-tree belief updating, so the planner can value an action
	// for what it reveals and not only for what it pays. It costs a model step
	// per sample on every transition, so keep the sample set small — a run with
	// hundreds of draws should plan on a fixed draw instead.
	Belief *planningBeliefSpec `yaml:"belief,omitempty"`
}

// readsInputs reports that the parameters are read from inputs when the run
// starts. Recorded rows are read either all from inputs, in a {type: simulation}
// environment, which refuses rows named any other way, or all by partition
// name, in mcts_planning, which refuses input:; so weights read from an input
// need samples read from one too.
func (p *planningParametersSpec) readsInputs() (bool, error) {
	if p.WeightsFrom != nil && p.WeightsFrom.Input != "" && p.SamplesFrom == nil {
		return false, fmt.Errorf("parameters.weights_from reads input %q when the run "+
			"starts, so its samples must too: name the input in samples_from", p.WeightsFrom.Input)
	}
	return p.SamplesFrom != nil && p.SamplesFrom.Input != "", nil
}

// resolveWeights produces the initial credence over the samples, or nil for
// equal credence.
func (p *planningParametersSpec) resolveWeights(
	storage *simulator.StateTimeStorage,
	sampleCount int,
) ([]float64, error) {
	if len(p.Weights) > 0 && p.WeightsFrom != nil {
		return nil, fmt.Errorf(
			"mcts_planning parameters: set weights: or weights_from:, not both")
	}
	if len(p.Weights) > 0 {
		if len(p.Weights) != sampleCount {
			return nil, fmt.Errorf(
				"mcts_planning parameters: %d weights for %d samples — there must be one "+
					"weight per sample", len(p.Weights), sampleCount)
		}
		return p.Weights, nil
	}
	if p.WeightsFrom == nil {
		return nil, nil
	}
	if storage == nil {
		return nil, fmt.Errorf(
			"mcts_planning parameters.weights_from needs a data: block, or an earlier "+
				"macro, producing %q", p.WeightsFrom.PartitionName)
	}
	rows := storage.GetValues(p.WeightsFrom.PartitionName)
	if len(rows) == 0 {
		return nil, fmt.Errorf(
			"mcts_planning parameters.weights_from: no recorded data for partition %q",
			p.WeightsFrom.PartitionName)
	}
	// The final row: the weights an inference run finished holding.
	row := rows[len(rows)-1]
	weights := row
	if indices := p.WeightsFrom.ValueIndices; len(indices) > 0 {
		weights = make([]float64, len(indices))
		for i, index := range indices {
			if index < 0 || index >= len(row) {
				return nil, fmt.Errorf(
					"mcts_planning parameters.weights_from: value index %d outside the "+
						"%d-wide rows of %q", index, len(row), p.WeightsFrom.PartitionName)
			}
			weights[i] = row[index]
		}
	}
	if len(weights) != sampleCount {
		return nil, fmt.Errorf(
			"mcts_planning parameters.weights_from: %q supplies %d weights for %d samples "+
				"— there must be one weight per sample",
			p.WeightsFrom.PartitionName, len(weights), sampleCount)
	}
	return append([]float64(nil), weights...), nil
}

// planningRowsRef reads a recorded partition: in mcts_planning, by name from
// its inputs and earlier macros; in a {type: simulation} environment, from the
// input named, when its run starts.
type planningRowsRef struct {
	Input         string `yaml:"input,omitempty"`
	PartitionName string `yaml:"partition_name"`
	ValueIndices  []int  `yaml:"value_indices,omitempty"`
}

// planningSamplesRef reads draws from recorded rows, discarding a burn-in.
type planningSamplesRef struct {
	Input         string `yaml:"input,omitempty"`
	PartitionName string `yaml:"partition_name"`
	ValueIndices  []int  `yaml:"value_indices,omitempty"`
	// BurnIn drops this many leading rows, which is how the transient of a
	// still-converging inference is kept out of the sample set.
	BurnIn int `yaml:"burn_in,omitempty"`
}

// planningBeliefSpec names what the decision-maker observes and how sharply that
// observation discriminates between parameter samples.
type planningBeliefSpec struct {
	ObservationPartition string  `yaml:"observation_partition"`
	Variance             float64 `yaml:"variance"`
}

// planningParamTarget routes part of each sample into the model.
type planningParamTarget struct {
	Partition string `yaml:"partition"`
	Param     string `yaml:"param"`
	Indices   []int  `yaml:"indices"`
}

// resolve produces the sample set, reading it out of storage when the config
// points at a recorded partition rather than listing draws inline.
func (p *planningParametersSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([][]float64, error) {
	if len(p.Targets) == 0 {
		return nil, fmt.Errorf(
			"mcts_planning parameters needs targets: saying where each sample's values go")
	}
	if (len(p.Samples) > 0) == (p.SamplesFrom != nil) {
		return nil, fmt.Errorf(
			"mcts_planning parameters needs exactly one of samples: or samples_from: " +
				"to draw from")
	}
	if len(p.Samples) > 0 {
		return p.Samples, nil
	}
	if storage == nil {
		return nil, fmt.Errorf(
			"mcts_planning parameters.samples_from needs a data: block to read %q from",
			p.SamplesFrom.PartitionName)
	}
	rows := storage.GetValues(p.SamplesFrom.PartitionName)
	if len(rows) == 0 {
		return nil, fmt.Errorf(
			"mcts_planning parameters.samples_from: no recorded data for partition %q",
			p.SamplesFrom.PartitionName)
	}
	if p.SamplesFrom.BurnIn >= len(rows) {
		return nil, fmt.Errorf(
			"mcts_planning parameters.samples_from: burn_in %d discards all %d recorded "+
				"rows of %q", p.SamplesFrom.BurnIn, len(rows), p.SamplesFrom.PartitionName)
	}
	rows = rows[p.SamplesFrom.BurnIn:]
	indices := p.SamplesFrom.ValueIndices
	samples := make([][]float64, len(rows))
	for i, row := range rows {
		if len(indices) == 0 {
			samples[i] = append([]float64(nil), row...)
			continue
		}
		draw := make([]float64, len(indices))
		for j, index := range indices {
			if index < 0 || index >= len(row) {
				return nil, fmt.Errorf(
					"mcts_planning parameters.samples_from: value index %d outside the "+
						"%d-wide rows of %q", index, len(row), p.SamplesFrom.PartitionName)
			}
			draw[j] = row[index]
		}
		samples[i] = draw
	}
	return samples, nil
}

// planningModelSpec is the forward model: ordinary partitions, plus the timestep
// they advance on. There is no simulation block, because a planning run drives
// the model one step at a time and supplies its own termination.
type planningModelSpec struct {
	Partitions []simulator.PartitionConfig `yaml:"partitions"`
	Timestep   float64                     `yaml:"timestep,omitempty"`
	InitTime   float64                     `yaml:"init_time,omitempty"`
}

// resolve reports that this is a live macro.
func (s *mctsPlanningSpec) resolve(
	*simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	return nil, nil, fmt.Errorf(
		"mcts_planning is a live macro; it drives its own model and must not be " +
			"combined with against-storage macros")
}

func (s *mctsPlanningSpec) resolveLive(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, int, float64, error) {
	environment, err := s.environment(storage)
	if err != nil {
		return nil, 0, 0, err
	}
	timestep := s.Timestep
	if timestep == 0 {
		timestep = 1.0
	}
	return macros.NewMCTSSelfPlayPartitions(s.search(environment)), s.Steps, timestep, nil
}

// environment checks the spec and builds its decision problem; storage is what
// parameters.samples_from and weights_from read.
func (s *mctsPlanningSpec) environment(
	storage *simulator.StateTimeStorage,
) (*simulationEnvironment, error) {
	if err := s.checkSearch(); err != nil {
		return nil, err
	}
	if err := s.refuseInputRefs(); err != nil {
		return nil, err
	}
	return s.simulationEnvironmentSpec.build(storage, "mcts_planning")
}

// refuseInputRefs refuses samples or weights read from a named input, which a
// {type: simulation} environment does but mcts_planning does not.
func (s *mctsPlanningSpec) refuseInputRefs() error {
	if s.Parameters != nil && ((s.Parameters.SamplesFrom != nil && s.Parameters.SamplesFrom.Input != "") ||
		(s.Parameters.WeightsFrom != nil && s.Parameters.WeightsFrom.Input != "")) {
		return fmt.Errorf("mcts_planning reads samples_from and weights_from by partition " +
			"name, from its inputs and earlier macros; input: is for a {type: simulation} " +
			"environment, which reads them when its run starts")
	}
	return nil
}

// checkSearch checks the spec names the search and how long it runs.
func (s *mctsPlanningSpec) checkSearch() error {
	if s.Name == "" {
		return fmt.Errorf("mcts_planning needs a name: (it prefixes the partition names)")
	}
	if s.Steps <= 0 {
		return fmt.Errorf("mcts_planning needs steps: > 0 (the number of decisions to take)")
	}
	return nil
}

// search is the planner's search over its environment, the config's settings
// applied over the environment's defaults.
func (s *mctsPlanningSpec) search(
	environment *simulationEnvironment,
) macros.MCTSSelfPlaySpec[[]float64, int] {
	typed := environment.typed()

	selfPlay := macros.MCTSSelfPlaySpec[[]float64, int]{
		Env: typed.env,
		Cfg: agents.MCTSConfig[[]float64, int]{
			Rollout:  typed.rollout,
			Progress: typed.progress,
		},
		InitState:       typed.initState,
		Decoder:         typed.decoder,
		Encoder:         typed.encoder,
		MaxLegalActions: typed.maxLegalActions,
		StateWidth:      typed.stateWidth,
		Players:         typed.players,
		ChanceNodes:     !s.PinnedNoise,
		SimsPerPly:      s.SimsPerDecision,
	}
	// Looking further than the horizon cannot pay: the episode ends there.
	selfPlay.Cfg.MaxTreeDepth = typed.searchDepth
	selfPlay.Cfg.RolloutMaxSteps = typed.searchDepth
	macros.ApplyMCTSSearchSettings(&selfPlay, macros.MCTSSearchSettings{
		Name:            s.Name,
		SimsPerPly:      s.SimsPerDecision,
		Simulations:     s.SimsPerDecision,
		Exploration:     s.Exploration,
		MaxTreeDepth:    s.MaxTreeDepth,
		RolloutMaxSteps: s.RolloutMaxSteps,
		Seed:            s.Seed,
	})
	return selfPlay
}

// build validates the decision problem and builds its environment. what names
// the config element for error messages ("mcts_planning", or an mcts_*
// iteration's environment), and storage is what parameters.samples_from and
// weights_from read, when the caller has one.
func (e *simulationEnvironmentSpec) build(
	storage *simulator.StateTimeStorage,
	what string,
) (*simulationEnvironment, error) {
	if e.Horizon <= 0 {
		return nil, fmt.Errorf("%s needs horizon: > 0 (how far each search looks ahead)", what)
	}
	if len(e.Actions) == 0 {
		return nil, fmt.Errorf("%s needs a non-empty actions: list", what)
	}
	if len(e.ReturnRange) != 2 {
		return nil, fmt.Errorf(
			"%s needs return_range: [min, max] to normalise returns onto "+
				"the [0,1] scale UCB1 requires", what)
	}
	if len(e.Model.Partitions) == 0 {
		return nil, fmt.Errorf("%s needs model.partitions", what)
	}

	// The planner writes the action, and any parameter draw, into the model's
	// params, so those keys are the model partitions' names too.
	injected := map[string][]string{e.ActionPartition: {e.ActionParam}}
	if e.Parameters != nil {
		for _, target := range e.Parameters.Targets {
			injected[target.Partition] = append(injected[target.Partition], target.Param)
		}
	}
	settings, implementations, err := e.Model.build(injected, what)
	if err != nil {
		return nil, err
	}
	rewardPartition := e.RewardPartition
	if rewardPartition == "" {
		return nil, fmt.Errorf(
			"%s needs reward_partition: naming the model partition whose "+
				"state[0] is the per-step reward", what)
	}
	if err := requireModelPartition(settings, rewardPartition, "reward_partition"); err != nil {
		return nil, err
	}
	if err := requireModelPartition(settings, e.ActionPartition, "action_partition"); err != nil {
		return nil, err
	}
	var progress func(rows map[string][]float64) (float64, bool)
	if e.ProgressPartition != "" {
		if err := requireModelPartition(
			settings, e.ProgressPartition, "progress_partition"); err != nil {
			return nil, err
		}
		progressPartition := e.ProgressPartition
		progress = func(rows map[string][]float64) (float64, bool) {
			return rows[progressPartition][0], true
		}
	}

	var samples [][]float64
	var weights []float64
	var targets []agents.SimulationParamTarget
	var belief *agents.BeliefSpec
	deferred := false
	if e.Parameters != nil {
		var err error
		if deferred, err = e.Parameters.readsInputs(); err != nil {
			return nil, err
		}
		switch {
		case deferred && len(e.Parameters.Targets) == 0:
			return nil, fmt.Errorf(
				"mcts_planning parameters needs targets: saying where each sample's values go")
		case deferred && e.Parameters.Belief != nil:
			return nil, fmt.Errorf("%s parameters.belief needs its samples when the config "+
				"loads, since its weights are part of the state, but samples_from reads "+
				"input %q when the run starts; list the samples inline", what, e.Parameters.SamplesFrom.Input)
		case !deferred:
			resolved, err := e.Parameters.resolve(storage)
			if err != nil {
				return nil, err
			}
			samples = resolved
			weights, err = e.Parameters.resolveWeights(storage, len(samples))
			if err != nil {
				return nil, err
			}
		}
		for _, target := range e.Parameters.Targets {
			if err := requireModelPartition(
				settings, target.Partition, "parameters.targets"); err != nil {
				return nil, err
			}
			targets = append(targets, agents.SimulationParamTarget{
				Partition: target.Partition,
				Param:     target.Param,
				Indices:   target.Indices,
			})
		}
		if e.Parameters.Belief != nil {
			if err := requireModelPartition(settings,
				e.Parameters.Belief.ObservationPartition,
				"parameters.belief.observation_partition"); err != nil {
				return nil, err
			}
			if e.Parameters.Belief.Variance <= 0 {
				return nil, fmt.Errorf(
					"%s parameters.belief needs variance: > 0 (the observation "+
						"noise the belief update scores against)", what)
			}
			belief = &agents.BeliefSpec{
				ObservationPartition: e.Parameters.Belief.ObservationPartition,
				Variance:             e.Parameters.Belief.Variance,
			}
		}
	}

	var readLater *planningParametersSpec
	if deferred {
		readLater = e.Parameters
	}
	return &simulationEnvironment{
		SimulationEnvironment: agents.NewSimulationEnvironment(
			settings, implementations, agents.SimulationEnvironmentSpec{
				Actions:         e.Actions,
				ActionPartition: e.ActionPartition,
				ActionParam:     e.ActionParam,
				Horizon:         e.Horizon,
				Reward: func(rows map[string][]float64) float64 {
					return rows[rewardPartition][0]
				},
				Discount:           e.Discount,
				MinReturn:          e.ReturnRange[0],
				MaxReturn:          e.ReturnRange[1],
				ScenarioSeed:       e.ScenarioSeed,
				Progress:           progress,
				ParameterSamples:   samples,
				ParameterWeights:   weights,
				ParameterTargets:   targets,
				Belief:             belief,
				DeferredParameters: deferred,
			}),
		actions:  len(e.Actions),
		horizon:  e.Horizon,
		deferred: readLater,
	}, nil
}

// simulationEnvironment is a built decision problem over a forward model, with
// what the search needs to know about it.
type simulationEnvironment struct {
	*agents.SimulationEnvironment
	actions, horizon int
	// deferred are the parameters read from inputs when the run starts, if any.
	deferred *planningParametersSpec
}

// bindParameters reads the environment's deferred parameters from its inputs.
func (e *simulationEnvironment) bindParameters(
	storageOf func(name string) (*simulator.StateTimeStorage, error),
) error {
	parameters := e.deferred
	storage, err := storageOf(parameters.SamplesFrom.Input)
	if err != nil {
		return err
	}
	if _, err := inputRows(storage, parameters.SamplesFrom.Input, parameters.SamplesFrom.PartitionName); err != nil {
		return err
	}
	samples, err := parameters.resolve(storage)
	if err != nil {
		return withKind(ErrData, fmt.Errorf("api: input %q: %w", parameters.SamplesFrom.Input, err))
	}
	if parameters.WeightsFrom != nil {
		if storage, err = storageOf(parameters.WeightsFrom.Input); err != nil {
			return err
		}
		if _, err := inputRows(storage, parameters.WeightsFrom.Input, parameters.WeightsFrom.PartitionName); err != nil {
			return err
		}
	}
	weights, err := parameters.resolveWeights(storage, len(samples))
	if err == nil {
		err = e.BindParameters(samples, weights)
	}
	if err != nil {
		return withKind(ErrData, fmt.Errorf("api: binding a planning environment's parameters: %w", err))
	}
	return nil
}

// typed is the environment as the MCTS iterations take it. Its state is the
// model's rows, already []float64, so the codec is the identity — copied,
// because the partitions retain what they are given.
func (e *simulationEnvironment) typed() *typedMCTSEnvironment[[]float64, int] {
	return &typedMCTSEnvironment[[]float64, int]{
		env: e.SimulationEnvironment,
		// A rollout that runs out of steps before the horizon is scored by the
		// progress proxy rather than discarded, which is what keeps the search
		// informed when the horizon exceeds rollout_max_steps.
		rollout: agents.FromProgress(
			agents.UniformRandomRollout[[]float64, int](),
			e.Progress,
		),
		progress:  e.Progress,
		initState: e.InitialState(),
		decoder: func(row []float64) ([]float64, error) {
			return append([]float64(nil), row...), nil
		},
		encoder: func(state []float64) []float64 {
			return append([]float64(nil), state...)
		},
		maxLegalActions: e.actions,
		stateWidth:      e.StateWidth(),
		players:         1,
		// Looking further than the horizon cannot pay: the episode ends there.
		searchDepth: e.horizon + 1,
		// A forward model can sample successors, so its search may use
		// chance nodes.
		stochastic: true,
	}
}

// build resolves the model's partitions and wraps them in the minimal simulation
// config a re-entrant model needs — the planner supplies the run length, so the
// model's own termination condition is never consulted. injected maps a partition
// to the params keys the planner writes into it, which its expressions must not
// confuse with any other name.
func (m *planningModelSpec) build(injected map[string][]string, what string) (
	*simulator.Settings, *simulator.Implementations, error,
) {
	partitions := make([]simulator.PartitionConfig, len(m.Partitions))
	copy(partitions, m.Partitions)
	for index := range partitions {
		isolateWiring(&partitions[index])
	}
	if err := resolveIterations(partitions); err != nil {
		return nil, nil, fmt.Errorf("mcts_planning model: %w", err)
	}
	if err := checkExpressionNames(
		RunConfig{Partitions: partitions}, nil, injected, what+" model", nil); err != nil {
		return nil, nil, err
	}
	timestep := m.Timestep
	if timestep == 0 {
		timestep = 1.0
	}
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: timestep},
		InitTimeValue:        m.InitTime,
	})
	for index := range partitions {
		generator.SetPartition(&partitions[index])
	}
	if err := CheckForDeadlock(generator); err != nil {
		return nil, nil, fmt.Errorf("mcts_planning model: %w", err)
	}
	settings, implementations := generator.GenerateConfigs()
	// Planning steps the model one transition at a time, so the goroutine
	// round-trip of the default strategy would dominate.
	implementations.ExecutionStrategy = &simulator.InlineExecution{}
	return settings, implementations, nil
}

// requireModelPartition checks a named partition exists, so a typo is a config
// error rather than a plan silently optimising a reward of zero.
func requireModelPartition(
	settings *simulator.Settings,
	name string,
	field string,
) error {
	for _, iteration := range settings.Iterations {
		if iteration.Name == name {
			return nil
		}
	}
	available := make([]string, 0, len(settings.Iterations))
	for _, iteration := range settings.Iterations {
		available = append(available, iteration.Name)
	}
	return fmt.Errorf(
		"mcts_planning %s: no model partition named %q (model has %v)",
		field, name, available)
}
