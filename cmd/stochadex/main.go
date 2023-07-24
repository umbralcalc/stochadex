package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"

	"github.com/akamensky/argparse"
	"gopkg.in/yaml.v2"
)

// ImplementationStrings is the yaml-loadable config which consists of string type
// names to insert into templating.
type ImplementationStrings struct {
	Iterations           []string `yaml:"iterations"`
	OutputCondition      string   `yaml:"output_condition"`
	OutputFunction       string   `yaml:"output_function"`
	TerminationCondition string   `yaml:"termination_condition"`
	TimestepFunction     string   `yaml:"timestep_function"`
}

// DashboardConfig is a yaml-loadable config for the real-time dashboard.
type DashboardConfig struct {
	Address          string `yaml:"address"`
	Handle           string `yaml:"handle"`
	MillisecondDelay uint64 `yaml:"millisecond_delay"`
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
			Help:     "yaml config path for string implementations",
		},
	)
	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}
	if *settingsFile == "" {
		panic(fmt.Errorf("Parsed no settings config file"))
	}
	if *implementationsFile == "" {
		panic(fmt.Errorf("Parsed no implementations config file"))
	}
	if err != nil {
		fmt.Print(parser.Usage(err))
	}
	yamlFile, err := ioutil.ReadFile(*implementationsFile)
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
		yamlFile, err := ioutil.ReadFile(*dashboardFile)
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
func writeMainProgram() {
	fmt.Println("\nReading in args...")
	settingsFile, implementations, dashboard := StochadexArgParse()
	dashboardOn := "true"
	if dashboard.Address == "" {
		dashboardOn = "false"
		dashboard.Address = "dummy"
		dashboard.Handle = "dummy"
	}
	fmt.Println("\nParsed implementations:")
	fmt.Println(implementations)
	iterations := "[]simulator.Iteration{" +
		strings.Join(implementations.Iterations, ", ") + "}"
	codeTemplate := template.New("stochadexMain")
	template.Must(codeTemplate.Parse(`package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	settings := simulator.NewLoadSettingsConfigFromYaml("{{.SettingsFile}}")
	iterations := {{.Iterations}}
	for partitionIndex := range settings.StateWidths {
		iterations[partitionIndex].Configure(partitionIndex, settings)
	}
	implementations := &simulator.LoadImplementationsConfig{
		Iterations:      iterations,
		OutputCondition: {{.OutputCondition}},
		OutputFunction:  {{.OutputFunction}},
		TerminationCondition: {{.TerminationCondition}},
		TimestepFunction: {{.TimestepFunction}},
	}
	config := simulator.NewStochadexConfig(
		settings,
		implementations,
	)
	if {{.Dashboard}} {
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
                config.Output.Function =
					simulator.NewWebsocketOutputFunction(connection, &mutex)
				coordinator := simulator.NewPartitionCoordinator(config)
				
				var wg sync.WaitGroup
				// terminate the for loop if the condition has been met
				for !coordinator.ReadyToTerminate() {
					coordinator.Step(&wg)
					time.Sleep({{.MillisecondDelay}} * time.Millisecond)
				}
			},
		)
		log.Fatal(http.ListenAndServe("{{.Address}}", nil))
	} else {
		coordinator := simulator.NewPartitionCoordinator(config)
		coordinator.Run()
	}
}`))
	file, err := os.Create("tmp/main.go")
	if err != nil {
		err := os.Mkdir("tmp", 0755)
		if err != nil {
			panic(err)
		}
		file, err = os.Create("tmp/main.go")
	}
	err = codeTemplate.Execute(
		file,
		map[string]string{
			"SettingsFile":         settingsFile,
			"Dashboard":            dashboardOn,
			"Address":              dashboard.Address,
			"Handle":               dashboard.Handle,
			"MillisecondDelay":     strconv.Itoa(int(dashboard.MillisecondDelay)),
			"Iterations":           iterations,
			"OutputCondition":      implementations.OutputCondition,
			"OutputFunction":       implementations.OutputFunction,
			"TerminationCondition": implementations.TerminationCondition,
			"TimestepFunction":     implementations.TimestepFunction,
		},
	)
	if err != nil {
		panic(err)
	}
	file.Close()
}

func main() {
	// hydrate the template code and write it to tmp/main.go
	writeMainProgram()

	// execute the code
	runCmd := exec.Command("go", "run", "tmp/main.go")
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		panic(err)
	}
}
