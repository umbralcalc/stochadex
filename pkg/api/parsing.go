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
	*StochadexConfigImplementationsStrings,
	*DashboardConfig,
) {
	fmt.Println("\nReading in args...")
	parser := argparse.NewParser(
		"stochadex",
		"A generalised simulation engine",
	)
	configFile := parser.String(
		"c",
		"config",
		&argparse.Options{
			Required: true,
			Help:     "yaml config path",
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
	if *configFile == "" {
		panic(fmt.Errorf("parsed no config file"))
	}
	yamlFile, err := os.ReadFile(*configFile)
	if err != nil {
		panic(err)
	}
	var implementations StochadexConfigImplementationsStrings
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
	return *configFile, &implementations, &dashboardConfig
}
