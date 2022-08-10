package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"anglersim/simio"

	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

type settable interface {
	SetVec(i int, v float64)
}

type doNothing struct{}

func (d *doNothing) SetVec(i int, v float64) {}

type Sim struct {
	SimParams          *SimParams
	FishPop            *FishPop
	expDist            *distuv.Exponential
	uniDist            *distuv.Uniform
	src                rand.Source
	timeStep           float64
	cumEventProbsDense *mat.Dense
	numSpecies         int
	eventTypeLookup    []settable
}

func NewSim(simParams *SimParams, fishPop *FishPop, seed uint64) *Sim {
	numSpecies := fishPop.Params.BirthRates.Len()
	eventTypeLookup := []settable{
		fishPop.latestIncreases,
		fishPop.latestDecreases,
		&doNothing{},
	}
	ones := make([]float64, numSpecies)
	for i := range ones {
		ones[i] = 1.0
	}
	cumEventProbsDense := mat.NewDense(numSpecies, 3, nil)
	src := rand.NewSource(seed)
	s := &Sim{
		SimParams: simParams,
		FishPop:   fishPop,
		expDist: &distuv.Exponential{
			Rate: 1.0 / simParams.TimeStepScale,
			Src:  src,
		},
		uniDist: &distuv.Uniform{
			Min: 0.0,
			Max: 1.0,
			Src: src,
		},
		src:                src,
		timeStep:           0.0,
		cumEventProbsDense: cumEventProbsDense,
		numSpecies:         numSpecies,
		eventTypeLookup:    eventTypeLookup,
	}
	return s
}

func (s *Sim) genNewTimeStep() {
	// note that time units are all in years
	s.timeStep = s.expDist.Rand()
	s.FishPop.StepTime(s.timeStep)
}

func (s *Sim) genCumulativeEventProbs() *mat.Dense {
	// these are the main model engine computations of probabilities
	// loop over species here and compute the normalised cumulative probabilites
	ni := 0.0
	cumProb := 0.0
	cumProbs := mat.NewVecDense(3, nil)
	reciTimeStep := 1.0 / s.timeStep
	for i := 0; i < s.numSpecies; i++ {
		ni = s.FishPop.Counts.AtVec(i)
		// increase events (modulated by prey)
		cumProb = ni*math.Exp(s.FishPop.Params.DensDepPowers.AtVec(i)*(1.0-ni)) +
			ni*s.FishPop.Params.PredatorBirthIncRates.AtVec(i)*
				mat.Dot(
					s.FishPop.Params.PreyMatrix.RowView(i),
					s.FishPop.Counts,
				)
		cumProbs.SetVec(0, cumProb)
		// decrease events (modulated by fishing and predation)
		cumProb += ni*s.FishPop.Params.DeathRates.AtVec(i) +
			ni*s.FishPop.Params.FishingRates.AtVec(i) +
			ni*s.FishPop.Params.PredationRates.AtVec(i)*
				mat.Dot(
					s.FishPop.Params.PredatorMatrix.RowView(i),
					s.FishPop.Counts,
				)
		cumProbs.SetVec(1, cumProb)
		// do nothing events
		cumProb += reciTimeStep
		cumProbs.SetVec(2, cumProb)
		// normalise the probabilities
		cumProbs.ScaleVec(1.0/cumProb, cumProbs)
		s.cumEventProbsDense.SetRow(i, cumProbs.RawVector().Data)
	}
	return s.cumEventProbsDense
}

func (s *Sim) genEvents() {
	// only an independent event each step for each population as a whole
	s.FishPop.latestIncreases.Zero()
	s.FishPop.latestDecreases.Zero()
	cumEventProbs := s.genCumulativeEventProbs()
	for i := 0; i < s.numSpecies; i++ {
		v := s.uniDist.Rand()
		j := floats.Within(
			cumEventProbs.RawRowView(i),
			v,
		) + 1
		s.eventTypeLookup[j].SetVec(i, 1.0)
	}
}

func (s *Sim) Step() {
	s.genNewTimeStep()
	s.genEvents()
	s.FishPop.ApplyUpdates()
}

func (s *Sim) Run() {
	for s.FishPop.Time < s.SimParams.TotalRunTime {
		s.Step()
	}
}

func RunSim(simParams *SimParams, fishPop *FishPop, seed uint64) *simio.AnglersimOutput {
	var wg sync.WaitGroup
	startTime := time.Now()
	outputSpecies := []int64{}
	outputCounts := []float64{}
	for i := 0; i < simParams.NumRealisations; i++ {
		for j := 0; j < fishPop.Params.numSpecies; j++ {
			outputSpecies = append(outputSpecies, int64(j))
			outputCounts = append(outputCounts, 0.0)
		}
	}
	for i := 0; i < simParams.NumRealisations; i++ {
		wg.Add(1)
		channelCounts := make(chan []float64, fishPop.Params.numSpecies)
		go func(c chan []float64, real int) {
			defer wg.Done()
			defer close(channelCounts)
			newFishPop := NewFishPop(fishPop.Params, fishPop.Counts)
			s := NewSim(simParams, newFishPop, seed+uint64(real))
			s.Run()
			c <- s.FishPop.Counts.RawVector().Data
		}(channelCounts, i)
		counts := <-channelCounts
		for j := 0; j < fishPop.Params.numSpecies; j++ {
			outputCounts[(i*fishPop.Params.numSpecies)+j] = counts[j]
		}
	}
	wg.Wait()
	fmt.Println(time.Since(startTime))
	outputCountsMessage := &simio.AnglersimOutput{
		Species: outputSpecies,
		Counts:  outputCounts,
	}
	return outputCountsMessage
}
