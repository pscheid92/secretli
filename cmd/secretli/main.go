// Command secretli shares and opens secrets from the terminal, speaking the
// same encrypted format as the web app, so links work in both directions.
package main

import (
	"os"

	"github.com/pscheid92/secretli/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
