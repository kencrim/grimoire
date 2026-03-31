package main

import (
	"os"

	"github.com/kencrim/grimoire/apps/wt/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
