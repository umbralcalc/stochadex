package analysis

import (
	"slices"
	"strconv"

	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// GetDataFrameFromPartition converts simulation partition data into a Gota DataFrame
// for convenient data manipulation and analysis.
//
// This function extracts time series data from a simulation partition and converts
// it into a structured DataFrame format. The resulting DataFrame has a "time"
// column followed by columns for each state dimension, making it easy to perform
// data analysis, visualization, and export operations.
//
// DataFrame Structure:
//   - Column 0: "time" - Contains the time axis values
//   - Column 1+: State dimension columns labeled by their integer indices (0, 1, 2, ...)
//
// Parameters:
//   - storage: StateTimeStorage containing the simulation data
//   - partitionName: Name of the partition to extract data from
//
// Returns:
//   - dataframe.DataFrame: Gota DataFrame with time and state columns
//
// Example:
//
//	// Extract price data from simulation storage
//	df := GetDataFrameFromPartition(storage, "prices")
//
//	// Access time column
//	timeCol := df.Col("time")
//
//	// Access state columns
//	price1Col := df.Col("0") // First price dimension
//	price2Col := df.Col("1") // Second price dimension
//
//	// Perform analysis
//	meanPrice1 := price1Col.Mean()
//	maxPrice2 := price2Col.Max()
//
// Use Cases:
//   - Data visualization and plotting
//   - Statistical analysis and computation
//   - Data export to various formats (CSV, JSON, etc.)
//   - Integration with data analysis tools
//   - Time series analysis and forecasting
//
// Performance:
//   - O(n * m) time complexity where n is number of samples, m is state dimensions
//   - Memory usage: O(n * m) for the resulting DataFrame
//   - Efficient for moderate-sized datasets (< 1M samples)
//
// Error Handling:
//   - Panics if partition name is not found in storage
//   - Provides helpful error messages with available partition names
func GetDataFrameFromPartition(
	storage *simulator.StateTimeStorage,
	partitionName string,
) dataframe.DataFrame {
	storedTimes := storage.GetTimes()
	storedValues := storage.GetValues(partitionName)
	df := dataframe.LoadMatrix(
		mat.NewDense(
			len(storedValues),
			len(storedValues[0]),
			slices.Concat(storedValues...),
		),
	)
	df = dataframe.LoadMatrix(
		mat.NewVecDense(len(storedTimes), storedTimes)).CBind(df)
	cols := []string{"time"}
	for i := range storedValues[0] {
		cols = append(cols, strconv.Itoa(i))
	}
	df.SetNames(cols...)
	return df
}

// SetPartitionFromDataFrame updates a partition's values from a Gota
// dataframe with schema [time, 0, 1, ...]. If overwriteTime is true, the
// storage's time vector is replaced with the "time" column.
func SetPartitionFromDataFrame(
	storage *simulator.StateTimeStorage,
	partitionName string,
	df dataframe.DataFrame,
	overwriteTime bool,
) {
	data := make([][]float64, 0)
	for i := range df.Nrow() {
		row := make([]float64, 0)
		for j := range df.Ncol() - 1 {
			row = append(
				row,
				df.Col(strconv.Itoa(j)).Elem(i).Float(),
			)
		}
		data = append(data, row)
	}
	storage.SetValues(partitionName, data)
	if overwriteTime {
		timeData := make([]float64, 0)
		for i := range df.Nrow() {
			timeData = append(timeData, df.Col("time").Elem(i).Float())
		}
		storage.SetTimes(timeData)
	}
}
