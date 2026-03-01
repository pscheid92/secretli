package main

import (
	"os"

	"github.com/pscheid92/secretli/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
