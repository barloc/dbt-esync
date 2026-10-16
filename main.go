package main

import (
	"os"

	"github.com/barloc/dbt-esync/app"
)

func main() {
	if err := app.NewCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
