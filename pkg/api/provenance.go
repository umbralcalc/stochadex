package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// Provenance and caching (PLAN.md O.5). A run's provenance says exactly what
// produced its outputs: the config as resolved (after --set and ${VAR}), a
// fingerprint of each input's contents, any model file an iteration reads, and
// the build that ran it. Its Key hashes all of that, so two runs with the same
// key produce the same outputs: an orchestrator can use it as an idempotency
// key, and --skip-if-unchanged skips a run whose outputs already carry it.
//
// A run has a key only when it is cacheable: when everything that decides its
// outputs can be fingerprinted. A Postgres input (whose table can change under
// the same query), a live stream, a served run (driven by its clients), and a
// build whose identity was not stamped deliberately all make a run
// uncacheable, and the provenance says why.
//
// A build is identified by what its build process stamped: a release version
// (-X main.version), an image's commit (-X main.revision), or the module version
// of a `go install ...@vX`. Go's own version-control stamp is recorded but not
// trusted: a local build may have uncommitted changes, and in a git worktree
// nested in its main checkout Go stamps the main checkout's commit, so a cache
// keyed on it could skip a run after the code changed.

// Provenance describes what produced a run's outputs.
type Provenance struct {
	// Key identifies the run's outputs; it is empty when the run is not
	// cacheable, and Uncacheable then says why.
	Key         string            `json:"key,omitempty"`
	Uncacheable []string          `json:"uncacheable,omitempty"`
	Config      ProvenanceConfig  `json:"config"`
	Inputs      []ProvenanceInput `json:"inputs"`
	Files       []ProvenanceFile  `json:"files,omitempty"`
	Seeds       []uint64          `json:"seeds,omitempty"`
	Build       ProvenanceBuild   `json:"build"`
}

// ProvenanceConfig is the config as resolved: its path, the overrides applied,
// and a digest of the resolved document that ignores comments, layout and key
// order.
type ProvenanceConfig struct {
	Path      string   `json:"path"`
	SHA256    string   `json:"sha256"`
	Overrides []string `json:"overrides,omitempty"`
	Variables []string `json:"variables,omitempty"`
}

// ProvenanceInput is one input and the fingerprint of its contents.
type ProvenanceInput struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Location    string `json:"location,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// ProvenanceFile is a file an iteration reads that is not an input: a model
// file named by a model_path field. (IO.5 will make these inputs.)
type ProvenanceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// ProvenanceBuild is the build that ran. Version and Revision are what its
// build process stamped, and identify it; VCSRevision and VCSDirty are Go's
// own version-control stamp, recorded for information only.
type ProvenanceBuild struct {
	Version     string   `json:"version"`
	Revision    string   `json:"revision,omitempty"`
	VCSRevision string   `json:"vcs_revision,omitempty"`
	VCSDirty    bool     `json:"vcs_dirty,omitempty"`
	Features    []string `json:"features,omitempty"`
	OS          string   `json:"os"`
	Arch        string   `json:"arch"`
	Image       string   `json:"image,omitempty"`
}

var (
	fingerprintsMu     sync.RWMutex
	sourceFingerprints = map[string]func(fields map[string]interface{}) (string, error){}
)

// RegisterSourceFingerprint tells provenance how to fingerprint a source
// registered with RegisterDataSource: for an object store, say, its object
// version rather than its bytes. A registered source without one is
// fingerprinted by the file at its path field, and is uncacheable without one.
func RegisterSourceFingerprint(
	name string,
	fingerprint func(fields map[string]interface{}) (string, error),
) {
	fingerprintsMu.Lock()
	defer fingerprintsMu.Unlock()
	sourceFingerprints[name] = fingerprint
}

// ComputeProvenance fingerprints what a run of config would read. It reads
// every file input and model file once, so a missing input is ErrUnavailable,
// as it would be for the run.
func ComputeProvenance(config *ApiRunConfig) (*Provenance, error) {
	p := &Provenance{
		Config: ProvenanceConfig{Path: config.sourcePath, Overrides: config.overrides,
			Variables: config.variables, SHA256: configDigest(config.source)},
		Inputs: []ProvenanceInput{},
		Seeds:  config.Run.Seeds,
		Build:  currentBuild(),
	}
	uncacheable := func(format string, args ...any) {
		p.Uncacheable = append(p.Uncacheable, fmt.Sprintf(format, args...))
	}
	if config.source == nil {
		uncacheable("the config was not loaded from a document")
	}
	if config.Run.Mode == "serve" {
		uncacheable("a served run is driven by its clients")
	}
	if p.Build.Revision == "" && p.Build.Version == "dev" {
		uncacheable("the build is not a release or a stamped image, so it cannot be " +
			"pinned (Go's own version-control stamp is not trusted)")
	}
	inputs := config.Inputs
	if len(config.Macros) > 0 {
		inputs = macroInputs(config)
	}
	for _, name := range sortedKeys(inputs) {
		input := inputs[name]
		entry := ProvenanceInput{Name: name}
		switch {
		case input.Stream != nil:
			entry.Kind = "stream"
			uncacheable("input %q is a live stream", name)
		case input.Source != nil:
			entry.Kind, entry.Location = sourceLocation(input.Source)
			fingerprint, reason, err := sourceFingerprint(input.Source)
			if err != nil {
				return nil, inputError(fmt.Errorf("api: fingerprinting input %q: %w", name, err))
			}
			if reason != "" {
				uncacheable("input %q %s", name, reason)
			}
			entry.Fingerprint = fingerprint
		default:
			// Made by a sub-simulation the config itself describes.
			entry.Kind, entry.Fingerprint = "simulation", "config"
		}
		p.Inputs = append(p.Inputs, entry)
	}
	for _, path := range modelPaths(config) {
		digest, err := fileDigest(path)
		if err != nil {
			return nil, inputError(fmt.Errorf("api: fingerprinting model file %s: %w", path, err))
		}
		p.Files = append(p.Files, ProvenanceFile{Path: path, SHA256: digest})
	}
	if len(p.Uncacheable) == 0 {
		p.Key = p.key()
	}
	return p, nil
}

// key hashes everything that decides a run's outputs, and nothing else.
func (p *Provenance) key() string {
	inputs := make([]string, len(p.Inputs))
	for i, input := range p.Inputs {
		inputs[i] = input.Name + "=" + input.Fingerprint
	}
	files := make([]string, len(p.Files))
	for i, file := range p.Files {
		files[i] = file.Path + "=" + file.SHA256
	}
	identity, _ := json.Marshal(struct {
		Config   string
		Inputs   []string
		Files    []string
		Version  string
		Revision string
		Features []string
		OS, Arch string
		Image    string
	}{p.Config.SHA256, inputs, files, p.Build.Version, p.Build.Revision, p.Build.Features,
		p.Build.OS, p.Build.Arch, p.Build.Image})
	sum := sha256.Sum256(identity)
	return hex.EncodeToString(sum[:])
}

// configDigest hashes a config document as data: decoded and re-encoded as
// canonical JSON, so comments, layout and key order do not change it. A
// document that does not decode to JSON-able data is hashed as written.
func configDigest(source []byte) string {
	var data interface{}
	canonical := source
	if err := yaml3.Unmarshal(source, &data); err == nil {
		if encoded, err := json.Marshal(data); err == nil {
			canonical = encoded
		}
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// sourceFingerprint fingerprints a stored input's contents, or says why it
// cannot be.
func sourceFingerprint(source *DataSource) (fingerprint, uncacheable string, err error) {
	switch {
	case source.Inline != nil:
		// Carried in the config, so its digest covers it.
		return "config", "", nil
	case source.Csv != nil:
		digest, err := fileDigest(source.Csv.Path)
		return "sha256:" + digest, "", err
	case source.JsonLog != nil:
		digest, err := fileDigest(source.JsonLog.Path)
		return "sha256:" + digest, "", err
	case source.Postgres != nil:
		return "", "reads a Postgres table, whose rows can change under the same query", nil
	}
	for _, name := range sortedKeys(source.Extra) {
		fields := source.Extra[name]
		fingerprintsMu.RLock()
		registered := sourceFingerprints[name]
		fingerprintsMu.RUnlock()
		if registered != nil {
			fingerprint, err := registered(fields)
			return fingerprint, "", err
		}
		if path, ok := fields["path"].(string); ok && path != "" {
			digest, err := fileDigest(path)
			return "sha256:" + digest, "", err
		}
		return "", fmt.Sprintf("is a %s source, which has no fingerprint", name), nil
	}
	return "", "has no source", nil
}

// fileDigest is the SHA-256 of a file's contents.
func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// modelPaths finds the files iterations read through a model_path field,
// anywhere in a partition's iteration spec, in main, embedded runs and data:.
func modelPaths(config *ApiRunConfig) []string {
	found := map[string]bool{}
	var walk func(value interface{})
	walk = func(value interface{}) {
		switch v := value.(type) {
		case map[string]interface{}:
			for key, field := range v {
				if path, ok := field.(string); ok && key == "model_path" {
					found[path] = true
				}
				walk(field)
			}
		case map[interface{}]interface{}:
			for key, field := range v {
				if path, ok := field.(string); ok && key == "model_path" {
					found[path] = true
				}
				walk(field)
			}
		case []interface{}:
			for _, item := range v {
				walk(item)
			}
		case simulator.ComponentSpec:
			walk(v.Fields)
		}
	}
	partitions := append([]simulator.PartitionConfig(nil), config.Main.Partitions...)
	for _, embedded := range config.Embedded {
		partitions = append(partitions, embedded.Run.Partitions...)
	}
	if config.Data != nil {
		partitions = append(partitions, config.Data.Partitions...)
	}
	for _, input := range config.Inputs {
		if input.Simulation != nil {
			partitions = append(partitions, input.Simulation.Partitions...)
		}
	}
	for _, partition := range partitions {
		walk(partition.IterationSpec.Fields)
	}
	paths := make([]string, 0, len(found))
	for path := range found {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// buildVCS reads Go's version-control stamp, and buildModuleVersion the main
// module's version (set by `go install ...@vX`); tests replace them.
var (
	buildVCS           = vcsRevision
	buildModuleVersion = func() string {
		info, _ := debug.ReadBuildInfo()
		return moduleVersion(info)
	}
)

// moduleVersion is the main module's version when it identifies the build: a
// `go install ...@vX` build from the module cache, which carries no
// version-control stamp. A build in a checkout has its version derived from
// that stamp (a pseudo-version since Go 1.24), which is not trusted.
func moduleVersion(info *debug.BuildInfo) string {
	if info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs" {
			return ""
		}
	}
	return info.Main.Version
}

func currentBuild() ProvenanceBuild {
	build := ProvenanceBuild{Version: BuildVersion, Revision: BuildRevision, OS: runtime.GOOS,
		Arch: runtime.GOARCH, Image: os.Getenv(imageDigestEnv)}
	if build.Version == "dev" {
		if module := buildModuleVersion(); module != "" {
			build.Version = module
		}
	}
	if revision, dirty, ok := buildVCS(); ok {
		build.VCSRevision, build.VCSDirty = revision, dirty
	}
	if len(BuildFeatures) > 0 {
		build.Features = append([]string(nil), BuildFeatures...)
		sort.Strings(build.Features)
	}
	return build
}

// sidecarSuffix names a file output's provenance sidecar.
const sidecarSuffix = ".provenance.json"

// sidecarTargets are the files a run's provenance is written beside: its
// json_log and arrow outputs, nested runs' included, one per ensemble member.
// Other outputs are databases, object stores or streams, with no file of the
// run's own, and a stream's record: belongs to a run that is never cacheable.
func sidecarTargets(config *ApiRunConfig) []string {
	targets := []string{}
	for _, output := range Manifest(config).Outputs {
		if output.Sink != "json_log" && output.Sink != "arrow" ||
			strings.HasPrefix(output.DeclaredIn, "inputs.") {
			continue
		}
		locations := output.Locations
		if len(locations) == 0 {
			locations = []string{output.Location}
		}
		targets = append(targets, locations...)
	}
	return targets
}

// upToDate reports whether every file output already exists with a sidecar
// carrying p's key: whether the run would only reproduce them.
func upToDate(p *Provenance, targets []string) (bool, string) {
	switch {
	case p.Key == "":
		return false, "the run is not cacheable: " + strings.Join(p.Uncacheable, "; ")
	case len(targets) == 0:
		return false, "the run writes no file outputs to compare"
	}
	for _, target := range targets {
		if _, err := os.Stat(target); err != nil {
			return false, target + " does not exist"
		}
		data, err := os.ReadFile(target + sidecarSuffix)
		if err != nil {
			return false, target + " has no provenance"
		}
		var recorded Provenance
		if json.Unmarshal(data, &recorded) != nil || recorded.Key != p.Key {
			return false, target + " was made by a different run"
		}
	}
	return true, ""
}

// writeSidecars writes p beside each target, replacing any earlier sidecar in
// one step. A run publishes its outputs first, so a sidecar only ever
// describes a whole output.
func writeSidecars(p *Provenance, targets []string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	var failures []error
	for _, target := range targets {
		path := target + sidecarSuffix
		partial := path + ".partial"
		if err := os.WriteFile(partial, append(data, '\n'), 0o644); err != nil {
			failures = append(failures, err)
			continue
		}
		failures = append(failures, os.Rename(partial, path))
	}
	if failed := errors.Join(failures...); failed != nil {
		return withKind(ErrUnavailable, fmt.Errorf("api: writing provenance: %w", failed))
	}
	return nil
}
