package api

import (
	"os"
	"testing"
)

// TestSkillRecipesMatchExamples binds the stochadex-model skill's bundled recipe
// files to their validated cfg/ twins. The skill ships copies so it is portable
// (installable next to an agent with no repo access), but a copy can silently
// drift from the config the engine tests actually exercise. Asserting byte
// identity makes the cfg convergence tests transitively validate the shipped
// recipes: every recipe here is the exact config that TestExampleConfigsRun runs
// and that a convergence test (evolution-strategy, SMC, posterior) pins to a known
// optimum. Update the cfg twin, re-run, then re-copy — never edit a recipe alone.
func TestSkillRecipesMatchExamples(t *testing.T) {
	pairs := map[string]string{
		"../../.claude/skills/stochadex-model/recipes/evolution_strategy_optimisation.yaml": "../../cfg/example_evolution_strategy_config.yaml",
		"../../.claude/skills/stochadex-model/recipes/smc_inference.yaml":                    "../../cfg/example_smc_config.yaml",
		"../../.claude/skills/stochadex-model/recipes/posterior_estimation.yaml":             "../../cfg/example_posterior_macro_config.yaml",
	}
	for recipe, twin := range pairs {
		t.Run(recipe, func(t *testing.T) {
			recipeBytes, err := os.ReadFile(recipe)
			if err != nil {
				t.Fatalf("reading skill recipe: %v", err)
			}
			twinBytes, err := os.ReadFile(twin)
			if err != nil {
				t.Fatalf("reading cfg twin: %v", err)
			}
			if string(recipeBytes) != string(twinBytes) {
				t.Errorf("skill recipe %s has drifted from its validated twin %s — "+
					"re-copy the cfg file so the shipped recipe stays the tested one",
					recipe, twin)
			}
		})
	}
}
