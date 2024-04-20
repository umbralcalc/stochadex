package api

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"text/template"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// SimulationConfigImplementationsStrings is the yaml-loadable config which
// consists of all the necessary config to load a simulation's implementations
// into templating.
type SimulationConfigImplementationStrings struct {
	Implementations simulator.ImplementationStrings `yaml:"implementations"`
}

// SimulationConfigSettings is the yaml-loadable config which consists of
// all the necessary config to load a simulation's settings into templating.
type SimulationConfigSettings struct {
	Settings simulator.Settings `yaml:"settings"`
}

// StochadexConfigImplementationsStrings is the yaml-loadable config which
// consists of all the necessary implementations information to compile a
// stochadex run binary with templating.
type StochadexConfigImplementationsStrings struct {
	Simulation          SimulationConfigImplementationStrings              `yaml:"simulation"`
	EmbeddedSimulations []map[string]SimulationConfigImplementationStrings `yaml:"embedded_simulations,omitempty"`
	ExtraVarsByPackage  []map[string][]map[string]string                   `yaml:"extra_vars_by_package,omitempty"`
}

// StochadexConfigSettings is the yaml-loadable config which consists of
// all the necessary settings information to compile a stochadex run binary
// with templating.
type StochadexConfigSettings struct {
	Simulation          SimulationConfigSettings              `yaml:"simulation"`
	EmbeddedSimulations []map[string]SimulationConfigSettings `yaml:"embedded_simulations,omitempty"`
}

// LoadStochadexConfigSettingsFromYaml creates a new
// StochadexConfigSettings from a provided yaml path.
func LoadStochadexConfigSettingsFromYaml(path string) *StochadexConfigSettings {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var settings StochadexConfigSettings
	err = yaml.Unmarshal(yamlFile, &settings)
	if err != nil {
		panic(err)
	}
	return &settings
}

// ImplementationsConfigFromStrings creates a single string representation of
// the Implementations config struct out of the simulator.ImplementationStrings.
func ImplementationsConfigFromStrings(
	implementationStrings simulator.ImplementationStrings,
) string {
	config := "&simulator.Implementations{"
	config += "Partitions: []simulator.Partition{"
	for _, partition := range implementationStrings.Partitions {
		config += "{Iteration: " + partition.Iteration
		config += ", ParamsFromUpstreamPartition: map[string]int{"
		for params, upstream := range partition.ParamsFromUpstreamPartition {
			config += `"` + params + `": ` + strconv.Itoa(upstream) + `,`
		}
		config += "}, ParamsFromSlice: map[string][]int{"
		for params, slice := range partition.ParamsFromSlice {
			config += `"` + params + `": []int{` + strconv.Itoa(slice[0]) +
				`, ` + strconv.Itoa(slice[1]) + `},`
		}
		config += "},}, "
	}
	config += "}, "
	config += "OutputCondition: " +
		implementationStrings.OutputCondition + ", "
	config += "OutputFunction: " +
		implementationStrings.OutputFunction + ", "
	config += "TimestepFunction: " +
		implementationStrings.TimestepFunction + ", "
	config += "TerminationCondition: " +
		implementationStrings.TerminationCondition + ","
	config += "}"
	return config
}

// WriteMainProgram writes string representations of various types of data
// to a template /tmp/*main.go file ready for runtime execution in this *main.go.
func WriteMainProgram(
	configFile string,
	config *StochadexConfigImplementationsStrings,
	dashboard *DashboardConfig,
) string {
	websocketOn := "true"
	if dashboard.Address == "" {
		websocketOn = "false"
		dashboard.Address = "dummy"
		dashboard.Handle = "dummy"
	}
	fmt.Println("\nParsed implementations:")
	fmt.Println(config.Simulation.Implementations)
	implementationsString := ImplementationsConfigFromStrings(
		config.Simulation.Implementations,
	)
	extraPackages := ""
	extraVariables := ""
	for _, extraVarsByPackage := range config.ExtraVarsByPackage {
		for extraPackage, extraVarsSlice := range extraVarsByPackage {
			if extraPackage != "" {
				extraPackages += "\"" + extraPackage + "\"" + "\n    "
			}
			for _, extraVars := range extraVarsSlice {
				for varName, varValue := range extraVars {
					extraVariables += varName + " := " + varValue + "\n    "
				}
			}
		}
	}
	if config.EmbeddedSimulations != nil {
		for i, embeddedSimulations := range config.EmbeddedSimulations {
			for varName, configStrings := range embeddedSimulations {
				extraVariables += varName + "Settings := " +
					"settingsConfig.EmbeddedSimulations[" + strconv.Itoa(i) + "]" +
					`["` + varName + `"].Settings` + "\n    "
				extraVariables += varName +
					" := simulator.NewEmbeddedSimulationRunIteration(" +
					"&" + varName + "Settings, " +
					ImplementationsConfigFromStrings(configStrings.Implementations) +
					")" + "\n    "
			}
		}
	}
	codeTemplate := template.New("stochadexMain")
	template.Must(codeTemplate.Parse(`package main

import (
	"log"
	"os"
	"time"

	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

func main() {
	settingsConfig := api.LoadStochadexConfigSettingsFromYaml("{{.SettingsFile}}")
	{{.ExtraVars}}
	implementations := {{.Implementations}}
	for index, partition := range implementations.Partitions {
		partition.Iteration.Configure(index, &settingsConfig.Simulation.Settings)
	}
	if {{.Websocket}} {
		var dashboardProcess *os.Process
		if {{.Dashboard}} {
		    dashboardProcess, err := api.StartReactApp("{{.ReactAppLocation}}")
			if err != nil {
				log.Fatal(err)
			}
			defer dashboardProcess.Signal(os.Interrupt)
		}
        api.StepAndServeWebsocket(
			&settingsConfig.Simulation.Settings,
			implementations,
			time.Duration({{.MillisecondDelay}}),
			"{{.Handle}}",
			"{{.Address}}",
		)
		if {{.Dashboard}} {
			dashboardProcess.Signal(os.Interrupt)
			dashboardProcess.Wait()
		}
	} else {
		coordinator := simulator.NewPartitionCoordinator(
			&settingsConfig.Simulation.Settings,
			implementations,
		)
		coordinator.Run()
	}
}`))
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
