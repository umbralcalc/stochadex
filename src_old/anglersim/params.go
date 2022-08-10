package main

import (
	"container/list"

	"gonum.org/v1/gonum/mat"
)

type SimParams struct {
	TimeStepScale   float64
	TotalRunTime    float64
	NumRealisations int
}

// population parameters to setup the simulation
type PopParams struct {
	SpeciesNames          *list.List
	DensDepPowers         *mat.VecDense
	BirthRates            *mat.VecDense
	DeathRates            *mat.VecDense
	PredationRates        *mat.VecDense
	PredatorBirthIncRates *mat.VecDense
	FishingRates          *mat.VecDense
	PredatorMatrix        *mat.Dense
	PreyMatrix            *mat.Dense
	numSpecies            int
}

func NewPopParams(
	BirthRates *mat.VecDense,
	DensDepPowers *mat.VecDense,
	DeathRates *mat.VecDense,
	PredationRates *mat.VecDense,
	PredatorBirthIncRates *mat.VecDense,
	FishingRates *mat.VecDense,
	PredatorMatrix *mat.Dense,
	PreyMatrix *mat.Dense,
) *PopParams {
	numSpecies := BirthRates.Len()
	p := &PopParams{
		BirthRates:            BirthRates,
		DensDepPowers:         DensDepPowers,
		DeathRates:            DeathRates,
		PredationRates:        PredationRates,
		PredatorBirthIncRates: PredatorBirthIncRates,
		FishingRates:          FishingRates,
		PredatorMatrix:        PredatorMatrix,
		PreyMatrix:            PreyMatrix,
		numSpecies:            numSpecies,
	}
	return p
}
