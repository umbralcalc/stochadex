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
	settings *simulator.Settings,
	implementations *simulator.Implementations,
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
			implementations.OutputFunction =
				simulator.NewWebsocketOutputFunction(connection, &mutex)
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
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
func Run(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
	withWebsocket bool,
	withDashboard bool,
	millisecondDelay uint64,
	reactAppLocation string,
	handle string,
	address string,
) {
	if withWebsocket {
		var dashboardProcess *os.Process
		if withDashboard {
			dashboardProcess, err := StartReactApp(reactAppLocation)
			if err != nil {
				log.Fatal(err)
			}
			defer dashboardProcess.Signal(os.Interrupt)
		}
		StepAndServeWebsocket(
			settings,
			implementations,
			time.Duration(millisecondDelay),
			handle,
			address,
		)
		if withDashboard {
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
}
