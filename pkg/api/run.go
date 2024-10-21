package api

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StepAndServeWebsocket runs a simulation while serving a websocket with
// the simulator.WebsocketOutputFunction.
func StepAndServeWebsocket(
	generator *simulator.ConfigGenerator,
	stepDelay time.Duration,
	handle string,
	address string,
) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	http.HandleFunc(
		handle,
		func(w http.ResponseWriter, r *http.Request) {
			connection, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Println("Error upgrading to WebSocket:", err)
				return
			}
			defer connection.Close()

			var mutex sync.Mutex
			simulationConfig := generator.GetSimulation()
			simulationConfig.OutputFunction =
				simulator.NewWebsocketOutputFunction(connection, &mutex)
			generator.SetSimulation(simulationConfig)
			coordinator := simulator.NewPartitionCoordinator(
				generator.GenerateConfigs(),
			)

			var wg sync.WaitGroup
			// terminate the for loop if the condition has been met
			for !coordinator.ReadyToTerminate() {
				coordinator.Step(&wg)
				time.Sleep(stepDelay * time.Millisecond)
			}
		},
	)
	log.Fatal(http.ListenAndServe(address, nil))
}

// Run the the main run routine for the API.
func Run(config *ApiRunConfig, dashboard *DashboardConfig) {
	generator := config.GetConfigGenerator()
	withDashboard := dashboard.Address != ""
	if withDashboard {
		var dashboardProcess *os.Process
		if withDashboard {
			dashboardProcess, err := StartReactApp(dashboard.ReactAppLocation)
			if err != nil {
				log.Fatal(err)
			}
			defer dashboardProcess.Signal(os.Interrupt)
		}
		StepAndServeWebsocket(
			generator,
			time.Duration(dashboard.MillisecondDelay),
			dashboard.Handle,
			dashboard.Address,
		)
		if withDashboard {
			dashboardProcess.Signal(os.Interrupt)
			dashboardProcess.Wait()
		}
	} else {
		coordinator := simulator.NewPartitionCoordinator(
			generator.GenerateConfigs(),
		)
		coordinator.Run()
	}
}
