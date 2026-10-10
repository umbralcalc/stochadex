package api

import (
	"fmt"
	"sort"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Files an iteration reads are inputs (PLAN.md IO.5, Q8): declared once in
// inputs:, so the I/O manifest and provenance see them like any other input.
//
//	inputs:
//	  brain: {file: {path: models/brain.onnx}}
//	main:
//	  partitions:
//	  - name: policy
//	    iteration: {type: onnx_inference, model_path: {input: brain}, ...}
//
// Any iteration field written {input: NAME}, naming a file input, is given that
// file's path when the config loads, before the iteration is built, so an
// iteration (the engine's or a downstream one) just sees a path. A bare
// model_path: some.onnx is shorthand for an implicit file input of that path,
// shared by every partition that names the same file.

// FileInputConfig is a file an iteration reads.
type FileInputConfig struct {
	Path string `yaml:"path"`
}

// modelPathField names the field a model file is read from; given as a plain
// path it is shorthand for a file input.
const modelPathField = "model_path"

// implicitFilePrefix names the input a bare model_path becomes.
const implicitFilePrefix = "file/"

// resolveFileInputs gives each {input: NAME} iteration field its file input's
// path, makes each bare model_path an implicit file input, and records what
// reads each file input. It looks at main partitions, embedded runs' and input
// sub-simulations'; a legacy data: block cannot have inputs: beside it, so its
// partitions are left as written.
func resolveFileInputs(config *ApiRunConfig) error {
	config.fileReaders = map[string][]string{}
	config.readsFiles = map[string]bool{}
	// scope names a run's partitions in readsFiles: "main", or an embedded
	// run's name; "" for partitions Check never configures.
	visit := func(at, scope string, partitions []simulator.PartitionConfig) error {
		for index := range partitions {
			partition := &partitions[index]
			where := at + "[name=" + partition.Name + "].iteration"
			fields := partition.IterationSpec.Fields
			before := countReaders(config.fileReaders)
			for _, key := range sortedKeys(fields) {
				value, err := resolveFileField(config, where+"."+key, key, fields[key])
				if err != nil {
					return err
				}
				fields[key] = value
			}
			if scope != "" && countReaders(config.fileReaders) > before {
				config.readsFiles[scope+"/"+partition.Name] = true
			}
		}
		return nil
	}
	if err := visit("main.partitions", "main", config.Main.Partitions); err != nil {
		return err
	}
	for _, embedded := range config.Embedded {
		if err := visit("embedded[name="+embedded.Name+"].partitions", embedded.Name,
			embedded.Run.Partitions); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(config.Inputs) {
		if simulation := config.Inputs[name].Simulation; simulation != nil {
			if err := visit("inputs."+name+".simulation.partitions", "", simulation.Partitions); err != nil {
				return err
			}
		}
	}
	for name := range config.fileReaders {
		sort.Strings(config.fileReaders[name])
	}
	return nil
}

func countReaders(readers map[string][]string) int {
	count := 0
	for _, where := range readers {
		count += len(where)
	}
	return count
}

// resolveFileField resolves one field, at any depth: a reference {input: NAME}
// becomes the file's path, and a bare model_path is made an implicit input.
func resolveFileField(config *ApiRunConfig, where, key string, value interface{}) (interface{}, error) {
	if name, ok := inputReference(value); ok {
		input, declared := config.Inputs[name]
		switch {
		case !declared:
			return nil, fmt.Errorf("api: %s names input %q, which inputs: does not declare", where, name)
		case input.File == nil:
			return nil, fmt.Errorf("api: %s names input %q, which is not a file input; "+
				"declare the file as %s: {file: {path: ...}}", where, name, name)
		}
		config.fileReaders[name] = append(config.fileReaders[name], where)
		return input.File.Path, nil
	}
	if path, ok := value.(string); ok && key == modelPathField {
		name := implicitFilePrefix + path
		if existing, declared := config.Inputs[name]; declared &&
			(existing.File == nil || existing.File.Path != path) {
			return nil, fmt.Errorf("api: input %q is the name the model file %s takes; "+
				"rename the input", name, path)
		}
		if config.Inputs == nil {
			config.Inputs = map[string]InputConfig{}
		}
		config.Inputs[name] = InputConfig{File: &FileInputConfig{Path: path}}
		config.fileReaders[name] = append(config.fileReaders[name], where)
		return value, nil
	}
	switch nested := value.(type) {
	case map[interface{}]interface{}:
		for field, inner := range nested {
			label, _ := field.(string)
			resolved, err := resolveFileField(config, where+"."+label, label, inner)
			if err != nil {
				return nil, err
			}
			nested[field] = resolved
		}
	case map[string]interface{}:
		for field, inner := range nested {
			resolved, err := resolveFileField(config, where+"."+field, field, inner)
			if err != nil {
				return nil, err
			}
			nested[field] = resolved
		}
	case []interface{}:
		for i, inner := range nested {
			resolved, err := resolveFileField(config, fmt.Sprintf("%s[%d]", where, i), "", inner)
			if err != nil {
				return nil, err
			}
			nested[i] = resolved
		}
	}
	return value, nil
}

// inputReference reads a field written {input: NAME}: a mapping with that one
// key.
func inputReference(value interface{}) (string, bool) {
	switch mapping := value.(type) {
	case map[interface{}]interface{}:
		if len(mapping) == 1 {
			name, ok := mapping["input"].(string)
			return name, ok
		}
	case map[string]interface{}:
		if len(mapping) == 1 {
			name, ok := mapping["input"].(string)
			return name, ok
		}
	}
	return "", false
}
