Create a new stochadex Iteration implementation.

If $ARGUMENTS is provided, use it as the description of what the iteration should model. Otherwise, ask the user what process or behaviour the iteration should model.

## Steps

1. Determine the iteration name and package location. Use snake_case for the file name and PascalCase with an `Iteration` suffix for the struct (e.g., `my_process.go` → `MyProcessIteration`). Place it in the appropriate `pkg/` subdirectory — create one if needed.

2. Create the Go source file with:
   - A struct implementing `simulator.Iteration` (import `github.com/umbralcalc/stochadex/pkg/simulator`)
   - A `Configure(partitionIndex int, settings *simulator.Settings)` method that reads any fixed config from `settings.Iterations[partitionIndex].Params.Map` and stores it on the struct. If randomness is needed, seed an RNG here.
   - An `Iterate(params *simulator.Params, partitionIndex int, stateHistories []*simulator.StateHistory, timestepsHistory *simulator.CumulativeTimestepsHistory) []float64` method that computes and returns the next state.
   - The `Iterate` method must NOT mutate `params`.
   - All mutable struct fields must be fully re-initializable via `Configure` (no statefulness residues between runs).

3. Create a colocated settings YAML file (`<name>_settings.yaml`) with the `iterations` array defining all partitions needed for the test. Use this schema:
   ```yaml
   iterations:
   - name: partition_name
     params:
       key: [1.0, 2.0]
     init_state_values: [0.0]
     seed: 1234
     state_width: 1
     state_history_depth: 2
   init_time_value: 0.0
   timesteps_history_depth: 2
   ```

4. Create a matching test file (`<name>_test.go`) with two subtests:
   - One that creates iterations, configures them, builds `Implementations`, creates a `PartitionCoordinator`, and calls `coordinator.Run()`.
   - One that passes the same `Implementations` to `simulator.RunWithHarnesses(settings, implementations)` and fails if it returns an error.
   - Load settings via `simulator.LoadSettingsFromYaml("./<name>_settings.yaml")`.

5. Run `go build ./...` and `go test -count=1 ./<package>/...` to verify everything works. If tests fail, diagnose and fix.

## Reference pattern

See `pkg/custom/moving_average.go`, `pkg/custom/moving_average_test.go`, and `pkg/custom/moving_average_settings.yaml` for the exact pattern to follow.
