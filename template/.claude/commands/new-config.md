Create a new stochadex YAML configuration file for running a simulation via the API code-generation path.

If $ARGUMENTS is provided, use it as a description of the desired simulation. Otherwise, ask the user what they want to simulate.

## Steps

1. Determine which iterations are needed — either built-in ones from stochadex packages or custom ones from this project. Consult the CLAUDE.md built-in iterations reference for available options and their params.

2. Create a YAML file in `cfg/` with the `ApiRunConfig` structure:

```yaml
main:
  partitions:

  - name: <unique_name>
    iteration: <varName>                    # references a variable from extra_vars
    params:
      <param_key>: [<float64 values>]       # all values are float64 arrays
    params_from_upstream:                    # optional: wire upstream output → params
      <param_name>:
        upstream: <upstream_partition_name>
        indices: [0, 1]                     # optional: select specific state indices
    params_as_partitions:                    # optional: reference partition names
      <param_name>: [<partition_name>]
    init_state_values: [<float64 values>]   # determines state_width
    state_history_depth: 1                  # rolling window size (increase if iteration reads history)
    seed: <integer>                         # RNG seed (0 if no randomness needed)
    extra_packages:
    - <go import path>
    extra_vars:
    - <varName>: "<Go expression>"          # e.g. "&continuous.WienerProcessIteration{}"

  simulation:
    output_condition: "&simulator.EveryStepOutputCondition{}"
    output_function: "&simulator.StdoutOutputFunction{}"
    termination_condition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}"
    timestep_function: "&simulator.ConstantTimestepFunction{Stepsize: 1.0}"
    init_time_value: 0.0
```

3. For each partition:
   - Choose a descriptive `name`.
   - Set `extra_packages` to the Go import path of the package containing the iteration.
   - Set `extra_vars` with a unique variable name mapping to the Go constructor expression.
   - Set `iteration` to that variable name.
   - Fill in `params` with the keys the iteration expects (see CLAUDE.md reference).
   - Set `init_state_values` to the correct width with sensible starting values.
   - Set `seed` to a non-zero random integer if the iteration uses randomness.
   - If the iteration reads from another partition's state, wire it via `params_from_upstream`.

4. Choose appropriate simulation settings:
   - Output: `StdoutOutputFunction` for debugging, `NewJsonLogOutputFunction("./path.log")` for file output.
   - Termination: set `MaxNumberOfSteps` or `MaxTimeElapsed` based on the simulation duration.
   - Timestep: `ConstantTimestepFunction` for fixed steps, `ExponentialDistributionTimestepFunction` for event-driven.

5. Add YAML comments explaining what each partition models and why params are set to their values.

## Reference configs

See `cfg/builtin_example.yaml` and `cfg/custom_example.yaml` for working examples.
