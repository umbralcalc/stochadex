package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	yaml3 "gopkg.in/yaml.v3"
)

// Per-invocation overrides (PLAN.md O.1): one config file runs as many
// parameterised jobs, varied from outside the file.
//
//   - A --set path=value (WithSet) replaces the value at a path in the config
//     tree. Mapping keys are joined with dots, and a list entry is selected by
//     its name, never its position: main.partitions[name=w].seed. The path must
//     already exist, and the new value must have the old one's shape (a number
//     for a number, a list for a list), so a typo is an error naming the path
//     rather than a run that silently measures the unmodified config.
//   - A ${VAR} placeholder in a value is filled from the environment at load.
//     In an unquoted value the substituted text reads as YAML, exactly as if it
//     had been written there, so a placeholder can stand for a number, a list
//     or a {type: ...} name; in a quoted value it is text. An unset or empty
//     variable is an error, a placeholder in a key is an error, and $${ writes
//     a literal ${.
//
// Both are applied to the document before the dead-key check and decoding, so
// a bad override fails the load like any other config error. The resolved
// document is what the config keeps: ensemble members and served connections
// are rebuilt from it, never from the file again.
//
// The document is edited as yaml.v3 nodes, which keep each scalar's original
// text, and then decoded by yaml.v2 as usual. A plain yaml.v2 round trip would
// not do: it reads a key like n or y as a YAML 1.1 boolean and writes it back
// as false or true. A config with no placeholders and no --set is never
// re-encoded at all.

// LoadOption configures LoadConfig.
type LoadOption func(*loadOptions)

type loadOptions struct {
	sets   []setOverride
	lookup func(string) (string, bool)
}

type setOverride struct {
	path, value string
	// label names the override in errors when it was not written as a --set.
	label string
}

func (s setOverride) String() string {
	if s.label != "" {
		return s.label
	}
	return "--set " + s.path + "=" + s.value
}

// WithSet replaces the value at path with value, read as YAML. path joins
// mapping keys with dots and selects a list entry by name, as in
// main.partitions[name=w].params.rate; it must already exist in the config.
func WithSet(path, value string) LoadOption {
	return func(o *loadOptions) { o.sets = append(o.sets, setOverride{path: path, value: value}) }
}

// WithEnv fills ${VAR} placeholders from lookup instead of the process
// environment; lookup reports whether the variable is set, as os.LookupEnv.
func WithEnv(lookup func(string) (string, bool)) LoadOption {
	return func(o *loadOptions) { o.lookup = lookup }
}

// setOptions reads the CLI's --set arguments; a malformed one is an ErrUsage.
func setOptions(args []string) ([]LoadOption, error) {
	options := make([]LoadOption, 0, len(args))
	for _, arg := range args {
		option, err := parseSetArg(arg)
		if err != nil {
			return nil, &Error{Kind: ErrUsage, Err: err}
		}
		options = append(options, option)
	}
	return options, nil
}

// seedRangeOption reads --seed-range FROM:TO as an override of run.seeds with
// the seeds FROM to TO inclusive, so it is checked, and fingerprinted, exactly as
// a --set of the same list would be.
func seedRangeOption(spec string) (LoadOption, error) {
	// Without a colon, TO is empty and does not parse.
	from, to, _ := strings.Cut(spec, ":")
	first, err1 := strconv.ParseUint(from, 10, 64)
	last, err2 := strconv.ParseUint(to, 10, 64)
	if err1 != nil || err2 != nil || first > last {
		return nil, &Error{Kind: ErrUsage, Err: fmt.Errorf(
			"--seed-range %s: expected FROM:TO, two seeds with FROM no greater than TO", spec)}
	}
	var seeds strings.Builder
	seeds.WriteByte('[')
	for seed := first; ; seed++ {
		seeds.WriteString(strconv.FormatUint(seed, 10))
		if seed == last {
			break
		}
		seeds.WriteString(", ")
	}
	seeds.WriteByte(']')
	return func(o *loadOptions) {
		o.sets = append(o.sets, setOverride{path: "run.seeds", value: seeds.String(),
			label: "--seed-range " + spec})
	}, nil
}

// warnSharedMemberNames warns when a shard's outputs name members by {member}
// alone: every --seed-range shard numbers its members from 0, so shards writing
// to the same place would overwrite each other. {seed} is unique across shards.
// It is a warning, not an error: shards may be given separate places by --set.
func warnSharedMemberNames(config *ApiRunConfig, w io.Writer) {
	for _, view := range config.Outputs {
		if hasPlaceholder(view.Function.Fields, []string{"{member}"}) &&
			!hasPlaceholder(view.Function.Fields, []string{"{seed}"}) {
			fmt.Fprintf(w, "stochadex: warning: %s names members by {member}, which every "+
				"--seed-range shard numbers from 0; put {seed} in it, or give each shard its "+
				"own place, so shards do not overwrite each other\n", view.label())
		}
	}
}

// parseSetArg splits a --set argument at its first '=' outside a [name=...]
// selector.
func parseSetArg(arg string) (LoadOption, error) {
	depth := 0
	for i, r := range arg {
		switch {
		case r == '[':
			depth++
		case r == ']':
			depth--
		case r == '=' && depth == 0 && i > 0:
			return WithSet(arg[:i], arg[i+1:]), nil
		}
	}
	return nil, fmt.Errorf("--set %s: expected path=value", arg)
}

var (
	// placeholderPattern matches an escaped $${ or a ${...} placeholder on one line.
	placeholderPattern = regexp.MustCompile(`\$\$\{|\$\{([^}\n]*)\}`)
	variableName       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// tokenPattern matches what tokenize puts in place of each placeholder: a
	// plain word, which YAML reads as text anywhere, inside {...} and [...] too.
	tokenPattern = regexp.MustCompile(`__stochadex_(placeholder|literal)_([0-9]+)__`)
)

// placeholders is a document whose ${VAR} placeholders have been swapped for
// tokens, which leaves every line where it was.
type placeholders struct {
	tokenized []byte
	// names[i] is the variable token i stands for; "" for an escaped $${.
	names []string
	// variables are the distinct names, in the order they first appear.
	variables []string
}

func tokenize(data []byte) (*placeholders, error) {
	p := &placeholders{}
	seen := map[string]bool{}
	var failure error
	p.tokenized = placeholderPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		index := len(p.names)
		if string(match) == "$${" {
			p.names = append(p.names, "")
			return []byte(fmt.Sprintf("__stochadex_literal_%d__", index))
		}
		name := string(match[2 : len(match)-1])
		if !variableName.MatchString(name) && failure == nil {
			failure = fmt.Errorf("line %d: %s is not a placeholder: a variable name "+
				"is letters, digits and underscores", lineOf(data, match), match)
		}
		p.names = append(p.names, name)
		if !seen[name] {
			seen[name] = true
			p.variables = append(p.variables, name)
		}
		return []byte(fmt.Sprintf("__stochadex_placeholder_%d__", index))
	})
	if failure != nil {
		return nil, failure
	}
	if at := bytes.Index(p.tokenized, []byte("${")); at >= 0 {
		return nil, fmt.Errorf("line %d: unclosed ${ placeholder",
			bytes.Count(p.tokenized[:at], []byte("\n"))+1)
	}
	return p, nil
}

// lineOf reports the line of the first occurrence of match in data.
func lineOf(data, match []byte) int {
	return bytes.Count(data[:bytes.Index(data, match)], []byte("\n")) + 1
}

// resolution is a document with its overrides applied.
type resolution struct {
	source []byte
	// blame, given the error loading source failed with, says which override is
	// at fault (see blameSet). Nil when there were no overrides.
	blame func(failure error, load func([]byte) error) error
	// sets are the --set overrides applied, as written; variables the ${VAR}
	// names filled, in the order they were filled.
	sets      []string
	variables []string
}

// resolveSource applies the options' sets and the document's placeholders. A
// document with neither is returned as is.
func resolveSource(data []byte, options []LoadOption) (resolution, error) {
	o := loadOptions{lookup: os.LookupEnv}
	for _, option := range options {
		option(&o)
	}
	if len(o.sets) == 0 && !bytes.Contains(data, []byte("${")) {
		return resolution{source: data}, nil
	}
	p, err := tokenize(data)
	if err != nil {
		return resolution{}, err
	}
	r := resolution{}
	recording := o
	seen := map[string]bool{}
	recording.lookup = func(name string) (string, bool) {
		if !seen[name] {
			seen[name] = true
			r.variables = append(r.variables, name)
		}
		return o.lookup(name)
	}
	if r.source, err = render(p, recording, len(o.sets)); err != nil {
		return resolution{}, err
	}
	for _, set := range o.sets {
		r.sets = append(r.sets, set.String())
	}
	r.blame = func(failure error, load func([]byte) error) error {
		return blameSet(data, p, o, failure, load)
	}
	return r, nil
}

// lineNumbers matches yaml.v2's line prefix, which counts lines of the
// re-encoded document rather than of the file.
var lineNumbers = regexp.MustCompile(`line [0-9]+: `)

// blameSet names the --set behind a document that fails to load: the first
// whose applying makes load fail. When none is at fault the file is: its own
// error is returned, from the file as written when it has no placeholders, and
// otherwise with the variables' values, since a variable has no unset value to
// try the file without.
func blameSet(
	data []byte,
	p *placeholders,
	o loadOptions,
	failure error,
	load func([]byte) error,
) error {
	for applied := 0; applied <= len(o.sets); applied++ {
		// Every prefix renders: the sets were applied in this order once already.
		partial, _ := render(p, o, applied)
		err := load(partial)
		if err == nil {
			continue
		}
		if applied == 0 {
			break
		}
		return fmt.Errorf("%s: %s", o.sets[applied-1], lineNumbers.ReplaceAllString(err.Error(), ""))
	}
	if len(p.names) == 0 {
		if err := load(data); err != nil {
			return err
		}
		return failure
	}
	filled := make([]string, len(p.variables))
	for i, name := range p.variables {
		value, _ := o.lookup(name)
		filled[i] = "${" + name + "}=" + value
	}
	return fmt.Errorf("%s (with %s)", lineNumbers.ReplaceAllString(failure.Error(), ""),
		strings.Join(filled, ", "))
}

// render applies the first applied sets, then fills every placeholder, and
// encodes the result.
func render(p *placeholders, o loadOptions, applied int) ([]byte, error) {
	var document yaml3.Node
	if err := yaml3.Unmarshal(p.tokenized, &document); err != nil {
		return nil, err
	}
	if len(document.Content) == 0 {
		return nil, errors.New("api: the config is empty")
	}
	root := document.Content[0]
	for _, set := range o.sets[:applied] {
		if err := applySet(root, set); err != nil {
			return nil, err
		}
	}
	if err := fillPlaceholders(root, p, o.lookup); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	encoder := yaml3.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// fillPlaceholders replaces the tokens in every value under node with their
// variables' values, refusing a placeholder in a key.
func fillPlaceholders(node *yaml3.Node, p *placeholders, lookup func(string) (string, bool)) error {
	switch node.Kind {
	case yaml3.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			if key := node.Content[i]; tokenPattern.MatchString(key.Value) {
				return fmt.Errorf("line %d: placeholders are only allowed in values, "+
					"not in the key %s", key.Line, untokenize(key.Value, p))
			}
			if err := fillPlaceholders(node.Content[i+1], p, lookup); err != nil {
				return err
			}
		}
	case yaml3.SequenceNode, yaml3.DocumentNode:
		for _, child := range node.Content {
			if err := fillPlaceholders(child, p, lookup); err != nil {
				return err
			}
		}
	case yaml3.ScalarNode:
		if tokenPattern.MatchString(node.Value) {
			return fillScalar(node, p, lookup)
		}
	}
	return nil
}

func fillScalar(node *yaml3.Node, p *placeholders, lookup func(string) (string, bool)) error {
	var failure error
	text := tokenPattern.ReplaceAllStringFunc(node.Value, func(token string) string {
		name := p.names[tokenIndex(token)]
		if name == "" {
			return "${"
		}
		value, ok := lookup(name)
		if failure == nil {
			if !ok {
				failure = fmt.Errorf("line %d: ${%s} is not set", node.Line, name)
			} else if value == "" && node.Style&quotedStyles == 0 {
				failure = fmt.Errorf("line %d: ${%s} is set but empty; "+
					"quote the value if an empty string is meant", node.Line, name)
			}
		}
		return value
	})
	if failure != nil {
		return failure
	}
	if node.Style&quotedStyles != 0 {
		node.Value, node.Tag = text, "!!str"
		return nil
	}
	value, err := parseValue(text)
	if err != nil {
		return fmt.Errorf("line %d: %s, filled in, is not valid YAML: %w",
			node.Line, untokenize(node.Value, p), err)
	}
	*node = *value
	return nil
}

const quotedStyles = yaml3.SingleQuotedStyle | yaml3.DoubleQuotedStyle

func tokenIndex(token string) int {
	index, _ := strconv.Atoi(tokenPattern.FindStringSubmatch(token)[2])
	return index
}

// untokenize writes a value back as it appears in the file.
func untokenize(value string, p *placeholders) string {
	return tokenPattern.ReplaceAllStringFunc(value, func(token string) string {
		if name := p.names[tokenIndex(token)]; name != "" {
			return "${" + name + "}"
		}
		return "$${"
	})
}

// parseValue reads text as a single YAML value. Collections are kept in flow
// style, so the re-encoded document stays close to the file's own layout.
func parseValue(text string) (*yaml3.Node, error) {
	var document yaml3.Node
	if err := yaml3.Unmarshal([]byte(text), &document); err != nil {
		return nil, err
	}
	if len(document.Content) == 0 {
		return &yaml3.Node{Kind: yaml3.ScalarNode, Tag: "!!str", Value: text}, nil
	}
	value := document.Content[0]
	if value.Kind == yaml3.MappingNode || value.Kind == yaml3.SequenceNode {
		value.Style = yaml3.FlowStyle
	}
	return value, nil
}

// pathSegment is one step of a --set path: a mapping key, and optionally the
// name of the list entry under it.
type pathSegment struct{ key, entry string }

var segmentPattern = regexp.MustCompile(`^([^.\[\]]+)(?:\[name=([^\]]+)\])?$`)

func parsePath(path string) ([]pathSegment, error) {
	segments := []pathSegment{}
	start, depth := 0, 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) {
			switch path[i] {
			case '[':
				depth++
				continue
			case ']':
				depth--
				continue
			case '.':
				if depth > 0 {
					continue
				}
			default:
				continue
			}
		}
		match := segmentPattern.FindStringSubmatch(path[start:i])
		if match == nil {
			return nil, fmt.Errorf("%q is not a path segment: write a key, or "+
				"key[name=entry] to select a list entry by its name", path[start:i])
		}
		segments = append(segments, pathSegment{key: match[1], entry: match[2]})
		start = i + 1
	}
	return segments, nil
}

// applySet replaces the value a --set path addresses.
func applySet(root *yaml3.Node, set setOverride) error {
	failf := func(format string, args ...any) error {
		return fmt.Errorf("%s: %s", set, fmt.Sprintf(format, args...))
	}
	segments, err := parsePath(set.path)
	if err != nil {
		return failf("%v", err)
	}
	node, at := root, ""
	for _, segment := range segments {
		if node.Kind != yaml3.MappingNode {
			return failf("%s is not a mapping, so it has no %s", at, segment.key)
		}
		at = strings.TrimPrefix(at+"."+segment.key, ".")
		value := mappingValue(node, segment.key)
		if value == nil {
			return failf("the config has no %s", at)
		}
		if value.Kind == yaml3.AliasNode {
			return failf("%s is a YAML alias; set the anchored value instead", at)
		}
		node = value
		if segment.entry == "" {
			continue
		}
		if node.Kind != yaml3.SequenceNode {
			return failf("%s is not a list, so [name=%s] selects nothing", at, segment.entry)
		}
		var found *yaml3.Node
		for _, item := range node.Content {
			if name := mappingValue(item, "name"); name != nil && name.Value == segment.entry {
				if found != nil {
					return failf("%s has more than one entry named %q", at, segment.entry)
				}
				found = item
			}
		}
		if found == nil {
			return failf("%s has no entry named %q", at, segment.entry)
		}
		node = found
		at += "[name=" + segment.entry + "]"
	}
	value, err := overrideValue(node, set.value)
	if err != nil {
		return failf("%v", err)
	}
	*node = *value
	return nil
}

func mappingValue(node *yaml3.Node, key string) *yaml3.Node {
	if node.Kind != yaml3.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// overrideValue reads raw as the replacement for existing, which it must match
// in shape. Text takes raw as it is; a placeholder, or null, takes any value.
func overrideValue(existing *yaml3.Node, raw string) (*yaml3.Node, error) {
	value, err := parseValue(raw)
	if err != nil {
		return nil, fmt.Errorf("the value is not valid YAML: %w", err)
	}
	if tokenPattern.MatchString(existing.Value) || existing.ShortTag() == "!!null" {
		return value, nil
	}
	want := shapeOf(existing)
	if want == "text" {
		return &yaml3.Node{Kind: yaml3.ScalarNode, Tag: "!!str", Value: raw}, nil
	}
	if got := shapeOf(value); got != want {
		return nil, fmt.Errorf("the config has %s here, not %s", want, got)
	}
	return value, nil
}

// oldBooleans are the plain spellings yaml.v2, which decodes the config, reads
// as booleans under YAML 1.1, though yaml.v3 reads them as text.
var oldBooleans = regexp.MustCompile(`^(y|Y|yes|Yes|YES|n|N|no|No|NO|on|On|ON|off|Off|OFF)$`)

// shapeOf names a value's shape as the config's decoder sees it.
func shapeOf(node *yaml3.Node) string {
	switch node.Kind {
	case yaml3.MappingNode:
		return "a mapping"
	case yaml3.SequenceNode:
		return "a list"
	}
	switch node.ShortTag() {
	case "!!int", "!!float":
		return "a number"
	case "!!bool":
		return "a boolean"
	case "!!str":
		if node.Style == 0 && oldBooleans.MatchString(node.Value) {
			return "a boolean"
		}
		return "text"
	}
	return node.ShortTag()
}
