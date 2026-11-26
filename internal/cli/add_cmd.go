// ars add saves a command to the user's cheatsheets.

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/history"
	"github.com/ynn3m/arsenal/internal/tui"
)

type addFlags struct {
	command, title, description, tags, file string
	last                                    bool
}

func newAddCmd() *cobra.Command {
	f := &addFlags{}
	cmd := &cobra.Command{
		Use:   "add [command]",
		Short: "Save a command to your cheatsheets",
		Long: "Adds a command to Arsenal. Write <name> for the parts that change, like <target> or <ports|22,80>,\n" +
			"and they are filled from your variables when you pick the command.",
		Example: "  ars add                                      # asks for everything\n" +
			"  ars add \"nmap -sV -p <ports|22,80> <target>\"  # start from a command\n" +
			"  ars add --last --title \"scan all ports\"      # no questions, last shell command\n" +
			"  ars add \"nmap -sV <target>\" --title \"service scan\"",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				f.command = strings.Join(args, " ")
			}
			return runAdd(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.title, "title", "", "title (with a command, nothing is asked)")
	cmd.Flags().StringVar(&f.description, "description", "", "description")
	cmd.Flags().StringVar(&f.tags, "tags", "", "tags, comma-separated")
	cmd.Flags().StringVar(&f.file, "file", "", "save in this cheatsheet `FILE` (.md) instead of <tool>.md")
	cmd.Flags().BoolVar(&f.last, "last", false, "use the last command of your shell history")
	return cmd
}

func runAdd(cmd *cobra.Command, f *addFlags) error {
	paths, err := config.CheatsPaths()
	if err != nil {
		return err
	}
	dir, err := config.CheatsDir()
	if err != nil {
		return err
	}
	targetOf := func(command string) string { return cheats.CommandFile(dir, command) }
	if f.file != "" {
		target, err := resolveCheatFile(f.file, dir)
		if err != nil {
			return err
		}
		targetOf = func(string) string { return target }
	}

	entry := cheats.Entry{Title: f.title, Command: f.command, Description: f.description, Tags: f.tags}
	given := f.command != "" || f.last

	note := ""
	if entry.Command == "" && (f.last || isInteractive()) {
		hist := history.File()
		last, err := history.Last(hist, history.IsArsenalCmd)
		switch {
		case err == nil:
			entry.Command = last
			note = "from your shell history: edit it if needed"
		case f.last:
			return fmt.Errorf("%w (looked in %s)", err, hist)
		}
	}

	if entry.Command == "" || entry.Title == "" || !given {
		if !isInteractive() {
			return errors.New("no terminal to ask in: give the command (or --last) and --title")
		}
		answers, err := runSteps(addBox(entry, note, targetOf))
		if err != nil {
			return err
		}
		entry = cheats.Entry{
			Command: answers.Text("command"), Title: answers.Text("title"),
			Description: answers.Text("description"), Tags: answers.Text("tags"),
		}
		target := targetOf(entry.Command)
		if err := cheats.AppendEntry(target, entry); err != nil {
			return err
		}
		return warnOutsidePaths(cmd, target, paths)
	}
	target := targetOf(entry.Command)
	if err := cheats.AppendEntry(target, entry); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), tui.Success(fmt.Sprintf("Added %q to %s", strings.TrimSpace(entry.Title), tildePath(target))))
	return warnOutsidePaths(cmd, target, paths)
}

func warnOutsidePaths(cmd *cobra.Command, target string, paths []string) error {
	if !inCheatsPaths(target, paths) {
		fmt.Fprintln(cmd.ErrOrStderr(), tui.Warn(filepath.Dir(target)+" is not one of your cheatsheet folders, so ars will not find this entry (see cheats_paths in config.yml)"))
	}
	return nil
}

func addBox(prefill cheats.Entry, note string, targetOf func(command string) string) tui.StepBox {
	cmdHelp := "use <name> or <name|default> for variables"
	if note != "" {
		cmdHelp = note
	}
	entryOf := func(a tui.Answers) cheats.Entry {
		return cheats.Entry{Command: a.Text("command"), Title: a.Text("title"), Description: a.Text("description"), Tags: a.Text("tags")}
	}
	lines := func(a tui.Answers, last string) []string {
		out := strings.Split(strings.TrimRight(cheats.FormatEntry(entryOf(a)), "\n"), "\n")
		return append(out, "", last)
	}
	return tui.StepBox{
		Title: "Add a command",
		Steps: []tui.Step{
			{Key: "command", Title: "Command", Help: cmdHelp, Kind: tui.StepText, Default: prefill.Command, Validate: cheats.ValidateCommand},
			{Key: "title", Title: "Title", Help: "what it does, in your words (this is what you search for)", Kind: tui.StepText, Default: prefill.Title,
				Validate: cheats.ValidateTitle,
				ValidateWith: func(a tui.Answers, s string) error {
					if target := targetOf(a.Text("command")); cheats.TitleExists(target, s) {
						return fmt.Errorf("%s already has an entry titled %q", filepath.Base(target), strings.TrimSpace(s))
					}
					return nil
				}},
			{Key: "description", Title: "Description", Help: "optional, searched too", Kind: tui.StepMultiline, Default: prefill.Description},
			{Key: "tags", Title: "Tags", Help: "optional, comma-separated, e.g. nmap, recon", Kind: tui.StepText, Default: prefill.Tags},
		},
		Summary: func(a tui.Answers) []string {
			return lines(a, "Saves to "+filepath.Base(targetOf(a.Text("command"))))
		},
		Confirmed: func(a tui.Answers) []string {
			return lines(a, "Saved to "+filepath.Base(targetOf(a.Text("command"))))
		},
	}
}

func resolveCheatFile(name, cheatsDir string) (string, error) {
	name = strings.TrimSpace(name)
	if err := cheats.ValidateCheatFile(name); err != nil {
		return "", err
	}
	if name == "~" || strings.HasPrefix(name, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(name, "~")), nil
	}
	if filepath.IsAbs(name) || strings.ContainsRune(name, filepath.Separator) {
		return filepath.Clean(name), nil
	}
	return filepath.Join(cheatsDir, name), nil
}

func inCheatsPaths(file string, paths []string) bool {
	abs := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	dir := abs(filepath.Dir(file))
	for _, p := range paths {
		root := abs(p)
		if dir == root || strings.HasPrefix(dir, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
