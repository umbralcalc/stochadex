package api

import (
	"fmt"
	"os"
	"text/template"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// PartitionConfigStrings describes a partition in its string-templated form
// for code generation.
//
// Usage hints:
//   - Iteration is a Go expression constructing the iteration (factory call).
//   - ExtraPackages lists import paths required by Iteration or ExtraVars.
//   - ExtraVars declares ad-hoc variables injected into the generated main.
type PartitionConfigStrings struct {
	Name          string              `yaml:"name"`
	Iteration     string              `yaml:"iteration,omitempty"`
	ExtraPackages []string            `yaml:"extra_packages,omitempty"`
	ExtraVars     []map[string]string `yaml:"extra_vars,omitempty"`
}

// RunConfig is the concrete, YAML-loadable configuration for a run: partitions
// plus a SimulationConfig.
type RunConfig struct {
	Partitions []simulator.PartitionConfig `yaml:"partitions"`
	Simulation simulator.SimulationConfig  `yaml:"-"`
}

// GetConfigGenerator constructs a ConfigGenerator preloaded with the run's
// SimulationConfig and Partitions.
func (r *RunConfig) GetConfigGenerator() *simulator.ConfigGenerator {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&r.Simulation)
	for _, partition := range r.Partitions {
		generator.SetPartition(&partition)
	}
	return generator
}

// EmbeddedRunConfig names and embeds an additional RunConfig that can be
// wired into a partition in the main run.
type EmbeddedRunConfig struct {
	Name string    `yaml:"name"`
	Run  RunConfig `yaml:",inline"`
}

// RunConfigStrings provides the string-templated inputs required to generate
// a runnable main for a simulation run.
type RunConfigStrings struct {
	Partitions []PartitionConfigStrings          `yaml:"partitions"`
	Simulation simulator.SimulationConfigStrings `yaml:"simulation"`
}

// EmbeddedRunConfigStrings names and provides string-templated inputs for an
// embedded simulation run.
type EmbeddedRunConfigStrings struct {
	Name string           `yaml:"name"`
	Run  RunConfigStrings `yaml:",inline"`
}

// ApiRunConfig is the concrete, YAML-loadable configuration for an API run:
// a main RunConfig and optional embedded runs.
type ApiRunConfig struct {
	Main     RunConfig           `yaml:"main"`
	Embedded []EmbeddedRunConfig `yaml:"embedded,omitempty"`
}

// GetConfigGenerator returns a ConfigGenerator for the main run. Any partition
// whose name matches an embedded run is replaced by an embedded simulation
// iteration wired to that embedded run.
func (a *ApiRunConfig) GetConfigGenerator() *simulator.ConfigGenerator {
	generator := a.Main.GetConfigGenerator()
	for _, embedded := range a.Embedded {
		partition := generator.GetPartition(embedded.Name)
		partition.Iteration = general.NewEmbeddedSimulationRunIteration(
			embedded.Run.GetConfigGenerator().GenerateConfigs(),
		)
		generator.ResetPartition(embedded.Name, partition)
	}
	return generator
}

// LoadApiRunConfigFromYaml loads ApiRunConfig from YAML and initialises
// partition defaults.
func LoadApiRunConfigFromYaml(path string) *ApiRunConfig {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var config ApiRunConfig
	err = yaml.Unmarshal(yamlFile, &config)
	for index := range config.Main.Partitions {
		config.Main.Partitions[index].Init()
	}
	for _, embedded := range config.Embedded {
		for index := range embedded.Run.Partitions {
			embedded.Run.Partitions[index].Init()
		}
	}
	if err != nil {
		panic(err)
	}
	return &config
}

// ApiRunConfigStrings is the string-templated configuration used to generate
// code for an API run (imports, variables, iteration factories).
type ApiRunConfigStrings struct {
	Main     RunConfigStrings           `yaml:"main"`
	Embedded []EmbeddedRunConfigStrings `yaml:"embedded,omitempty"`
}

// validateApiRunConfigStrings asserts the templated config is coherent.
// Any partition without an Iteration must correspond to a named embedded run.
func validateApiRunConfigStrings(config *ApiRunConfigStrings) {
	embeddedNames := make(map[string]bool)
	for _, embedded := range config.Embedded {
		embeddedNames[embedded.Name] = true
	}
	for _, partition := range config.Main.Partitions {
		if partition.Iteration == "" {
			_, ok := embeddedNames[partition.Name]
			if !ok {
				panic("config omits iteration for partition name: " +
					partition.Name +
					" and no embedded simulation runs have this name")
			}
		}
	}
}

// LoadApiRunConfigStringsFromYaml loads the templated config from YAML and
// validates it for code generation.
func LoadApiRunConfigStringsFromYaml(path string) *ApiRunConfigStrings {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var config ApiRunConfigStrings
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	validateApiRunConfigStrings(&config)
	return &config
}

// formatExtraCode serialises Iteration factories and SimulationConfig into
// Go code fragments for main and embedded runs.
func formatExtraCode(args ParsedArgs) string {
	extraCode := ""
	for i, partition := range args.ConfigStrings.Main.Partitions {
		if partition.Iteration == "" {
			continue
		}
		extraCode += fmt.Sprintf(
			`config.Main.Partitions[%d].Iteration = %s`+"\n    ",
			i, partition.Iteration,
		)
	}
	extraCode += fmt.Sprintf(
		`config.Main.Simulation = simulator.SimulationConfig{
	OutputCondition: %s,
	OutputFunction: %s,
	TerminationCondition: %s,
	TimestepFunction: %s,
	InitTimeValue: %f}`+"\n    ",
		args.ConfigStrings.Main.Simulation.OutputCondition,
		args.ConfigStrings.Main.Simulation.OutputFunction,
		args.ConfigStrings.Main.Simulation.TerminationCondition,
		args.ConfigStrings.Main.Simulation.TimestepFunction,
		args.ConfigStrings.Main.Simulation.InitTimeValue,
	)
	for i, embedded := range args.ConfigStrings.Embedded {
		for j, partition := range embedded.Run.Partitions {
			if partition.Iteration == "" {
				continue
			}
			extraCode += fmt.Sprintf(
				`config.Embedded[%d].Run.Partitions[%d].Iteration = %s`+"\n    ",
				i, j, partition.Iteration,
			)
		}
		extraCode += fmt.Sprintf(
			`config.Embedded[%d].Run.Simulation = simulator.SimulationConfig{
		OutputCondition: %s,
		OutputFunction: %s,
		TerminationCondition: %s,
		TimestepFunction: %s,
		InitTimeValue: %f}`+"\n    ",
			i,
			embedded.Run.Simulation.OutputCondition,
			embedded.Run.Simulation.OutputFunction,
			embedded.Run.Simulation.TerminationCondition,
			embedded.Run.Simulation.TimestepFunction,
			embedded.Run.Simulation.InitTimeValue,
		)
	}
	return extraCode
}

// formatExtraVariables emits variable declarations from the templated config
// so they are available to code fragments.
func formatExtraVariables(args ParsedArgs) string {
	extraVariables := ""
	for _, partition := range args.ConfigStrings.Main.Partitions {
		if partition.ExtraVars == nil {
			continue
		}
		for _, extraVars := range partition.ExtraVars {
			for varName, varValue := range extraVars {
				extraVariables += varName + " := " + varValue + "\n    "
			}
		}
	}
	for _, embedded := range args.ConfigStrings.Embedded {
		for _, partition := range embedded.Run.Partitions {
			if partition.ExtraVars == nil {
				continue
			}
			for _, extraVars := range partition.ExtraVars {
				for varName, varValue := range extraVars {
					extraVariables += varName + " := " + varValue + "\n    "
				}
			}
		}
	}
	return extraVariables
}

// formatExtraPackages de-duplicates and renders import paths required by code
// fragments for main and embedded runs.
func formatExtraPackages(args ParsedArgs) string {
	extraPackages := ""
	extraPackagesSet := make(map[string]bool)
	if args.ConfigStrings.Embedded != nil {
		extraPackagesSet["github.com/umbralcalc/stochadex/pkg/general"] = true
	}
	for _, partition := range args.ConfigStrings.Main.Partitions {
		if partition.ExtraPackages == nil {
			continue
		}
		for _, packageName := range partition.ExtraPackages {
			extraPackagesSet[packageName] = true
		}
	}
	for _, embedded := range args.ConfigStrings.Embedded {
		for _, partition := range embedded.Run.Partitions {
			if partition.ExtraPackages == nil {
				continue
			}
			for _, packageName := range partition.ExtraPackages {
				extraPackagesSet[packageName] = true
			}
		}
	}
	for extraPackage := range extraPackagesSet {
		if extraPackage != "" {
			extraPackages += "\"" + extraPackage + "\"" + "\n    "
		}
	}
	return extraPackages
}

// ApiCodeTemplate is the Go source template used to generate a runnable
// temporary main program that executes the requested run configuration.
var ApiCodeTemplate = `package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

func main() {
	config := api.LoadApiRunConfigFromYaml("{{.ConfigFile}}")
	socket := api.LoadSocketConfigFromYaml("{{.SocketFile}}")
	{{.ExtraVars}}
	{{.ExtraCode}}
    api.Run(config, socket)
}`

// WriteMainProgram renders ApiCodeTemplate to a /tmp/*main.go and returns the
// file path.
//
// Usage hints:
//   - The generated program imports extra packages, declares extra variables,
//     and wires iterations per the templated config.
func WriteMainProgram(args ParsedArgs) string {
	fmt.Println("\nParsed run config ...")
	codeTemplate := template.New("stochadexMain")
	template.Must(codeTemplate.Parse(ApiCodeTemplate))
	file, err := os.CreateTemp("/tmp", "*main.go")
	if err != nil {
		err := os.Mkdir("/tmp", 0755)
		if err != nil {
			panic(err)
		}
		file, _ = os.CreateTemp("/tmp", "*main.go")
	}
	err = codeTemplate.Execute(
		file,
		map[string]string{
			"ConfigFile":    args.ConfigFile,
			"SocketFile":    args.SocketFile,
			"ExtraVars":     formatExtraVariables(args),
			"ExtraPackages": formatExtraPackages(args),
			"ExtraCode":     formatExtraCode(args),
		},
	)
	if err != nil {
		panic(err)
	}
	return file.Name()
}
