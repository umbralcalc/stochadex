package api

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/interactions"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StepperOrRunner defines the interface which must be implemented
// to provide a simulation which steps for serving apps with
// websockets or running simulations with other outputs.
type StepperOrRunner interface {
	Run()
	Step(wg *sync.WaitGroup)
	ReadyToTerminate() bool
}

// LoadStepperOrRunner loads the appropriate partition coordinator
// depending on whether there are agents specified in the simulation
// or not.
func LoadStepperOrRunner(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
	agents map[int]*interactions.AgentConfig,
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

// StepAndServeWebsocket runs a simulation while serving a websocket with
// the simulator.WebsocketOutputFunction.
func StepAndServeWebsocket(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
	agents map[int]*interactions.AgentConfig,
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
			stepperOrRunner := LoadStepperOrRunner(
				settings,
				implementations,
				agents,
			)

			var wg sync.WaitGroup
			// terminate the for loop if the condition has been met
			for !stepperOrRunner.ReadyToTerminate() {
				stepperOrRunner.Step(&wg)
				time.Sleep(stepDelay * time.Millisecond)
			}
		},
	)
	log.Fatal(http.ListenAndServe(address, nil))
}
