package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/akamensky/argparse"
	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// DashboardConfig is the yaml-loadable config which defines all of
// the settings for running the real-time dashboard.
type DashboardConfig struct {
	LoadSettingsConfig string `yaml:"load_settings_config"`
	NumberOfStepsToRun int    `yaml:"number_of_steps_to_run"`
}

// NewDashboardConfigFromArgParse builds a dashboard config struct from
// an arg-parsed yaml file.
func NewDashboardConfigFromArgParse() *DashboardConfig {
	parser := argparse.NewParser(
		"stochadex dashboard",
		"simulates your chosen stochastic process and displays it in a real-time dashboard",
	)
	s := parser.String(
		"c",
		"config",
		&argparse.Options{Required: true, Help: "yaml config path for settings"},
	)
	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}
	yamlFile, err := ioutil.ReadFile(*s)
	if err != nil {
		panic(err)
	}
	var config DashboardConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	return &config
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	config := NewDashboardConfigFromArgParse()
	http.HandleFunc(
		"/dashboard",
		func(w http.ResponseWriter, r *http.Request) {
			connection, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Println("Error upgrading to WebSocket:", err)
				return
			}
			defer connection.Close()

			settings := simulator.NewLoadSettingsConfigFromYaml(config.LoadSettingsConfig)
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.StateWidths {
				iteration := &phenomena.WienerProcessIteration{}
				iteration.Configure(partitionIndex, settings)
				iterations = append(iterations, iteration)
			}
			implementations := &simulator.LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  simulator.NewWebsocketOutputFunction(connection),
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: config.NumberOfStepsToRun,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			config := simulator.NewStochadexConfig(
				settings,
				implementations,
			)
			coordinator := simulator.NewPartitionCoordinator(config)
			coordinator.Run()
		},
	)
	log.Fatal(http.ListenAndServe(":2112", nil))
}
