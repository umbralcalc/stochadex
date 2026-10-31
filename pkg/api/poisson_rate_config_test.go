package api

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// poissonRateSums reads cfg/example_poisson_rate_data.csv's columns (goals,
// cards, penalties, minutes) and returns their running sums, row by row.
func poissonRateSums(t *testing.T) [][4]float64 {
	t.Helper()
	file, err := os.Open("cfg/example_poisson_rate_data.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var sums [][4]float64
	var running [4]float64
	for _, record := range records[1:] {
		for i := range running {
			value, err := strconv.ParseFloat(record[i+1], 64)
			if err != nil {
				t.Fatal(err)
			}
			running[i] += value
		}
		sums = append(sums, running)
	}
	return sums
}

// TestPoissonRateConfig is the fidelity test for cfg/example_poisson_rate_config.yaml:
// the rates it estimates are the closed-form maximum likelihood rates of the
// CSV it reads, total count over total exposure with every row counted, and
// its one output writes them once.
func TestPoissonRateConfig(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	const path = "cfg/example_poisson_rate_config.yaml"
	sums := poissonRateSums(t)

	t.Run("the rates are the closed-form sums over every row so far", func(t *testing.T) {
		config, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		rates := result.Storage.GetValues("rates")
		if len(rates) != len(sums) {
			t.Fatalf("%d rows of rates, want one per CSV row, %d", len(rates), len(sums))
		}
		// The first row is the replay's initial state, which the rates do not
		// yet read (they start at their init_state_values); every later row's
		// rates cover every row up to it, the first included.
		for row := 1; row < len(sums); row++ {
			for k := range 3 {
				want := sums[row][k] / sums[row][3]
				if got := rates[row][k]; math.Abs(got-want) > 1e-12*math.Max(want, 1e-3) {
					t.Fatalf("row %d: rate %d = %v, want the closed form %v", row, k, got, want)
				}
			}
		}
		// The generating rates are 0.030, 0.042 and 0.0032 per minute; with
		// ~86,000 minutes each estimate is within three standard errors.
		final := rates[len(rates)-1]
		for k, rate := range []float64{0.030, 0.042, 0.0032} {
			if se := math.Sqrt(rate / sums[len(sums)-1][3]); math.Abs(final[k]-rate) > 3*se {
				t.Errorf("rate %d = %v, more than 3 standard errors (%v) from %v", k, final[k], se, rate)
			}
		}
	})

	t.Run("the final_step output writes the rates once", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "estimate.log")
		config, err := LoadConfig(path, WithSet("outputs[name=estimate].function",
			`{type: json_log, path: "`+log+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunWith(config, WithConfigOutputs()); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(log)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		var lines []map[string]interface{}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var line map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, line)
		}
		if len(lines) != 1 || lines[0]["partition_name"] != "rates" ||
			lines[0]["time"] != float64(len(sums)) {
			t.Fatalf("the output wrote %v, want one line: rates at time %d", lines, len(sums))
		}
		last := sums[len(sums)-1]
		for k, value := range lines[0]["state"].([]interface{}) {
			if want := last[k] / last[3]; math.Abs(value.(float64)-want) > 1e-12*want {
				t.Errorf("written rate %d = %v, want %v", k, value, want)
			}
		}
	})
}
