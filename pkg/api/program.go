package api

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"text/template"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// PartitionConfigStrings holds the string Iteration implementations
// and the potential to define extra variables in configuration.
type PartitionConfigStrings struct {
	Iteration          string                       `yaml:"iteration"`
	ExtraVarsByPackage map[string]map[string]string `yaml:"extra_vars_by_package,omitempty"`
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
	Main     RunConfig            `yaml:"main_run"`
	Embedded map[string]RunConfig `yaml:"embedded_runs,omitempty"`
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
	Main     RunConfigStrings            `yaml:"main_run"`
	Embedded map[string]RunConfigStrings `yaml:"embedded_runs,omitempty"`
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
	return &config
}

// ApiCodeTemplate is a string representing the template for the API run code.
var ApiCodeTemplate = `package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

func main() {
	settingsConfig := api.LoadStochadexConfigSettingsFromYaml("{{.SettingsFile}}")
	{{.ExtraVars}}
	implementations := {{.Implementations}}
    api.Run(
	    &settingsConfig.Simulation.Settings,
		implementations,
	    {{.Websocket}},
		{{.Dashboard}},
		{{.MillisecondDelay}},
		"{{.ReactAppLocation}}",
		"{{.Handle}}",
		"{{.Address}}",
	)
}`

// WriteMainProgram writes string representations of various types of data
// to a template /tmp/*main.go file ready for runtime execution in this *main.go.
func WriteMainProgram(
	configFile string,
	config *ApiRunConfigStrings,
	dashboard *DashboardConfig,
) string {
	websocketOn := "true"
	if dashboard.Address == "" {
		websocketOn = "false"
		dashboard.Address = "dummy"
		dashboard.Handle = "dummy"
	}
	fmt.Println("\nParsed run config ...")
	// simulationConfigRender :=
	implementationsString := ImplementationsConfigFromStrings(
		config.Simulation.Implementations,
	)
	extraVariables := ""
	extraPackagesSet := make(map[string]bool, 0)
	for _, extraVarsByPackage := range config.ExtraVarsByPackage {
		for extraPackage, extraVarsSlice := range extraVarsByPackage {
			if extraPackage != "" {
				extraPackagesSet[extraPackage] = true
			}
			for _, extraVars := range extraVarsSlice {
				for varName, varValue := range extraVars {
					extraVariables += varName + " := " + varValue + "\n    "
				}
			}
		}
	}
	if config.EmbeddedSimulations != nil {
		extraPackagesSet["github.com/umbralcalc/stochadex/pkg/general"] = true
		for i, embeddedSimulations := range config.EmbeddedSimulations {
			for varName, configStrings := range embeddedSimulations {
				extraVariables += varName + "Settings := " +
					"settingsConfig.EmbeddedSimulations[" + strconv.Itoa(i) + "]" +
					`["` + varName + `"].Settings` + "\n    "
				extraVariables += varName +
					" := general.NewEmbeddedSimulationRunIteration(" +
					"&" + varName + "Settings, " +
					ImplementationsConfigFromStrings(configStrings.Implementations) +
					")" + "\n    "
			}
		}
	}
	extraPackages := ""
	for extraPackage := range extraPackagesSet {
		if extraPackage != "" {
			extraPackages += "\"" + extraPackage + "\"" + "\n    "
		}
	}
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
			"SettingsFile":     configFile,
			"Dashboard":        strconv.FormatBool(dashboard.LaunchDashboard),
			"Websocket":        websocketOn,
			"Address":          dashboard.Address,
			"Handle":           dashboard.Handle,
			"ReactAppLocation": dashboard.ReactAppLocation,
			"MillisecondDelay": strconv.Itoa(int(dashboard.MillisecondDelay)),
			"ExtraVars":        extraVariables,
			"ExtraPackages":    extraPackages,
			"Implementations":  implementationsString,
		},
	)
	if err != nil {
		panic(err)
	}
	return file.Name()
}

// RunWithParsedArgs takes in the outputs from ArgParse and runs the
// stochadex with these configurations.
func RunWithParsedArgs(
	configFile string,
	config *StochadexConfigImplementationsStrings,
	dashboard *DashboardConfig,
) {
	// hydrate the template code and write it to a /tmp/*main.go
	fileName := WriteMainProgram(configFile, config, dashboard)
	defer os.Remove(fileName)

	// execute the code
	runCmd := exec.Command("go", "run", fileName)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		panic(err)
	}
}
