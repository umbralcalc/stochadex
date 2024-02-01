package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"

	"github.com/akamensky/argparse"
	"github.com/umbralcalc/stochadex/pkg/interactions"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// ImplementationStrings is the yaml-loadable config which consists of string type
// names to insert into templating.
type ImplementationStrings struct {
	Simulator          simulator.ImplementationStrings   `yaml:"simulator"`
	Agents             []interactions.AgentConfigStrings `yaml:"agents,omitempty"`
	ExtraVarsByPackage []map[string][]map[string]string  `yaml:"extra_vars_by_package,omitempty"`
}

// DashboardConfig is a yaml-loadable config for the real-time dashboard.
type DashboardConfig struct {
	Address          string `yaml:"address"`
	Handle           string `yaml:"handle"`
	MillisecondDelay uint64 `yaml:"millisecond_delay"`
	ReactAppLocation string `yaml:"react_app_location"`
	LaunchDashboard  bool   `yaml:"launch_dashboard"`
}

// StochadexArgParse builds the configs parsed as args to the stochadex binary and
// also retrieves other args.
func StochadexArgParse() (
	string,
	*ImplementationStrings,
	*DashboardConfig,
) {
	parser := argparse.NewParser("stochadex", "a simulator of stochastic phenomena")
	settingsFile := parser.String(
		"s",
		"settings",
		&argparse.Options{Required: true, Help: "yaml config path for settings"},
	)
	implementationsFile := parser.String(
		"i",
		"implementations",
		&argparse.Options{
			Required: true,
			Help:     "yaml config path for string implementations",
		},
	)
	dashboardFile := parser.String(
		"d",
		"dashboard",
		&argparse.Options{
			Required: false,
			Help:     "yaml config path for dashboard",
		},
	)
	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}
	if *settingsFile == "" {
		panic(fmt.Errorf("parsed no settings config file"))
	}
	if *implementationsFile == "" {
		panic(fmt.Errorf("parsed no implementations config file"))
	}
	yamlFile, err := os.ReadFile(*implementationsFile)
	if err != nil {
		panic(err)
	}
	var implementations ImplementationStrings
	err = yaml.Unmarshal(yamlFile, &implementations)
	if err != nil {
		panic(err)
	}
	dashboardConfig := DashboardConfig{}
	if *dashboardFile == "" {
		fmt.Printf("Parsed no dashboard config file: running without dashboard")
	} else {
		yamlFile, err := os.ReadFile(*dashboardFile)
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(yamlFile, &dashboardConfig)
		if err != nil {
			panic(err)
		}
	}
	return *settingsFile, &implementations, &dashboardConfig
}

// writeMainProgram writes string representations of various types of data
// to a template tmp/main.go file ready for runtime execution in this main.go
func writeMainProgram() string {
	fmt.Println("\nReading in args...")
	settingsFile, implementations, dashboard := StochadexArgParse()
	websocketOn := "true"
	if dashboard.Address == "" {
		websocketOn = "false"
		dashboard.Address = "dummy"
		dashboard.Handle = "dummy"
	}
	fmt.Println("\nParsed implementations:")
	fmt.Println(implementations)
	iterations := "[]simulator.Iteration{" +
		strings.Join(implementations.Simulator.Iterations, ", ") + "}"
	agents := "[]*interactions.AgentConfig{"
	for _, agentStrings := range implementations.Agents {
		agents += "{Actor: " + agentStrings.Actor
		agents += ", Generator: " + agentStrings.Generator
		agents += ", Observation: " + agentStrings.Observation + "},"
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
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/interactions"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func startDashboardApp() (*os.Process, error) {
	cmd := exec.Command("serve", "-s", "build")
	cmd.Dir = "{{.ReactAppLocation}}"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start dashboard app: %w", err)
	}

	return cmd.Process, nil
}

type StepperOrRunner interface {
	Run()
	Step(wg *sync.WaitGroup)
	ReadyToTerminate() bool
}

func LoadStepperOrRunner(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
	agents []*interactions.AgentConfig,
) StepperOrRunner {
	if len(agents) == 0 {
		return simulator.NewPartitionCoordinator(
			settings,
		    implementations,
		)
	} else {
		return interactions.NewPartitionCoordinatorWithAgents(
			settings,
			implementations,
			agents,
		)
	}
}

func main() {
	{{.ExtraVars}}
	settings := simulator.LoadSettingsFromYaml("{{.SettingsFile}}")
	iterations := {{.Iterations}}
	for partitionIndex := range settings.StateWidths {
		iterations[partitionIndex].Configure(partitionIndex, settings)
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
		    dashboardProcess, err := startDashboardApp()
			if err != nil {
				log.Fatal(err)
			}
			defer dashboardProcess.Signal(os.Interrupt)
		}
		http.HandleFunc(
			"{{.Handle}}",
			func(w http.ResponseWriter, r *http.Request) {
				connection, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					log.Println("Error upgrading to WebSocket:", err)
					return
				}
				defer connection.Close()
                
				var mutex sync.Mutex
                implementations.OutputFunction =
					simulator.NewWebsocketOutputFunction(connection, &mutex)
				stepperOrRunner := LoadStepperOrRunner(settings, implementations, agents)

				var wg sync.WaitGroup
				// terminate the for loop if the condition has been met
				for !stepperOrRunner.ReadyToTerminate() {
					stepperOrRunner.Step(&wg)
					time.Sleep({{.MillisecondDelay}} * time.Millisecond)
				}
			},
		)
		log.Fatal(http.ListenAndServe("{{.Address}}", nil))
		if {{.Dashboard}} {
			dashboardProcess.Signal(os.Interrupt)
			dashboardProcess.Wait()
		}
	} else {
		stepperOrRunner := LoadStepperOrRunner(settings, implementations, agents)
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

func main() {
	// hydrate the template code and write it to tmp/main.go
	fileName := writeMainProgram()
	defer os.Remove(fileName)

	// execute the code
	runCmd := exec.Command("go", "run", fileName)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		panic(err)
	}
}
