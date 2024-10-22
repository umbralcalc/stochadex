package api

import (
	"fmt"
	"os"
	"os/exec"
	"text/template"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// PartitionConfigStrings holds the string Iteration implementations
// and the potential to define extra packages and variables in configuration.
type PartitionConfigStrings struct {
	Name          string              `yaml:"name"`
	Iteration     string              `yaml:"iteration,omitempty"`
	ExtraPackages []string            `yaml:"extra_packages,omitempty"`
	ExtraVars     []map[string]string `yaml:"extra_vars,omitempty"`
}

// RunConfig is the yaml-loadable config which consists of all the
// necessary config data for a simulation run.
type RunConfig struct {
	Partitions []simulator.PartitionConfig `yaml:"partitions"`
	Simulation simulator.SimulationConfig
}

// GetConfigGenerator returns a config generator which has been configured
// to produce the settings and implementations based on the configured fields
// of the RunConfig.
func (r *RunConfig) GetConfigGenerator() *simulator.ConfigGenerator {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&r.Simulation)
	for _, partition := range r.Partitions {
		generator.SetPartition(&partition)
	}
	return generator
}

// RunConfigStrings is the yaml-loadable config which consists of
// all the necessary templating inputs for a simulation run.
type RunConfigStrings struct {
	Partitions []PartitionConfigStrings          `yaml:"partitions"`
	Simulation simulator.SimulationConfigStrings `yaml:"simulation"`
}

// ApiRunConfig is the yaml-loadable config which specifies the loadable
// config data within the generated code for an API run.
type ApiRunConfig struct {
	Main     RunConfig            `yaml:"main"`
	Embedded map[string]RunConfig `yaml:"embedded,omitempty"`
}

// GetConfigGenerator returns a config generator which has been configured
// to produce the settings and implementations based on the configured fields
// of the main RunConfig. This method also takes into account the mappings of
// embedded simulation runs into the main run.
func (a *ApiRunConfig) GetConfigGenerator() *simulator.ConfigGenerator {
	generator := a.Main.GetConfigGenerator()
	for name, embeddedRun := range a.Embedded {
		partition := generator.GetPartition(name)
		partition.Iteration = general.NewEmbeddedSimulationRunIteration(
			embeddedRun.GetConfigGenerator().GenerateConfigs(),
		)
		generator.ResetPartition(name, partition)
	}
	return generator
}

// LoadApiRunConfigFromYaml creates a new ApiRunConfig from a provided
// yaml path.
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
	for _, embeddedRun := range config.Embedded {
		for index := range embeddedRun.Partitions {
			embeddedRun.Partitions[index].Init()
		}
	}
	if err != nil {
		panic(err)
	}
	return &config
}

// ApiRunConfigStrings is the yaml-loadable config which specifies the
// templating inputs for an API run.
type ApiRunConfigStrings struct {
	Main     RunConfigStrings            `yaml:"main"`
	Embedded map[string]RunConfigStrings `yaml:"embedded,omitempty"`
}

// validateApiRunConfigStrings checks the ApiRunConfigStrings for errors
// and panics if there are any.
func validateApiRunConfigStrings(config *ApiRunConfigStrings) {
	for _, partition := range config.Main.Partitions {
		if partition.Iteration == "" {
			_, ok := config.Embedded[partition.Name]
			if !ok {
				panic("config omits iteration for partition name: " +
					partition.Name +
					" and no embedded simulation runs have this name")
			}
		}
	}
}

// LoadApiRunConfigStringsFromYaml creates a new ApiRunConfigStrings
// from a provided yaml path.
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

// formatExtraCode takes the parsed args and returns the extra code formatted
// to work with templating.
func formatExtraCode(args ParsedArgs) string {
	extraCode := ""
	for i, partition := range args.ConfigStrings.Main.Partitions {
		if partition.Iteration == "" {
			continue
		}
		extraCode += fmt.Sprintf(
			`config.Main.Partitions[%d].Iteration = %s`,
			i,
			partition.Iteration,
		)
		extraCode += "\n    "
	}
	extraCode += fmt.Sprintf(
		`config.Main.Simulation = simulator.SimulationConfig{
	    OutputCondition: %s,
	    OutputFunction: %s,
	    TerminationCondition: %s,
	    TimestepFunction: %s,
	    InitTimeValue: %f,
	    TimestepsHistoryDepth: %d}`,
		args.ConfigStrings.Main.Simulation.OutputCondition,
		args.ConfigStrings.Main.Simulation.OutputFunction,
		args.ConfigStrings.Main.Simulation.TerminationCondition,
		args.ConfigStrings.Main.Simulation.TimestepFunction,
		args.ConfigStrings.Main.Simulation.InitTimeValue,
		args.ConfigStrings.Main.Simulation.TimestepsHistoryDepth,
	)
	extraCode += "\n    "
	for runName, embeddedRun := range args.ConfigStrings.Embedded {
		extraCode += fmt.Sprintf(
			`if embeddedSim, ok := config.Embedded["%s"]; ok {`,
			runName,
		)
		extraCode += "\n        "
		for i, partition := range embeddedRun.Partitions {
			if partition.Iteration == "" {
				continue
			}
			extraCode += fmt.Sprintf(
				`embeddedSim.Partitions[%d].Iteration = %s`,
				i,
				partition.Iteration,
			)
			extraCode += "\n        "
		}
		extraCode += fmt.Sprintf(
			`embeddedSim.Simulation = simulator.SimulationConfig{
		    OutputCondition: %s,
		    OutputFunction: %s,
		    TerminationCondition: %s,
		    TimestepFunction: %s,
		    InitTimeValue: %f,
		    TimestepsHistoryDepth: %d}`,
			args.ConfigStrings.Main.Simulation.OutputCondition,
			args.ConfigStrings.Main.Simulation.OutputFunction,
			args.ConfigStrings.Main.Simulation.TerminationCondition,
			args.ConfigStrings.Main.Simulation.TimestepFunction,
			args.ConfigStrings.Main.Simulation.InitTimeValue,
			args.ConfigStrings.Main.Simulation.TimestepsHistoryDepth,
		)
		extraCode += "\n        "
		extraCode += fmt.Sprintf(
			`config.Embedded["%s"] = embeddedSim`,
			runName,
		)
		extraCode += "\n    }"
	}
	return extraCode
}

// formatExtraVariables takes the parsed args and returns the extra variable
// declarations formatted to work with templating.
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
	for _, embeddedRun := range args.ConfigStrings.Embedded {
		for _, partition := range embeddedRun.Partitions {
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

// formatExtraPackages takes the parsed args and returns the extra package
// imports formatted to work with templating.
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
	for _, embeddedRun := range args.ConfigStrings.Embedded {
		for _, partition := range embeddedRun.Partitions {
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

// ApiCodeTemplate is a string representing the template for the API run code.
var ApiCodeTemplate = `package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

func main() {
	config := api.LoadApiRunConfigFromYaml("{{.ConfigFile}}")
	dashboard := api.LoadDashboardConfigFromYaml("{{.DashboardFile}}")
	{{.ExtraVars}}
	{{.ExtraCode}}
    api.Run(config, dashboard)
}`

// WriteMainProgram writes string representations of various types of data
// to a template /tmp/*main.go file ready for runtime execution in this *main.go.
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
			"DashboardFile": args.DashboardFile,
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

// RunWithParsedArgs takes in the outputs from ArgParse and runs the
// stochadex with these configurations.
func RunWithParsedArgs(args ParsedArgs) {
	// hydrate the template code and write it to a /tmp/*main.go
	fileName := WriteMainProgram(args)
	defer os.Remove(fileName)

	// execute the code
	runCmd := exec.Command("go", "run", fileName)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		panic(err)
	}
}
