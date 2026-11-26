// Prints the summary box shown by ars init.

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/replay"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

type initInfo struct {
	New         bool
	Name, Dir   string
	RecOn       bool
	Hints       bool
	Integration bool
	RCInstalled bool
}

func sessionCounts(dir string) (commands, notes, recordings int) {
	if recs, err := cmdlog.Load(cmdlog.Path(dir), 0); err == nil {
		commands = len(cmdlog.Fold(recs))
	}
	if md, err := session.ReadNotes(dir); err == nil {
		notes = len(session.ParseNotes(md))
	}
	if list, err := replay.List(dir, cmdlog.Path(dir)); err == nil {
		recordings = len(list)
	}
	return
}

func printInit(out io.Writer, in initInfo) error {
	vars, err := session.Resolve(in.Dir)
	if err != nil {
		return err
	}
	target := vars.Vars["target"]

	type row struct{ label, value string }
	rows := []row{{"Folder", tildePath(in.Dir)}}
	if !in.New && target != "" {
		rows = append(rows, row{"Target", target})
	}
	if !in.New {
		c, n, r := sessionCounts(in.Dir)
		rows = append(rows, row{"So far", fmt.Sprintf("%s, %s, %s",
			plural(c, "command", "commands"), plural(n, "note", "notes"), plural(r, "recording", "recordings"))})
	}
	rec := "off"
	switch {
	case in.RecOn && in.New:
		rec = "on (exit or ctrl-d stops it)"
	case in.RecOn:
		rec = "on"
	case in.New:
		rec = "off (ars rec starts it, exit stops it)"
	}
	rows = append(rows, row{"Recording", rec})
	if !in.Integration {
		how := "run ars setup to do it automatically"
		if in.RCInstalled {
			how = restartHint
		}
		rows = append(rows, row{"Shell", "cd there yourself (" + how + ")"})
	}

	width := 0
	for _, r := range rows {
		width = max(width, len(r.label))
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, tui.KeyStyle.Render(fmt.Sprintf("%-*s", width, r.label))+"  "+r.value)
	}
	title := "Resumed: " + in.Name
	if in.New {
		title = "New session: " + in.Name
	}
	fmt.Fprintln(out, tui.Box(title, strings.Join(lines, "\n")))

	if !in.Hints {
		return nil
	}
	fmt.Fprintln(out)
	if in.New {
		fmt.Fprintln(out, tui.Heading("Start here:"))
		steps := [][2]string{
			{"ars set target=<ip>", "the target, used to fill commands"},
			{"ars -s <tool>", "find a command, e.g. ars -s nmap"},
			{`ars note "<text>"`, "keep notes while you work"},
		}
		for i, s := range steps {
			fmt.Fprintf(out, "  %d. %s  %s\n", i+1, tui.KeyStyle.Render(fmt.Sprintf("%-*s", 20, s[0])), tui.Muted(s[1]))
		}
		return nil
	}
	next := []string{"ars -s <tool>", "ars notes", "ars history", "ars status"}
	if target == "" {
		next = append([]string{"ars set target=<ip>"}, next...)
	}
	for i := range next {
		next[i] = tui.KeyStyle.Render(next[i])
	}
	fmt.Fprintln(out, tui.Heading("Next:")+" "+strings.Join(next, tui.Muted(" | ")))
	return nil
}
