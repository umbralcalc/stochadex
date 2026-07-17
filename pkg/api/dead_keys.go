package api

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"
)

// deadKeyPattern matches yaml.v2's complaint about a key with no corresponding struct field:
// "line 8: field state_width not found in type simulator.PartitionConfig".
var deadKeyPattern = regexp.MustCompile(`field (\S+) not found in type`)

// unknownYamlKeys strictly parses data into v and reports the keys yaml could find no field
// for. Any other parse failure is ignored: this is a key check, and a genuine type error is
// the business of the real Unmarshal that follows.
//
// A key this view does not own shows up here whether it is dead or merely somebody else's,
// so a caller must intersect two views rather than trust one alone.
func unknownYamlKeys(data []byte, v any) map[string]bool {
	keys := make(map[string]bool)
	var typeError *yaml.TypeError
	if err := yaml.UnmarshalStrict(data, v); err != nil && errors.As(err, &typeError) {
		for _, message := range typeError.Errors {
			if match := deadKeyPattern.FindStringSubmatch(message); match != nil {
				keys[match[1]] = true
			}
		}
	}
	return keys
}

// validateNoDeadKeys rejects a config key that neither view of the file will ever read.
//
// yaml.v2 ignores an unknown key in silence, so a typo, or a key left behind by an older
// schema, does nothing whatsoever while looking load-bearing. This is not hypothetical: a
// state_width key sat in every config in this repo doing nothing (the width comes from
// init_state_values), and pkg/simulator's partition fixture still carried two keys naming a
// schema that no longer exists. Both read as meaningful and neither was.
//
// Plain strict parsing cannot express the rule, because the two views deliberately share one
// file and split its keys: the concrete view owns params and seed but has no iteration, the
// code-generation view owns iteration and simulation but has no params, and each rejects the
// other's keys. Neither is the whole schema. Their union is, so a key is dead only when BOTH
// reject it.
func validateNoDeadKeys(data []byte) error {
	var concrete ApiRunConfig
	var templated ApiRunConfigStrings
	unknownToConcrete := unknownYamlKeys(data, &concrete)
	unknownToTemplated := unknownYamlKeys(data, &templated)

	dead := make([]string, 0, len(unknownToConcrete))
	for key := range unknownToConcrete {
		if unknownToTemplated[key] {
			dead = append(dead, key)
		}
	}
	if len(dead) == 0 {
		return nil
	}
	sort.Strings(dead)
	return fmt.Errorf(
		"api: config sets %s, which nothing reads — check for a typo, or a key from an "+
			"older schema", strings.Join(dead, ", "))
}
