package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
)

func main() {
	settingsFile, implementations, dashboard := api.ArgParse()
	api.RunWithParsedArgs(settingsFile, implementations, dashboard)
}
