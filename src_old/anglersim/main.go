package main

import (
	"anglersim/simio"
	"io/ioutil"
	"log"

	"google.golang.org/protobuf/proto"
)

func main() {
	in, err := ioutil.ReadFile("input/anglersim.data")
	if err != nil {
		log.Fatalln("Error reading anglersim input data:", err)
	}
	inputData := &simio.AnglersimInput{}
	if err := proto.Unmarshal(in, inputData); err != nil {
		log.Fatalln("Failed to parse anglersim input:", err)
	}
	outputData := initialiseAndSimulate(inputData)
	out, err := proto.Marshal(outputData)
	if err != nil {
		log.Fatalln("Failed to encode anglersim output:", err)
	}
	if err := ioutil.WriteFile("output/anglersim.data", out, 0644); err != nil {
		log.Fatalln("Failed to write anglersim output:", err)
	}
}

func initialiseAndSimulate(inputData *simio.AnglersimInput) *simio.AnglersimOutput {
	fishPop := NewFishPopFromInput(inputData)
	simParams := &SimParams{
		TimeStepScale:   inputData.TimeScale,
		TotalRunTime:    inputData.RunTime,
		NumRealisations: int(inputData.NumReals),
	}
	return RunSim(simParams, fishPop, uint64(inputData.Seed))
}
