package main

import (
	"embed"
	"os"

	"github.com/pscheid92/secretli/cmd"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	if err := cmd.Run(migrationsFS); err != nil {
		os.Exit(1)
	}
}
