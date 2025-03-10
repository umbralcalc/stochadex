package api

import (
	"log"
	"net/http"
	"os"
	"os/exec"
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
func Run(config *ApiRunConfig, socket *SocketConfig) {
	generator := config.GetConfigGenerator()
	activeSocket := socket.Active()
	if activeSocket {
		StepAndServeWebsocket(
			generator,
			time.Duration(socket.MillisecondDelay),
			socket.Handle,
			socket.Address,
		)
	} else {
		coordinator := simulator.NewPartitionCoordinator(
			generator.GenerateConfigs(),
		)
		coordinator.Run()
	}
}

// RunWithParsedArgs takes in the outputs from ArgParse and runs the
// stochadex with these configurations.
func RunWithParsedArgs(args ParsedArgs) {
	// hydrate the template code and write it to a /tmp/*main.go
	fileName := WriteMainProgram(args)
	defer os.Remove(fileName)

	// execute the code
	runCmd := exec.Command("go", "run", fileName)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		panic(err)
	}
}
