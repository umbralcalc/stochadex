package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docsBlock is a fenced code block in a docs page.
var docsBlock = regexp.MustCompile("(?s)```([a-z]*)\n(.*?)```")

// configName is the first line of a docs block that is a whole config file.
var configName = regexp.MustCompile(`^# ([a-z_]+\.yaml)\n`)

// TestDocsExamples runs the configs the docs pages show. A yaml block that
// starts with its file name (# step.yaml) is a whole config: it must pass
// --check, and when a text block follows it, running it must print exactly
// that text, with a failed run's error line last as the CLI prints it.
func TestDocsExamples(t *testing.T) {
	examples, checked := 0, 0
	for _, page := range []string{"reference.md", "quickstart.md"} {
		text, err := os.ReadFile(filepath.Join(repoDir, "docs", page))
		if err != nil {
			t.Fatal(err)
		}
		blocks := docsBlock.FindAllStringSubmatchIndex(string(text), -1)
		for index, block := range blocks {
			language, body := string(text[block[2]:block[3]]), string(text[block[4]:block[5]])
			name := configName.FindStringSubmatch(body)
			if language != "yaml" || name == nil {
				continue
			}
			var want string
			hasOutput := false
			if index+1 < len(blocks) {
				next := blocks[index+1]
				between := strings.TrimSpace(string(text[block[1]:next[0]]))
				if between == "" && string(text[next[2]:next[3]]) == "text" {
					want, hasOutput = string(text[next[4]:next[5]]), true
				}
			}
			t.Run(page+"/"+name[1], func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, name[1])
				writeFile(t, path, body)
				if err, _ := executeIn(t, dir, "--config", path, "--check"); err != nil &&
					!(hasOutput && strings.HasPrefix(want, "stochadex: ")) {
					t.Fatalf("--check: %v", err)
				}
				if !hasOutput {
					return
				}
				err, printed := executeIn(t, dir, "--config", path)
				if err != nil {
					printed += fmt.Sprintf("stochadex: %v\n", err)
				}
				if printed != want {
					t.Errorf("printed\n%s\nthe page shows\n%s", printed, want)
				}
			})
			checked++
			if hasOutput {
				examples++
			}
		}
	}
	// The pages show their examples; finding none means this test stopped reading them.
	if examples < 6 || checked < 7 {
		t.Errorf("ran %d examples and checked %d configs, want the pages' every one", examples, checked)
	}
}
