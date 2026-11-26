// Package arsenal holds the cheatsheets of the original Arsenal, built into the binary.
package arsenal

import (
	"embed"
	"io/fs"
)

//go:embed arsenal-cheats
var bundle embed.FS

func Cheats() fs.FS {
	sub, err := fs.Sub(bundle, "arsenal-cheats")
	if err != nil {
		panic(err)
	}
	return sub
}
