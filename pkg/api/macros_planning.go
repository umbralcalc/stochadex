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
	// Model is the forward model being planned over.
	Model planningModelSpec `yaml:"model"`
	// Search hyperparameters; unset values fall back to the agents defaults.
	SimsPerDecision int     `yaml:"sims_per_decision,omitempty"`
	Exploration     float64 `yaml:"exploration,omitempty"`
	MaxTreeDepth    int     `yaml:"max_tree_depth,omitempty"`
	RolloutMaxSteps int     `yaml:"rollout_max_steps,omitempty"`
	Seed            uint64  `yaml:"seed,omitempty"`
	Timestep        float64 `yaml:"timestep,omitempty"`
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
	*simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, int, float64, error) {
	if s.Name == "" {
		return nil, 0, 0, fmt.Errorf("mcts_planning needs a name: (it prefixes the partition names)")
	}
	if s.Steps <= 0 {
		return nil, 0, 0, fmt.Errorf("mcts_planning needs steps: > 0 (the number of decisions to take)")
	}
	if s.Horizon <= 0 {
		return nil, 0, 0, fmt.Errorf("mcts_planning needs horizon: > 0 (how far each search looks ahead)")
	}
	if len(s.Actions) == 0 {
		return nil, 0, 0, fmt.Errorf("mcts_planning needs a non-empty actions: list")
	}
	if len(s.ReturnRange) != 2 {
		return nil, 0, 0, fmt.Errorf(
			"mcts_planning needs return_range: [min, max] to normalise returns onto " +
				"the [0,1] scale UCB1 requires")
	}
	if len(s.Model.Partitions) == 0 {
		return nil, 0, 0, fmt.Errorf("mcts_planning needs model.partitions")
	}

	settings, implementations, err := s.Model.build()
	if err != nil {
		return nil, 0, 0, err
	}
	rewardPartition := s.RewardPartition
	if rewardPartition == "" {
		return nil, 0, 0, fmt.Errorf(
			"mcts_planning needs reward_partition: naming the model partition whose " +
				"state[0] is the per-step reward")
	}
	if err := requireModelPartition(settings, rewardPartition, "reward_partition"); err != nil {
		return nil, 0, 0, err
	}
	if err := requireModelPartition(settings, s.ActionPartition, "action_partition"); err != nil {
		return nil, 0, 0, err
	}

	environment := agents.NewSimulationEnvironment(
		settings, implementations, agents.SimulationEnvironmentSpec{
			Actions:         s.Actions,
			ActionPartition: s.ActionPartition,
			ActionParam:     s.ActionParam,
			Horizon:         s.Horizon,
			Reward: func(rows map[string][]float64) float64 {
				return rows[rewardPartition][0]
			},
			Discount:     s.Discount,
			MinReturn:    s.ReturnRange[0],
			MaxReturn:    s.ReturnRange[1],
			ScenarioSeed: s.ScenarioSeed,
		})

	// The encoded state is already []float64, so the self-play topology's codec
	// is the identity — copied, because the partitions retain what they are given.
	selfPlay := macros.MCTSSelfPlaySpec[[]float64, int]{
		Env: environment,
		Cfg: agents.MCTSConfig[[]float64, int]{
			Rollout: agents.UniformRandomRollout[[]float64, int](),
		},
		InitState: environment.InitialState(),
		Decoder: func(row []float64) ([]float64, error) {
			return append([]float64(nil), row...), nil
		},
		Encoder: func(state []float64) []float64 {
			return append([]float64(nil), state...)
		},
		MaxLegalActions: len(s.Actions),
		StateWidth:      environment.StateWidth(),
		Players:         1,
		ChanceNodes:     !s.PinnedNoise,
		// Looking further than the horizon cannot pay: the episode ends there.
		SimsPerPly: s.SimsPerDecision,
	}
	if selfPlay.Cfg.MaxTreeDepth == 0 {
		selfPlay.Cfg.MaxTreeDepth = s.Horizon + 1
	}
	if selfPlay.Cfg.RolloutMaxSteps == 0 {
		selfPlay.Cfg.RolloutMaxSteps = s.Horizon + 1
	}
	macros.ApplyMCTSSearchSettings(&selfPlay, macros.MCTSSearchSettings{
		Name:            s.Name,
		SimsPerPly:      s.SimsPerDecision,
		Simulations:     s.SimsPerDecision,
		Exploration:     s.Exploration,
		MaxTreeDepth:    s.MaxTreeDepth,
		RolloutMaxSteps: s.RolloutMaxSteps,
		Seed:            s.Seed,
	})

	timestep := s.Timestep
	if timestep == 0 {
		timestep = 1.0
	}
	return macros.NewMCTSSelfPlayPartitions(selfPlay), s.Steps, timestep, nil
}

// build resolves the model's partitions and wraps them in the minimal simulation
// config a re-entrant model needs — the planner supplies the run length, so the
// model's own termination condition is never consulted.
func (m *planningModelSpec) build() (
	*simulator.Settings, *simulator.Implementations, error,
) {
	partitions := make([]simulator.PartitionConfig, len(m.Partitions))
	copy(partitions, m.Partitions)
	if err := resolveIterations(partitions); err != nil {
		return nil, nil, fmt.Errorf("mcts_planning model: %w", err)
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
