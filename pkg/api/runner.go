package api

import (
	"log"
	"net/http"
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
