// Command ars is the Arsenal binary.
package main

import (
	"os"

	"github.com/ynn3m/arsenal/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
