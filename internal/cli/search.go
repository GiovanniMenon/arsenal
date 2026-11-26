// Runs the search you get by typing ars alone, then puts the chosen command in the prompt.

package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/insert"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

type searchFlags struct {
	query   string
	prefix  bool
	noTags  bool
	print   bool
	copy    bool
	exec    bool
	tmux    bool
	check   bool
	outfile string
}

var (
	runSearchUI = tui.RunSearch
	deliver     = deliverCommand
	checkFinder = cheats.ShellFinder
)

const maxWarnings = 5

func loadCheats(errOut io.Writer) ([]*cheats.Cheat, error) {
	paths, err := config.CheatsPaths()
	if err != nil {
		return nil, err
	}
	res := cheats.Load(paths)
	for i, w := range res.Warnings {
		if i == maxWarnings {
			fmt.Fprintln(errOut, tui.Warn(fmt.Sprintf("... and %d more cheatsheet problems", len(res.Warnings)-maxWarnings)))
			break
		}
		fmt.Fprintln(errOut, tui.Warn(w.Error()))
	}
	return res.Cheats, nil
}

func runSearch(cmd *cobra.Command, f *searchFlags) error {
	if f.check {
		return runCheck(cmd)
	}
	list, err := loadCheats(cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	editor, err := newEntryEditor(&list)
	if err != nil {
		return err
	}
	query, pos, notice := f.query, 0, ""
	for {
		here, _, err := currentSession()
		if err != nil {
			return err
		}
		sess := here.Name
		vars, err := session.Resolve(here.Dir)
		if err != nil {
			return err
		}
		prefix := vars.Vars[cheats.PrefixVar]

		res, err := runSearchUI(tui.SearchOptions{
			Cheats:   list,
			Vars:     vars,
			Session:  sess,
			Query:    query,
			Position: pos,
			ShowTags: !f.noTags,
			Prefix:   prefix,
			PrefixOn: f.prefix,
			Notice:   notice,
			Save:     editor.save,
			ReadOnly: editor.readOnly,
			Delete:   editor.delete,
		})
		if err != nil {
			return fmt.Errorf("cannot open the search: it needs a terminal (%w)", err)
		}
		if res.Cheat == nil {
			return nil
		}
		query, pos, notice = res.Query, res.Position, ""

		c := cheats.NewCommand(res.Cheat, vars.Vars)
		usePrefix := ""
		if f.prefix {
			usePrefix = prefix
		}
		for name, v := range res.Values {
			c.Set(name, v)
		}
		line, err := c.Build()
		if err != nil {
			notice = tui.Error(err.Error())
			continue
		}

		final := cheats.Prefixed(line, usePrefix)
		again, d, err := deliver(cmd, final, f)
		if err == nil {
			logInsert(cmd, final, res.Cheat.Name, d)
		}
		if err != nil || !again {
			return err
		}
	}
}

type delivery struct {
	How  string
	Exit *int
}

func deliverCommand(cmd *cobra.Command, line string, f *searchFlags) (again bool, d delivery, err error) {
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	switch {
	case f.copy:
		if err := insert.Copy(line); err != nil {
			return false, d, err
		}
		fmt.Fprintln(errOut, tui.Success("Copied to the clipboard"))
		return false, delivery{How: cmdlog.HowClipboard}, nil

	case f.print:
		_, err := fmt.Fprintln(out, line)
		return false, delivery{How: cmdlog.HowPrint}, err

	case f.outfile != "":
		return false, delivery{How: cmdlog.HowFile}, insert.WriteFile(f.outfile, line)

	case f.exec && !f.tmux:
		code, err := insert.RunCode(line, os.Stdin, out, errOut)
		return false, delivery{How: cmdlog.HowExec, Exit: &code}, err

	case f.tmux:
		err := insert.SendToPane(line, f.exec)
		if err == nil {
			how := cmdlog.HowTmux
			if f.exec {
				how = cmdlog.HowTmuxExec
			}
			return true, delivery{How: how}, nil
		}
		fmt.Fprintln(errOut, tui.Warn(err.Error()+": putting the command in the prompt instead"))
	}

	method, hint, err := insert.ToPrompt(line)
	if hint != "" {
		fmt.Fprintln(errOut, tui.Warn(hint))
	}
	if err != nil {
		fmt.Fprintln(out, line)
		return false, d, err
	}
	if method == insert.MethodClipboard {
		fmt.Fprintln(errOut, tui.Success("Copied to the clipboard"))
	}
	return false, delivery{How: howOf(method)}, nil
}

func howOf(m insert.Method) string {
	switch m {
	case insert.MethodShell:
		return cmdlog.HowShell
	case insert.MethodClipboard:
		return cmdlog.HowClipboard
	}
	return cmdlog.HowPrompt
}

func runCheck(cmd *cobra.Command) error {
	list, err := loadCheats(cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	for _, r := range cheats.Check(list, checkFinder) {
		switch r.Status {
		case cheats.CheckOK:
			fmt.Fprintln(out, tui.SuccessStyle.Render(fmt.Sprintf("%-4s : %s (%s)", "OK", r.Binary, r.Detail)))
		case cheats.CheckMissing:
			fmt.Fprintln(out, tui.WarnStyle.Render(fmt.Sprintf("%-4s : %s (full command is : %s )", "KO", r.Binary, r.Detail)))
		case cheats.CheckSkipped:
			fmt.Fprintln(out, tui.KeyStyle.Render(fmt.Sprintf("%-4s : %s (full command is : %s )", "SKIP", r.Binary, r.Detail)))
		}
	}
	return nil
}
