// ars status shows the session and what is being logged, and ars pause stops and resumes the logging.

package cli

import (
	"strings"
	"time"

	"errors"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/rec"
	"github.com/ynn3m/arsenal/internal/replay"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

type statusRow struct{ label, value, next string }

func printStatus(out io.Writer, rows []statusRow) {
	labelW, valueW := 0, 0
	for _, r := range rows {
		labelW = max(labelW, lipgloss.Width(r.label))
		if r.next != "" {
			valueW = max(valueW, lipgloss.Width(r.value))
		}
	}
	pad := func(styled string, width int) string {
		return styled + strings.Repeat(" ", max(0, width-lipgloss.Width(styled)))
	}
	for _, r := range rows {
		line := "  " + pad(tui.Muted(r.label), labelW) + "  " + r.value
		if r.next != "" {
			line = "  " + pad(tui.Muted(r.label), labelW) + "  " + pad(r.value, valueW) + "   " + tui.Muted("-> "+r.next)
		}
		fmt.Fprintln(out, line)
	}
}

func hhmm(t time.Time) string {
	y1, m1, d1 := t.Date()
	if y2, m2, d2 := time.Now().Date(); y1 == y2 && m1 == m2 && d1 == d2 {
		return t.Format("15:04")
	}
	return t.Format("2006-01-02 15:04")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show a summary of the current session",
		Long:  "Shows the commands logged, recordings, notes and variables of the session you are in.",
		Example: "  ars status                  # in a session folder\n" +
			"  cd ~ && ars status          # outside a session: global variables only",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, tui.Heading("Arsenal status"))

			here, inFolder, err := currentSession()
			if err != nil {
				return err
			}
			bound, isBound := boundSession()
			sess := here
			if isBound {
				sess = bound
			}
			global, err := session.LoadGlobalVars()
			if err != nil {
				return err
			}

			var rows []statusRow
			switch {
			case isBound && inFolder && here.Dir == bound.Dir, !isBound && inFolder:
				rows = append(rows, statusRow{"Session", tui.Title(here.Name) + tui.Muted(" ("+tildePath(here.Dir)+")"), ""})
			case isBound && inFolder:
				rows = append(rows, statusRow{"Session", tui.Title(here.Name) + tui.Muted(" ("+tildePath(here.Dir)+"; this terminal is recording session "+bound.Name+")"), ""})
			case isBound:
				rows = append(rows, statusRow{"Session", "none here; this terminal is recording session " + tui.Title(bound.Name), ""})
			default:
				rows = append(rows, statusRow{"Session", "none", "ars init <name>"})
				rows = append(rows, statusRow{"Variables", plural(len(global), "global", "global"), "ars set -g / ars vars -g"})
				printStatus(out, rows)
				return nil
			}

			rows = append(rows, statusRow{"Data", tildePath(session.MetaDir(sess.Dir)), ""})

			paused := "no"
			if os.Getenv(EnvPaused) != "" {
				paused = tui.WarnStyle.Render("yes")
			}
			rows = append(rows, statusRow{"Paused", paused, "ars pause"})

			cmds, next := commandsSummary(sess.Dir, os.Getenv(EnvCDFile) != "" || isBound)
			rows = append(rows, statusRow{"Commands", cmds, next})

			recording := os.Getenv(rec.EnvRec) != ""
			rows = append(rows, statusRow{"Recording", recordingSummary(sess.Dir, recording), "ars rec / ars replay"})

			rows = append(rows, statusRow{"Notes", notesSummary(sess.Dir), "ars notes"})

			local, _ := session.LoadVars(sess.Dir)
			rows = append(rows, statusRow{"Variables", fmt.Sprintf("%d local, %d global", len(local), len(global)), "ars vars / ars set"})
			printStatus(out, rows)

			if !recording {
				fmt.Fprintln(out, tui.Muted("  What you type inside SSH or a reverse shell is only in a recording."))
			}
			return nil
		},
	}
}

func commandsSummary(dir string, integration bool) (value, next string) {
	if !integration {
		if rcHasIntegration() {
			return "not logged: " + restartHint, ""
		}
		return "not logged: shell integration is off", "ars setup"
	}
	p := cmdlog.Path(dir)
	fi, err := os.Stat(p)
	if err != nil {
		return "none logged yet", "ars history"
	}
	recs, _ := cmdlog.Load(p, 0)
	entries := cmdlog.Fold(recs)
	if len(entries) == 0 {
		return "none logged yet", "ars history"
	}
	s := fmt.Sprintf("%d logged", len(entries))
	if fi.Size() > cmdlog.DefaultTailBytes {
		s += " (last " + humanBytes(cmdlog.DefaultTailBytes) + " read)"
	}
	return s + ", last " + hhmm(entries[len(entries)-1].Time()), "ars history"
}

func recordingSummary(dir string, recording bool) string {
	state := "no"
	if recording {
		state = tui.SuccessStyle.Render("yes")
	}
	list, err := replay.List(dir, cmdlog.Path(dir))
	if err != nil {
		return state + " - " + err.Error()
	}
	if len(list) == 0 {
		return state + " - none yet"
	}
	var total int64
	interrupted := 0
	for _, in := range list {
		total += in.Size
		if in.State == replay.StateInterrupted {
			interrupted++
		}
	}
	s := fmt.Sprintf("%s - %s, %s, last %s", state, plural(len(list), "recording", "recordings"), humanBytes(total), hhmm(list[0].Started))
	if interrupted > 0 {
		s += fmt.Sprintf(" (%d interrupted)", interrupted)
	}
	return s
}

func notesSummary(dir string) string {
	md, err := session.ReadNotes(dir)
	if err != nil {
		if errors.Is(err, session.ErrNoNotes) {
			return "none yet"
		}
		return err.Error()
	}
	notes := session.ParseNotes(md)
	if len(notes) == 0 {
		return "none yet"
	}
	last := notes[0].Time
	for _, n := range notes {
		if n.Time.After(last) {
			last = n.Time
		}
	}
	return plural(len(notes), "note", "notes") + ", last " + hhmm(last)
}

func newPauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pause or resume logging and recording",
		Long:  "Pauses logging and recording in this terminal. Run it again to resume.",
		Example: "  ars pause    # stop logging and recording in this terminal; run it again to resume\n" +
			"  ars status   # shows whether this terminal is paused",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rcHasIntegration() {
				return errors.New("ars pause needs the shell integration: restart the terminal")
			}
			return errors.New("ars pause needs the shell integration: run ars setup")
		},
	}
}

var writeTTY = func(s string) {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.WriteString(f, s)
}

func emitMarker(how, cheat string) {
	if os.Getenv(rec.EnvRec) == "" || os.Getenv(EnvPaused) != "" {
		return
	}
	label := "ars: " + how
	if cheat != "" {
		label += ": " + cheat
	}
	writeTTY(rec.Sequence(rec.VerbMarker, label))
}
