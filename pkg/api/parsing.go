package api

import (
	"fmt"
	"os"

	"github.com/akamensky/argparse"
	"gopkg.in/yaml.v2"
)

// ArgParse builds the configs parsed as args to the stochadex binary and
// also retrieves other args.
func ArgParse() (
	string,
	*ImplementationsConfigStrings,
	*DashboardConfig,
) {
	fmt.Println("\nReading in args...")
	parser := argparse.NewParser(
		"stochadex",
		"a simulator of stochastic phenomena",
	)
	settingsFile := parser.String(
		"s",
		"settings",
		&argparse.Options{
			Required: true,
			Help:     "yaml config path for settings",
		},
	)
	implementationsFile := parser.String(
		"i",
		"implementations",
		&argparse.Options{
			Required: true,
			Help:     "yaml config path for string implementations",
		},
	)
	dashboardFile := parser.String(
		"d",
		"dashboard",
		&argparse.Options{
			Required: false,
			Help:     "yaml config path for dashboard",
		},
	)
	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}
	if *settingsFile == "" {
		panic(fmt.Errorf("parsed no settings config file"))
	}
	if *implementationsFile == "" {
		panic(fmt.Errorf("parsed no implementations config file"))
	}
	yamlFile, err := os.ReadFile(*implementationsFile)
	if err != nil {
		panic(err)
	}
	var implementations ImplementationsConfigStrings
	err = yaml.Unmarshal(yamlFile, &implementations)
	if err != nil {
		panic(err)
	}
	dashboardConfig := DashboardConfig{}
	if *dashboardFile == "" {
		fmt.Printf("Parsed no dashboard config file: running without dashboard")
	} else {
		yamlFile, err := os.ReadFile(*dashboardFile)
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(yamlFile, &dashboardConfig)
		if err != nil {
			panic(err)
		}
	}
	return *settingsFile, &implementations, &dashboardConfig
}
