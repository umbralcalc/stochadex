package api

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"

	"github.com/umbralcalc/stochadex/pkg/interactions"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ImplementationsConfigStrings is the yaml-loadable config which consists of
// string type names to insert into templating.
type ImplementationsConfigStrings struct {
	Simulator          simulator.ImplementationStrings         `yaml:"simulator"`
	AgentByPartition   map[int]interactions.AgentConfigStrings `yaml:"agent_by_partition,omitempty"`
	ExtraVarsByPackage []map[string][]map[string]string        `yaml:"extra_vars_by_package,omitempty"`
}

// WriteMainProgram writes string representations of various types of data
// to a template /tmp/*main.go file ready for runtime execution in this *main.go.
func WriteMainProgram(
	settingsFile string,
	implementations *ImplementationsConfigStrings,
	dashboard *DashboardConfig,
) string {
	websocketOn := "true"
	if dashboard.Address == "" {
		websocketOn = "false"
		dashboard.Address = "dummy"
		dashboard.Handle = "dummy"
	}
	fmt.Println("\nParsed implementations:")
	fmt.Println(implementations)
	iterations := "[][]simulator.Iteration{"
	for _, serialIterations := range implementations.Simulator.Iterations {
		iterations += "{" + strings.Join(serialIterations, ", ") + "}, "
	}
	iterations += "}"
	agents := "map[int]*interactions.AgentConfig{"
	for i, agentStrings := range implementations.AgentByPartition {
		agents += strconv.Itoa(i) + ": "
		agents += "{Actor: " + agentStrings.Actor
		agents += ", GeneratorPartition: " +
			strconv.Itoa(agentStrings.GeneratorPartition) + "},"
	}
	agents += "}"
	extraPackages := ""
	extraVariables := ""
	for _, extraVarsByPackage := range implementations.ExtraVarsByPackage {
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
	codeTemplate := template.New("stochadexMain")
	template.Must(codeTemplate.Parse(`package main

import (
	"log"
	"os"
	"time"

	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/interactions"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

func main() {
	{{.ExtraVars}}
	settings := simulator.LoadSettingsFromYaml("{{.SettingsFile}}")
	iterations := {{.Iterations}}
	index := 0
	for _, serialPartitions := range iterations {
		for _, iteration := range serialPartitions {
			iteration.Configure(index, settings)
			index += 1
		}
	}
	implementations := &simulator.Implementations{
		Iterations:      iterations,
		OutputCondition: {{.OutputCondition}},
		OutputFunction:  {{.OutputFunction}},
		TerminationCondition: {{.TerminationCondition}},
		TimestepFunction: {{.TimestepFunction}},
	}
	agents := {{.Agents}}
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
			settings,
			implementations,
			agents,
			time.Duration({{.MillisecondDelay}}),
			"{{.Handle}}",
			"{{.Address}}",
		)
		if {{.Dashboard}} {
			dashboardProcess.Signal(os.Interrupt)
			dashboardProcess.Wait()
		}
	} else {
		stepperOrRunner := api.LoadStepperOrRunner(
			settings, 
			implementations, 
			agents,
		)
		stepperOrRunner.Run()
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
			"SettingsFile":         settingsFile,
			"Dashboard":            strconv.FormatBool(dashboard.LaunchDashboard),
			"Websocket":            websocketOn,
			"Address":              dashboard.Address,
			"Handle":               dashboard.Handle,
			"ReactAppLocation":     dashboard.ReactAppLocation,
			"MillisecondDelay":     strconv.Itoa(int(dashboard.MillisecondDelay)),
			"Iterations":           iterations,
			"Agents":               agents,
			"ExtraVars":            extraVariables,
			"ExtraPackages":        extraPackages,
			"OutputCondition":      implementations.Simulator.OutputCondition,
			"OutputFunction":       implementations.Simulator.OutputFunction,
			"TerminationCondition": implementations.Simulator.TerminationCondition,
			"TimestepFunction":     implementations.Simulator.TimestepFunction,
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
	settingsFile string,
	implementations *ImplementationsConfigStrings,
	dashboard *DashboardConfig,
) {
	// hydrate the template code and write it to a /tmp/*main.go
	fileName := WriteMainProgram(
		settingsFile,
		implementations,
		dashboard,
	)
	defer os.Remove(fileName)

	// execute the code
	runCmd := exec.Command("go", "run", fileName)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		panic(err)
	}
}