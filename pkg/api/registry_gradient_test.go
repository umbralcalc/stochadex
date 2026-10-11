package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/inference"
	"gonum.org/v1/gonum/floats"
)

// driverRows is the series the gradient's batch is drawn from.
var driverRows = [][]float64{{1, 10}, {2, 14}, {4, 9}, {7, 3}, {3, 6}, {5, 8}, {6, 2}}

// gradientYAML hosts a data_comparison_gradient in an embedded run, its batch
// fed from the driver partition's history of depth 3, with a fixed mean and a
// diagonal covariance. hostExtra and innerExtra are spliced into the host's and
// the gradient's specs.
func gradientYAML(hostExtra, innerExtra string) string {
	rows := make([]string, len(driverRows))
	for i, row := range driverRows {
		rows[i] = fmt.Sprintf("[%v, %v]", row[0], row[1])
	}
	return `inputs:
  driver: {source: {inline: {times: [0, 1, 2, 3, 4, 5, 6], partitions: {driver: [` + strings.Join(rows, ", ") + `]}}}}
main:
  partitions:
  - {name: driver, iteration: {type: from_input, input: driver}, state_history_depth: 3, seed: 0}
  - name: fit
    params: {burn_in_steps: [0]}
` + hostExtra + `    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 6}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: fit
  partitions:
  - name: gradient
    iteration: {type: data_comparison_gradient, likelihood: {type: normal}, gradient_function: mean_gradient}
    params:
      mean: [3.0, 5.0]
      covariance_matrix: [2.0, 0.0, 0.0, 4.0]
` + innerExtra + `    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 1}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
}

const (
	gradientFeed = "    params_as_partitions: {gradient/update_from_partition_history: [driver]}\n"
	gradientUse  = "      driver->data_values: []\n"
)

func TestDataComparisonGradient(t *testing.T) {
	t.Run("averages the normal log-likelihood's mean gradient over the fed batch", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, gradientYAML(gradientFeed, gradientUse)))
		if err != nil {
			t.Fatal(err)
		}
		rows := result.Storage.GetValues("fit")
		mean, variance := []float64{3, 5}, []float64{2, 4}
		// From step 3 the batch is the driver's last three rows as of the
		// previous step (a history read is lag-1), so it is all real data.
		for step := 3; step < len(rows); step++ {
			want := make([]float64, 2)
			for _, row := range driverRows[step-3 : step] {
				for i := range want {
					want[i] += (row[i] - mean[i]) / variance[i] / 3
				}
			}
			if !floats.EqualApprox(rows[step], want, 1e-12) {
				t.Errorf("step %d: gradient %v, want %v", step, rows[step], want)
			}
		}
	})

	t.Run("its spike twin reproduces likelihood_mean_function_fit exactly", func(t *testing.T) {
		macro, err := RunToStorage(LoadApiRunConfigFromYaml("testdata/gradient/fit_macro.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		twin, err := RunToStorage(LoadApiRunConfigFromYaml("testdata/gradient/fit_twin.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
		fit := macro.Storage.GetValues("mean_fit")
		if floats.Equal(fit[20], fit[len(fit)-1]) {
			t.Error("the fit never moved, so the comparison proves little")
		}
	})

	t.Run("likelihood_mean_function_fit with an unknown likelihood fails before running", func(t *testing.T) {
		yaml := strings.Replace(readFile(t, "testdata/gradient/fit_macro.yaml"),
			"    likelihood: {type: normal}\n", "    likelihood: {type: lognormal}\n", 1)
		_, err := RunToStorage(writeConfig(t, yaml))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), `"lognormal"`) {
			t.Errorf("expected a config error naming the likelihood, got %v", err)
		}
	})

	cases := []struct{ name, yaml, want string }{
		{"in the main run, where nothing feeds it",
			strings.Replace(gradientYAML(gradientFeed, gradientUse),
				"  - {name: driver,", "  - {name: loose, iteration: {type: data_comparison_gradient, likelihood: {type: normal}, "+
					"gradient_function: mean_gradient}, params: {}, init_state_values: [0.0], state_history_depth: 1, seed: 0}\n  - {name: driver,", 1),
			`main partition "loose" (data_comparison_gradient) reads a history its host feeds it, so it only runs inside an embedded run`},
		{"unfed by its host, after an embedded run no partition hosts",
			strings.Replace(gradientYAML("", gradientUse), "embedded:\n", "embedded:\n- name: spare\n"+
				"  partitions: [{name: idle, iteration: {type: constant_values}, params: {}, init_state_values: [0.0], "+
				"state_history_depth: 1, seed: 0}]\n  simulation: {termination_condition: {type: number_of_steps, max_steps: 1}, "+
				"timestep_function: {type: constant, stepsize: 1.0}, init_time_value: 0.0}\n", 1),
			`embedded run "fit" partition "gradient" (data_comparison_gradient) reads a history its host feeds it, ` +
				`but host "fit" does not; add params_as_partitions: {gradient/update_from_partition_history: [<partition>]}`},
		{"fed a partition it does not declare", gradientYAML(gradientFeed, ""),
			`embedded run "fit" partition "gradient" is fed "driver", so it declares "driver->data_values" in its params`},
		{"with an unknown gradient function", strings.Replace(gradientYAML(gradientFeed, gradientUse),
			"gradient_function: mean_gradient", "gradient_function: slope", 1), `unknown`},
		{"with an unknown likelihood", strings.Replace(gradientYAML(gradientFeed, gradientUse),
			"likelihood: {type: normal}", "likelihood: {type: lognormal}", 1),
			`data_comparison_gradient: field "likelihood": `},
		{"without a likelihood", strings.Replace(gradientYAML(gradientFeed, gradientUse),
			"likelihood: {type: normal}, ", "", 1), `data_comparison_gradient: missing required field "likelihood"`},
		{"inside a replicated run", replicatedYAML(strings.Replace(replicatedSpec, "iteration: {type: param_values}",
			"iteration: {type: from_history}", 1)),
			`partition "echo" reads a history an embedded run's host feeds it, which no replica has`},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}

// gradientless is a likelihood with no mean gradient, which no registered
// likelihood is.
type gradientless struct {
	inference.LikelihoodDistribution
}

func TestGradientLikelihood(t *testing.T) {
	if _, err := gradientLikelihood(gradientless{}); err == nil ||
		!strings.Contains(err.Error(), "likelihood api.gradientless does not support gradients") {
		t.Errorf("got %v", err)
	}
	normal := &inference.NormalLikelihoodDistribution{}
	if got, err := gradientLikelihood(normal); err != nil || got != inference.LikelihoodDistributionWithGradient(normal) {
		t.Errorf("a normal likelihood should pass through, got %v, %v", got, err)
	}
}
