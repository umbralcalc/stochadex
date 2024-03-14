package api

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"text/template"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ImplementationsConfigStrings is the yaml-loadable config which consists of
// string type names to insert into templating.
type ImplementationsConfigStrings struct {
	Simulator          simulator.ImplementationStrings  `yaml:"simulator"`
	ExtraVarsByPackage []map[string][]map[string]string `yaml:"extra_vars_by_package,omitempty"`
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
	partitions := "[]simulator.Partition{"
	for _, partition := range implementations.Simulator.Partitions {
		partitions += "{ Iteration: " + partition.Iteration
		partitions += ", ParamsByUpstreamPartition: map[int]string{"
		for upstream, params := range partition.ParamsByUpstreamPartition {
			partitions += strconv.Itoa(upstream) + `: "` + params + `",`
		}
		partitions += "},}, "
	}
	partitions += "}"
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
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

func main() {
	{{.ExtraVars}}
	settings := simulator.LoadSettingsFromYaml("{{.SettingsFile}}")
	partitions := {{.Partitions}}
	for index, partition := range partitions {
		partition.Iteration.Configure(index, settings)
	}
	implementations := &simulator.Implementations{
		Partitions:      partitions,
		OutputCondition: {{.OutputCondition}},
		OutputFunction:  {{.OutputFunction}},
		TerminationCondition: {{.TerminationCondition}},
		TimestepFunction: {{.TimestepFunction}},
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
			settings,
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
			settings,
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
			"SettingsFile":         settingsFile,
			"Dashboard":            strconv.FormatBool(dashboard.LaunchDashboard),
			"Websocket":            websocketOn,
			"Address":              dashboard.Address,
			"Handle":               dashboard.Handle,
			"ReactAppLocation":     dashboard.ReactAppLocation,
			"MillisecondDelay":     strconv.Itoa(int(dashboard.MillisecondDelay)),
			"Partitions":           partitions,
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
