// Package cli has the cobra commands of ars.
// This file builds the root command, the help groups and what runs before every command.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

var Version = "dev"

var (
	isInteractive = tui.IsInteractive
	askText       = tui.AskText
	confirm       = tui.Confirm
)

func version() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return Version
}

func binaryName() string {
	switch n := filepath.Base(os.Args[0]); n {
	case "ars", "arsenal":
		return n
	}
	return "ars"
}

func newRootCmd(name string) *cobra.Command {
	flags := &searchFlags{}
	root := &cobra.Command{
		Use:   name,
		Short: "Search pentest commands and put one in your prompt",
		Long: "Search a library of annotated pentest commands, fill in your variables (target, ports, ...) and get the finished command in your shell prompt.\n" +
			"Sessions keep your variables, notes and command log per engagement.",
		Example: "  ars                       # open the search\n" +
			"  ars -s nmap               # start with a search\n" +
			"  ars init acme             # start an engagement\n" +
			"  ars set target=10.10.10.5 # save a variable that fills commands",
		Version:           version(),
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.NoArgs,
		PersistentPreRunE: preRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, flags)
		},
	}
	root.Flags().StringVarP(&flags.query, "search", "s", "", "start with `QUERY` in the search box")
	root.Flags().BoolVarP(&flags.prefix, "prefix", "f", false, "add your prefix command ("+cheats.PrefixVar+") in front")
	root.Flags().BoolVar(&flags.noTags, "no-tags", false, "hide the tags column")
	root.Flags().BoolVarP(&flags.print, "print", "p", false, "print the command to stdout")
	root.Flags().StringVarP(&flags.outfile, "outfile", "o", "", "write the command to `FILE`")
	root.Flags().BoolVarP(&flags.copy, "copy", "x", false, "copy the command to the clipboard")
	root.Flags().BoolVarP(&flags.exec, "exec", "e", false, "run the command instead of putting it in the prompt")
	root.Flags().BoolVarP(&flags.tmux, "tmux", "t", false, "send the command to another tmux pane (with -e, run it there)")
	root.Flags().BoolVarP(&flags.check, "check", "c", false, "check which programs used by the cheatsheets are installed")
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpCommand(&cobra.Command{Hidden: true, GroupID: "setup"})
	root.Flags().BoolP("version", "V", false, "print the version")
	root.SetVersionTemplate(versionTemplate())
	root.AddCommand(newPauseCmd())
	root.AddCommand(
		newInitCmd(),
		newSessionsCmd(),
		newSetCmd(),
		newUnsetCmd(),
		newVarsCmd(),
		newSetupCmd(),
		newConfigCmd(),
		newHookCmd(),
		newRecCmd(),
		newStatusCmd(),
		newReplayCmd(),
		newHistoryCmd(),
		newAddCmd(),
		newNoteCmd(),
		newNotesCmd(),
	)
	groupCommands(root)
	return root
}

var commandGroups = []struct {
	id, title string
	names     []string
}{
	{"sessions", "Sessions:", []string{"init", "sessions"}},
	{"vars", "Variables:", []string{"set", "unset", "vars"}},
	{"cheats", "Cheatsheets and notes:", []string{"add", "note", "notes"}},
	{"rec", "Command log and recording:", []string{"rec", "status", "pause", "replay", "history"}},
	{"setup", "Setup:", []string{"setup", "config"}},
}

func groupCommands(root *cobra.Command) {
	for _, g := range commandGroups {
		root.AddGroup(&cobra.Group{ID: g.id, Title: g.title})
	}
	for _, c := range root.Commands() {
		for _, g := range commandGroups {
			for _, n := range g.names {
				if c.Name() == n {
					c.GroupID = g.id
				}
			}
		}
	}
}

func preRun(cmd *cobra.Command, _ []string) error {
	migratedNow = nil
	switch cmd.Name() {
	case "hook", "setup":
		return nil
	}
	errOut := cmd.ErrOrStderr()
	if notice := config.OtherConfigNotice(); notice != "" {
		fmt.Fprintln(errOut, tui.Warn(notice))
	}
	wd, _ := os.Getwd()
	sessMoved, err := session.Migrate(wd)
	migratedNow = session.MigratedNames(sessMoved)
	for _, m := range sessMoved {
		fmt.Fprintln(errOut, tui.Muted(m))
	}
	if len(sessMoved) > 0 {
		fmt.Fprintln(errOut, tui.Muted("A session's data is in its .arsenal folder; the rest of the session folder is yours."))
	}
	if err != nil {
		return err
	}
	exists, err := config.FileExists()
	if err != nil || exists || !isInteractive() {
		return err
	}
	fmt.Fprintln(errOut, tui.Muted("First run: setting up Arsenal. Run ars setup to change these answers later."))
	fmt.Fprintln(errOut)
	return runSetup(errOut)
}

var migratedNow map[string]bool

func Execute() int {
	return execute(newRootCmd(binaryName()), os.Stderr)
}

func execute(root *cobra.Command, errOut io.Writer) int {
	if err := root.Execute(); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			return ee.code
		}
		if errors.Is(err, tui.ErrCancelled) {
			fmt.Fprintln(errOut, tui.Muted("Cancelled."))
			return 130
		}
		fmt.Fprintln(errOut, tui.Error(err.Error()))
		return 1
	}
	return 0
}

func requireSession() (session.Info, error) { return session.Require() }

func currentSession() (session.Info, bool, error) { return session.Current() }
