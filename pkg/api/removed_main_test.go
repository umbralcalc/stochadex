package api

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// underMain rewrites a config in the removed form: its partitions, expressions
// and simulation blocks moved under a main: block, as older configs were written.
func underMain(contents string) string {
	var top, main []string
	inMain := false
	for _, line := range strings.Split(contents, "\n") {
		if key := regexp.MustCompile(`^[a-z_]+:`).FindString(line); key != "" {
			inMain = key == "partitions:" || key == "expressions:" || key == "simulation:"
		}
		if inMain {
			if line != "" {
				line = "  " + line
			}
			main = append(main, line)
		} else {
			top = append(top, line)
		}
	}
	return strings.Join(top, "\n") + "\nmain:\n" + strings.Join(main, "\n") + "\n"
}

// TestMainBlockIsRefused checks every shipped example with a main run fails to
// load when written in the removed main: form, with an error saying where its
// contents go, and loads as shipped.
func TestMainBlockIsRefused(t *testing.T) {
	root := repoRootPath(t)
	t.Chdir(root)
	paths, _ := filepath.Glob("cfg/example_*.yaml")
	checked := 0
	for _, path := range paths {
		shipped := readFile(t, path)
		if !strings.Contains("\n"+shipped, "\npartitions:") {
			continue // a macros-only config has no main run
		}
		checked++
		t.Run(path, func(t *testing.T) {
			if _, err := LoadConfig(path); err != nil {
				t.Fatalf("the shipped config does not load: %v", err)
			}
			legacy := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, legacy, underMain(shipped))
			_, err := LoadConfig(legacy)
			if KindOf(err) != ErrConfig || !strings.HasSuffix(err.Error(), "api: main: was removed: "+
				"write its partitions, expressions and simulation at the top level of the config, "+
				"one level less indented, and drop main. from --set paths (partitions[name=w].seed=7)") {
				t.Errorf("the main: form should be refused saying where its contents go, got %v", err)
			}
		})
	}
	if checked < 8 {
		t.Fatalf("checked only %d examples with a main run", checked)
	}

	t.Run("invalid YAML reports its own error, not a removed main:", func(t *testing.T) {
		message := loadError(t, "main: [\n")
		if strings.Contains(message, "removed") {
			t.Errorf("got %q", message)
		}
	})

	t.Run("a --set path through main is refused, naming it", func(t *testing.T) {
		_, err := LoadConfig("cfg/example_config.yaml", WithSet("main.partitions[name=first_wiener_process].seed", "7"))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "the config has no main") {
			t.Errorf("got %v", err)
		}
		config, err := LoadConfig("cfg/example_config.yaml", WithSet("partitions[name=first_wiener_process].seed", "7"))
		if err != nil || config.Main.Partitions[0].Seed != 7 {
			t.Errorf("the top-level path should set the seed, got %v", err)
		}
	})
}

// TestReleaseSmokeConfigLoads runs the config the release workflow writes inline
// to smoke-test each built binary, and checks it prints what that step greps for.
func TestReleaseSmokeConfigLoads(t *testing.T) {
	workflow := readFile(t, filepath.Join(repoRootPath(t), ".github", "workflows", "release.yml"))
	block := regexp.MustCompile(`(?s)cat > /tmp/smoke\.yaml <<'EOF'\n(.*?)\n\s*EOF\n`).FindStringSubmatch(workflow)
	if block == nil {
		t.Fatal("the release workflow's smoke config was not found")
	}
	lines := strings.Split(block[1], "\n")
	indent := len(lines[0]) - len(strings.TrimLeft(lines[0], " "))
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, strings.Repeat(" ", indent))
	}
	path := writeConfigPath(t, strings.Join(lines, "\n")+"\n")
	var err error
	printed := captureStdout(t, func() { err = Execute([]string{"stochadex", "--config", path}) })
	if err != nil {
		t.Fatalf("the smoke config does not run: %v", err)
	}
	if !strings.Contains(printed, "3 walk") || !strings.Contains(workflow, `grep -q "3 walk"`) {
		t.Errorf("the smoke test greps for a row the config does not print:\n%s", printed)
	}
}
