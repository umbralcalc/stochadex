package main

import (
	"anglersim/simio"

	"gonum.org/v1/gonum/mat"
)

type FishPop struct {
	Counts          *mat.VecDense
	Time            float64
	Params          *PopParams
	latestIncreases *mat.VecDense
	latestDecreases *mat.VecDense
}

func NewFishPop(p *PopParams, initCounts *mat.VecDense) *FishPop {
	f := &FishPop{
		Counts:          initCounts,
		Time:            0.0,
		Params:          p,
		latestIncreases: mat.NewVecDense(p.numSpecies, nil),
		latestDecreases: mat.NewVecDense(p.numSpecies, nil),
	}
	return f
}

func NewFishPopFromInput(inputData *simio.AnglersimInput) *FishPop {
	n := int(inputData.NumSpecies)
	initCounts := mat.NewVecDense(n, nil)
	densDepPowers := mat.NewVecDense(n, nil)
	birthRates := mat.NewVecDense(n, nil)
	deathRates := mat.NewVecDense(n, nil)
	predationRates := mat.NewVecDense(n, nil)
	predatorBirthIncRates := mat.NewVecDense(n, nil)
	fishingRates := mat.NewVecDense(n, nil)
	predatorMatrix := mat.NewDense(n, n, nil)
	preyMatrix := mat.NewDense(n, n, nil)
	inputFishPops := inputData.GetFishPops()
	for i := 0; i < n; i++ {
		initCounts.SetVec(i, inputFishPops[i].GetInitCount())
		densDepPowers.SetVec(i, inputFishPops[i].GetDensDepPower())
		birthRates.SetVec(i, inputFishPops[i].GetBirthRate())
		deathRates.SetVec(i, inputFishPops[i].GetDeathRate())
		predationRates.SetVec(i, inputFishPops[i].GetPredationRate())
		predatorBirthIncRates.SetVec(i, inputFishPops[i].GetPredatorBirthIncRate())
		fishingRates.SetVec(i, inputFishPops[i].GetFishingRate())
		predatorMatrixRow := inputFishPops[i].GetPredatorMatrixRow()
		preyMatrixRow := inputFishPops[i].GetPreyMatrixRow()
		for j := 0; j < n; j++ {
			predatorMatrix.Set(i, j, predatorMatrixRow[j])
			preyMatrix.Set(i, j, preyMatrixRow[j])
		}
	}
	params := NewPopParams(
		birthRates,
		densDepPowers,
		deathRates,
		predationRates,
		predatorBirthIncRates,
		fishingRates,
		predatorMatrix,
		preyMatrix,
	)
	return NewFishPop(params, initCounts)
}

func (f *FishPop) StepTime(timeStep float64) {
	f.Time += timeStep
}

func (f *FishPop) ApplyUpdates() {
	f.Counts.SubVec(f.Counts, f.latestDecreases)
	f.Counts.AddVec(f.Counts, f.latestIncreases)
}
