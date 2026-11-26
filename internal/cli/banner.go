// This file draws the art banner printed by ars --version.
package cli

import (
	"strings"

	"github.com/ynn3m/arsenal/internal/tui"
)

const banner = ` ▀▀▀░░▄                                         ░░░
███ ▒▒▒                                         ░░░
░░░ ▓▓▓                                         ▓▓▓
▒▒▒▄▓▓▓ ▄▒░▀░▓▄ ▄▒░▀░▒▄ ▄█▒▀█░▄ █▒░▀░▒▄ ▄█▒▀█░▄ ░▒▓
▓▓▓ ▒▒▒ ▒░░ ▓██ ▒░▒ ▓██ ▒░░ ░▒▓ ▒░░ ▒▓█ ▒░█ ▒▓█ ▒▓█
███ ░░░ ░▒▓     ▀▒▓▄▄▄  ░░▒▀▀▀▀ ░░▒ ▓██  ▄▄▄▓██ ▓██
▓▓▓ ░░░ ▒▓█     ▄▄▄ ███ ▒▓█ ███ ░▒▓ ███ ███ ███ ███ ▄▄▄
▒▒▒ ▒▒▒ ▓█▓     ▒▓▓ ███ ▓██ ▓██ ▒▓█ ▓██ ███ ██▓ ██▓ ███
░░░ ▓▓▓ █▓▒     ▀▓▒▄▓█▀ ▀█▒▄▒▓▀ ▓██ ▒▓█ ▀██▄█▓▒ ▀▓▒▄▓█▀
         ▀░       ░▀      ░▀        ░▀       ▀░   ▀░
          ▄       ▄       ▄         ▄         ▄    ▄`

func versionTemplate() string {
	var b strings.Builder
	for _, l := range strings.Split(banner, "\n") {
		b.WriteString(tui.Title(l))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString("{{.Name}} {{.Version}}\n")
	b.WriteString(tui.Muted("Search pentest commands and put one in your prompt") + "\n")
	return b.String()
}
