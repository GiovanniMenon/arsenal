// Command arsenal is the same binary as ars under its long name.
package main

import (
	"os"

	"github.com/ynn3m/arsenal/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
