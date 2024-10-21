package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
)

func main() {
	api.RunWithParsedArgs(api.ArgParse())
}
