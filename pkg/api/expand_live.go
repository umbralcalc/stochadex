package api

import (
	"fmt"
	"maps"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// The macros that run their own clock as core-language config: each expands to
// ordinary partitions, and gives the run its clock (its steps of its timestep,
// from time zero).

// expandedClock is a macro's own clock: steps of timestep, from time zero.
type expandedClock struct {
	steps    int
	timestep float64
}

// ownClock is a macro's clock, its timestep 1 when the config sets none.
func ownClock(steps int, timestep float64) *expandedClock {
	if timestep == 0 {
		timestep = 1.0
	}
	return &expandedClock{steps: steps, timestep: timestep}
}

// expand states an evolution strategy as data, as
// macros.NewEvolutionStrategyOptimisationPartitions builds it: a nested run of
// the window's partitions and the discounted reward, then the sampler, the
// sorted collection, and the mean and covariance updates.
func (s *evolutionStrategySpec) expand(*expansionContext) (*expansion, error) {
	if len(s.Window.Data) > 0 {
		return nil, fmt.Errorf("evolution_strategy_optimisation's window reads data:, but " +
			"it runs its own clock with no inputs, so has no recorded rows to read")
	}
	var inner []expandedPartition
	var initState []float64
	upstreams := map[string]expandedUpstream{}
	outside := func(partition string, params map[string]simulator.NamedUpstreamConfig) {
		for param, upstream := range upstreamsAsData(params) {
			upstreams[partition+"/"+param] = upstream
		}
	}
	for _, windowed := range s.Window.Partitions {
		partition, err := partitionAsData(windowed.Partition)
		if err != nil {
			return nil, err
		}
		inner = append(inner, partition)
		initState = append(initState, partition.InitStateValues...)
		outside(partition.Name, windowed.OutsideUpstreams)
	}

	reward, err := partitionAsData(s.Reward.Partition.Partition)
	if err != nil {
		return nil, err
	}
	// The accumulator starts at the sorting sentinel, not the reward's own
	// init, so the never-sampled initial point ranks last (see the macro).
	reward.Name = "discounted_reward"
	reward.Iteration = component("discounted_cumulative",
		map[string]interface{}{"iteration": reward.Iteration})
	if reward.Params == nil {
		reward.Params = map[string][]float64{}
	}
	reward.Params["discount_factor"] = []float64{s.Reward.DiscountFactor}
	reward.InitStateValues = repeated(s.Sorting.EmptyValue, len(reward.InitStateValues))
	reward.StateHistoryDepth, reward.Seed = 1, 0
	inner = append(inner, reward)
	sortBy := len(initState)
	initState = append(initState, reward.InitStateValues...)
	outside(reward.Name, s.Reward.Partition.OutsideUpstreams)

	dimension := len(s.Sampler.Default)
	sampleIndices := make([]int, dimension)
	for i := range sampleIndices {
		sampleIndices[i] = i
	}
	simulation := s.Sampler.Name + "_simulation"
	collection := map[string][]float64{
		"weights":            s.Mean.Weights,
		"values_state_width": {float64(dimension)},
		"empty_value":        {s.Sorting.EmptyValue},
	}
	update := func(name string, learningRate float64, initState []float64,
		upstreams map[string]expandedUpstream, typeName string) expandedPartition {
		params := maps.Clone(collection)
		params["learning_rate"] = []float64{learningRate}
		return expandedPartition{Name: name, Iteration: component(typeName, nil), Params: params,
			ParamsFromUpstream: upstreams, InitStateValues: initState, StateHistoryDepth: 1}
	}
	return &expansion{
		partitions: []expandedPartition{
			{
				Name: simulation,
				Iteration: nestedRunComponent(inner, map[string]interface{}{
					"termination_condition": component("number_of_steps",
						map[string]interface{}{"max_steps": s.Window.Depth}),
					"timestep_function": component("constant",
						map[string]interface{}{"stepsize": 1.0}),
					"init_time_value": 0.0,
				}),
				Params:             map[string][]float64{"burn_in_steps": {0}},
				ParamsFromUpstream: upstreams,
				InitStateValues:    initState,
				StateHistoryDepth:  1,
			},
			{
				Name: s.Sampler.Name,
				Iteration: component("data_generation", map[string]interface{}{
					"likelihood": component("normal",
						map[string]interface{}{"allow_default_covariance_fallback": true}),
				}),
				Params: map[string][]float64{"default_covariance": s.Covariance.Default},
				ParamsFromUpstream: map[string]expandedUpstream{
					"mean":              {Upstream: s.Mean.Name},
					"covariance_matrix": {Upstream: s.Covariance.Name},
				},
				InitStateValues:   s.Sampler.Default,
				StateHistoryDepth: 1,
				Seed:              s.Seed,
			},
			{
				Name: s.Sorting.Name,
				Iteration: component("values_sorting_collection",
					map[string]interface{}{"push_and_sort": "other_partitions"}),
				Params: map[string][]float64{
					"value_indices":       indexParams(sampleIndices),
					"value_index_sort_by": {float64(sortBy)},
					"empty_value":         {s.Sorting.EmptyValue},
					"values_state_width":  {float64(dimension)},
				},
				ParamsAsPartitions: map[string][]string{
					"other_partition":         {s.Sampler.Name},
					"other_partition_sort_by": {simulation},
				},
				InitStateValues:   repeated(s.Sorting.EmptyValue, s.Sorting.CollectionSize*(dimension+1)),
				StateHistoryDepth: 2,
			},
			update(s.Mean.Name, s.Mean.LearningRate, s.Mean.Default,
				map[string]expandedUpstream{"sorted_collection": {Upstream: s.Sorting.Name}},
				"values_sorted_collection_mean"),
			update(s.Covariance.Name, s.Covariance.LearningRate, s.Covariance.Default,
				map[string]expandedUpstream{
					"sorted_collection": {Upstream: s.Sorting.Name},
					"mean":              {Upstream: s.Mean.Name},
				},
				"values_sorted_collection_covariance"),
		},
		clock: s.clock(),
	}, nil
}

// expand states self-play as data: the search macros.NewMCTSSelfPlayPartitions
// wires, over the registered environment the config names.
func (s *mctsSelfPlaySpec) expand(*expansionContext) (*expansion, error) {
	environment, err := s.environment()
	if err != nil {
		return nil, err
	}
	tree := searchFields(s.Exploration, s.MaxTreeDepth)
	// Self-play never searches chance nodes, which an mcts_tree over an
	// environment that samples successors does unless told not to.
	if environment.samplesSuccessors() {
		tree["pinned_noise"] = true
	}
	return &expansion{
		partitions: expandSearch(s.Name, componentOf(s.Env, ""), tree,
			rolloutFields(s.RolloutMaxSteps), environment.selfPlayLayout(s.settings())),
		clock: s.clock(),
	}, nil
}

// expand states a planner as data: the search macros.NewMCTSSelfPlayPartitions
// wires, over a {type: simulation} environment holding the macro's own
// decision-problem fields, as written.
func (s *mctsPlanningSpec) expand(ctx *expansionContext) (*expansion, error) {
	if err := s.checkSearch(); err != nil {
		return nil, err
	}
	if err := s.refuseInputRefs(); err != nil {
		return nil, err
	}
	stated := planningEnvironment(ctx.source)
	planner := s
	if s.readsInputsInside() {
		var err error
		if planner, err = s.readingInputs(ctx, stated); err != nil {
			return nil, err
		}
	}
	environment, err := planner.simulationEnvironmentSpec.build(nil, "mcts_planning")
	if err != nil {
		return nil, err
	}
	tree := searchFields(s.Exploration, s.MaxTreeDepth)
	if s.PinnedNoise {
		tree["pinned_noise"] = true
	}
	return &expansion{
		partitions: expandSearch(s.Name, stated, tree,
			rolloutFields(s.RolloutMaxSteps), macros.NewMCTSSelfPlayLayout(s.search(environment))),
		clock: s.clock(),
	}, nil
}

// readsInputsInside reports that the planner's parameters are read from
// recorded rows, which its environment reads when the run starts.
func (s *mctsPlanningSpec) readsInputsInside() bool {
	return s.Parameters != nil && (s.Parameters.SamplesFrom != nil || s.Parameters.WeightsFrom != nil)
}

// readingInputs states the planner's samples and weights as read from the
// inputs that hold them, and returns the planner with its samples read so,
// which is what its search's layout depends on: deferred, not their values.
func (s *mctsPlanningSpec) readingInputs(
	ctx *expansionContext,
	stated *yaml3.Node,
) (*mctsPlanningSpec, error) {
	parameters := *s.Parameters
	if parameters.SamplesFrom == nil {
		return nil, fmt.Errorf("mcts_planning's parameters.weights_from beside inline samples " +
			"does not expand: a run reads weights from recorded rows only with samples read " +
			"from them too; list the weights inline, or read the samples with samples_from")
	}
	if parameters.Belief != nil {
		return nil, fmt.Errorf("mcts_planning's parameters.belief over recorded samples does " +
			"not expand: its weights are part of the search state, so the samples must be " +
			"known when the config loads, but they are read when the run starts; list them inline")
	}
	owner := func(field, partition string) (string, error) {
		input, supplied := ctx.owners[partition]
		if !supplied {
			return "", fmt.Errorf("mcts_planning's parameters.%s reads %q, which no input or "+
				"earlier macro supplies", field, partition)
		}
		ref := mappingValue(mappingValue(stated, "parameters"), field)
		ref.Content = append(ref.Content, &yaml3.Node{Kind: yaml3.ScalarNode, Value: "input"},
			&yaml3.Node{Kind: yaml3.ScalarNode, Value: input})
		return input, nil
	}
	if parameters.SamplesFrom != nil {
		samples := *parameters.SamplesFrom
		input, err := owner("samples_from", samples.PartitionName)
		if err != nil {
			return nil, err
		}
		samples.Input, parameters.SamplesFrom = input, &samples
	}
	if parameters.WeightsFrom != nil {
		if _, err := owner("weights_from", parameters.WeightsFrom.PartitionName); err != nil {
			return nil, err
		}
	}
	planner := *s
	planner.Parameters = &parameters
	return &planner, nil
}

// plannerOnly are mcts_planning's fields that shape its search, not its
// decision problem.
var plannerOnly = map[string]bool{"type": true, "name": true, "steps": true,
	"pinned_noise": true, "sims_per_decision": true, "exploration": true,
	"max_tree_depth": true, "rollout_max_steps": true, "seed": true, "timestep": true}

// planningEnvironment is an mcts_planning macro's decision problem as a
// {type: simulation} environment: its fields as the config writes them,
// without the search's.
func planningEnvironment(macro *yaml3.Node) *yaml3.Node {
	environment := &yaml3.Node{Kind: yaml3.MappingNode, Content: []*yaml3.Node{
		{Kind: yaml3.ScalarNode, Value: "type"}, {Kind: yaml3.ScalarNode, Value: "simulation"}}}
	for i := 0; i+1 < len(macro.Content); i += 2 {
		if !plannerOnly[macro.Content[i].Value] {
			environment.Content = append(environment.Content, macro.Content[i], macro.Content[i+1])
		}
	}
	return environment
}

// searchFields are an mcts_tree's search settings, each left out when unset so
// the environment's default stands.
func searchFields(exploration float64, maxTreeDepth int) map[string]interface{} {
	fields := map[string]interface{}{}
	if exploration > 0 {
		fields["exploration"] = exploration
	}
	if maxTreeDepth > 0 {
		fields["max_tree_depth"] = maxTreeDepth
	}
	return fields
}

// rolloutFields are an mcts_rollout's settings, left out when unset.
func rolloutFields(maxSteps int) map[string]interface{} {
	fields := map[string]interface{}{}
	if maxSteps > 0 {
		fields["rollout_max_steps"] = maxSteps
	}
	return fields
}

// expandSearch states an MCTS search as data, as
// macros.NewMCTSSelfPlayPartitions wires it: <name>_apply, holding the state
// after each decision, and <name>_search, a nested run of <name>_tree and
// <name>_rollout. Each iteration states the environment in full; a run builds
// one environment for identical specs, so the three search one environment, as
// the macro's do.
func expandSearch(
	name string,
	environment *yaml3.Node,
	tree, rollout map[string]interface{},
	layout macros.MCTSSelfPlayLayout,
) []expandedPartition {
	applyName, searchName := name+"_apply", name+"_search"
	treeName, rolloutName := name+"_tree", name+"_rollout"
	apply := map[string]interface{}{"environment": environment, "best_index_slot": layout.BestIdxSlot}
	tree["environment"], rollout["environment"] = environment, environment
	inner := []expandedPartition{
		{
			Name:      treeName,
			Iteration: component("mcts_tree", tree),
			ParamsAsPartitions: map[string][]string{
				agents.MCTSTreeParamRolloutScoresPartition: {rolloutName}},
			InitStateValues:   layout.TreeInit,
			StateHistoryDepth: 1,
			Seed:              layout.Seed,
		},
		{
			Name:      rolloutName,
			Iteration: component("mcts_rollout", rollout),
			ParamsFromUpstream: map[string]expandedUpstream{
				agents.MCTSRolloutParamLeaf: {Upstream: treeName, Indices: layout.LeafIndices}},
			InitStateValues:   layout.RolloutInit,
			StateHistoryDepth: 1,
			Seed:              layout.RolloutSeed,
		},
	}
	return []expandedPartition{
		{
			Name:      applyName,
			Iteration: component("mcts_apply", apply),
			ParamsAsPartitions: map[string][]string{
				agents.ApplyParamBestIdxPartition: {searchName}},
			InitStateValues:   layout.ApplyInit,
			StateHistoryDepth: 1,
			Seed:              layout.Seed,
		},
		{
			Name: searchName,
			Iteration: nestedRunComponent(inner, map[string]interface{}{
				"termination_condition": component("number_of_steps",
					map[string]interface{}{"max_steps": layout.SimsPerPly}),
				"timestep_function": component("constant", map[string]interface{}{"stepsize": 1.0}),
				"init_time_value":   0.0,
			}),
			Params: map[string][]float64{"burn_in_steps": {0}},
			ParamsFromUpstream: map[string]expandedUpstream{
				treeName + "/" + agents.MCTSTreeParamRootState: {
					Upstream: applyName, Indices: layout.RootStateIndices}},
			InitStateValues:   layout.SearchInit,
			StateHistoryDepth: 1,
			Seed:              layout.Seed,
		},
	}
}

// readsInputsInside reports that the particles replay observed data from the
// inputs, inside their own runs.
func (s *smcInferenceSpec) readsInputsInside() bool {
	return s.Model.ObservedData != nil
}

// expand states SMC inference as data, as macros.NewSMCInferencePartitions
// builds it: the proposals, the particles (a replicated run of the model,
// each replaying the observed data from its input on that input's clock), and
// the posterior.
func (s *smcInferenceSpec) expand(ctx *expansionContext) (*expansion, error) {
	priors := make([]inference.Prior, len(s.Priors))
	priorSpecs := make([]interface{}, len(s.Priors))
	for i, spec := range s.Priors {
		prior, err := resolvePrior(spec)
		if err != nil {
			return nil, fmt.Errorf("smc_inference prior %d: %w", i, err)
		}
		priors[i], priorSpecs[i] = prior, componentOf(spec, "")
	}
	// Checks the model's own simulation, when it states one.
	if _, err := s.Model.builder(nil); err != nil {
		return nil, err
	}
	var particle []expandedPartition
	simulation := map[string]interface{}{
		"termination_condition": componentOf(s.Model.Simulation.TerminationCondition, ""),
		"timestep_function":     componentOf(s.Model.Simulation.TimestepFunction, ""),
		"output_condition":      componentOf(s.Model.Simulation.OutputCondition, ""),
		"output_function":       componentOf(s.Model.Simulation.OutputFunction, ""),
		"init_time_value":       s.Model.Simulation.InitTimeValue,
	}
	if observed := s.Model.ObservedData; observed != nil {
		ref := observed.Ref.PartitionName
		input, supplied := ctx.owners[ref]
		if !supplied {
			return nil, fmt.Errorf("smc_inference model.observed_data reads %q, which no input "+
				"supplies; the particles replay it from an input", ref)
		}
		particle = append(particle, expandedPartition{
			Name: observed.Name,
			Iteration: component("from_input",
				map[string]interface{}{"input": input, "partition": ref}),
			InitStateValues:   ctx.firstRows[ref],
			StateHistoryDepth: 2,
		})
		simulation = map[string]interface{}{
			"termination_condition": component("input_exhausted", map[string]interface{}{"input": input}),
			"timestep_function":     component("from_input", map[string]interface{}{"input": input}),
			"init_time_value":       ctx.initTime,
		}
	}
	for _, model := range s.Model.Partitions {
		partition, err := partitionAsData(model)
		if err != nil {
			return nil, err
		}
		particle = append(particle, partition)
	}

	particles, dimension := s.NumParticles, len(s.Priors)
	verbose := 0.0
	if s.Verbose {
		verbose = 1.0
	}
	priorTypes, priorParams := inference.EncodePriors(priors)
	posteriorInit := make([]float64, inference.PosteriorStateWidth(dimension))
	for j := range dimension {
		posteriorInit[dimension+j*dimension+j] = 1.0 // identity covariance
	}
	replicated := map[string]interface{}{
		"replicas":           particles,
		"params_per_replica": dimension,
		"output_partition":   s.Model.LoglikePartition,
		"run":                map[string]interface{}{"partitions": particle, "simulation": simulation},
	}
	if len(s.Model.ParamForwarding) > 0 {
		replicated["param_forwarding"] = s.Model.ParamForwarding
	}
	return &expansion{
		partitions: []expandedPartition{
			{
				Name:      s.ProposalName,
				Iteration: component("smc_proposal", map[string]interface{}{"priors": priorSpecs}),
				Params: map[string][]float64{
					"verbose":       {verbose},
					"num_particles": {float64(particles)},
					"prior_types":   priorTypes,
					"prior_params":  priorParams,
				},
				ParamsAsPartitions: map[string][]string{"posterior_partition": {s.PosteriorName}},
				InitStateValues:    make([]float64, particles*dimension),
				StateHistoryDepth:  2,
				Seed:               s.Seed,
			},
			{
				Name:               s.SimName,
				Iteration:          component("replicated_simulation_run", replicated),
				Params:             map[string][]float64{"replica_params": make([]float64, particles*dimension)},
				ParamsFromUpstream: map[string]expandedUpstream{"replica_params": {Upstream: s.ProposalName}},
				InitStateValues:    make([]float64, particles),
				StateHistoryDepth:  2,
				Seed:               s.Seed + 100,
			},
			{
				Name:      s.PosteriorName,
				Iteration: component("smc_posterior", map[string]interface{}{"param_names": s.ParamNames}),
				Params: map[string][]float64{
					"verbose":           {verbose},
					"num_particles":     {float64(particles)},
					"num_params":        {float64(dimension)},
					"particle_loglikes": make([]float64, particles),
					"particle_params":   make([]float64, particles*dimension),
				},
				ParamsFromUpstream: map[string]expandedUpstream{
					"particle_loglikes": {Upstream: s.SimName},
					"particle_params":   {Upstream: s.ProposalName},
				},
				InitStateValues:   posteriorInit,
				StateHistoryDepth: 2,
			},
		},
		clock: s.clock(),
	}, nil
}
