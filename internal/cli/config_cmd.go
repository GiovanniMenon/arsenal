// ars config prints where the config, the cheats and the pentest data live.

package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show where Arsenal keeps its files",
		Long: "Prints the config file, the cheatsheet folders, the pentest folder and the current session.\n" +
			"Edit config.yml to change settings.",
		Example: "  ars config                    # all paths\n" +
			"  ars config | grep -i cheat    # just the cheatsheet folders",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printConfig(cmd.OutOrStdout())
		},
	}
	return cmd
}

func mark(p string) string {
	if _, err := os.Stat(p); err != nil {
		return tui.Muted(" (missing)")
	}
	return ""
}

func printConfig(out io.Writer) error {
	if err := printConfigRows(out); err != nil {
		return err
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Edit "+tui.KeyStyle.Render("config.yml")+" to change settings.")
	return nil
}

func printConfigRows(out io.Writer) error {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	cfgFile, _ := config.FilePath()
	varsFile, _ := config.GlobalVarsPath()
	cheatsDir, _ := config.CheatsDir()
	paths, err := config.CheatsPaths()
	if err != nil {
		return err
	}
	pentest, err := config.PentestPath()
	if err != nil {
		return err
	}
	source := "default"
	if os.Getenv(config.EnvPentestPath) != "" {
		source = "$PENTEST_PATH"
	} else if f, err := config.LoadFile(); err == nil && f.PentestPath != "" {
		source = "config.yml"
	}

	row := func(label, value string) {
		if label != "" {
			label += ":"
		}
		fmt.Fprintf(out, "%-22s %s\n", label, value)
	}
	row("Config dir", tildePath(cfgDir))
	row("Config file", tildePath(cfgFile)+mark(cfgFile))
	row("Global vars", tildePath(varsFile)+mark(varsFile))
	row("Cheatsheet folder", tildePath(cheatsDir)+mark(cheatsDir)+tui.Muted("  (ars setup and ars add save here)"))
	for i, p := range paths {
		label := ""
		if i == 0 {
			label = "Reads cheatsheets from"
		}
		row(label, tildePath(p)+mark(p))
	}
	row("Pentest path", tildePath(pentest)+tui.Muted("  ("+source+")")+mark(pentest))

	sess, ok, err := session.Current()
	if err != nil {
		return err
	}
	if !ok {
		row("Session", tui.Muted("none (run ars init <name> to create one)"))
		return nil
	}
	row("Session", sess.Name+tui.Muted("  ("+tildePath(sess.Dir)+")"))
	meta := session.MetaDir(sess.Dir)
	row("Session data", tildePath(meta))
	return nil
}
