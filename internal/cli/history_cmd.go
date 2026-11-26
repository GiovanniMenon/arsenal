// ars history lists or browses the commands logged in a session.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/redact"
	"github.com/ynn3m/arsenal/internal/replay"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

var runHistoryUI = tui.RunHistory

type historyFlags struct {
	query       string
	session     string
	all         bool
	failed      bool
	since       string
	cwd         string
	allCommands bool
	print       bool
	json        bool
	list        bool
	redact      bool
}

var sinceRe = regexp.MustCompile(`^(\d+)([smhdw])$`)

func parseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if m := sinceRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{
			"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour,
		}[m[2]]
		return now.Add(-time.Duration(n) * unit), nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("--since %q is not understood: use 90m, 2h, 3d, 1w, or a date such as 2026-11-24 or \"2026-11-24 14:30\"", s)
}

func historyEntries(sessions []session.Info, f cmdlog.Filter) ([]cmdlog.Entry, error) {
	var all []cmdlog.Entry
	for _, s := range sessions {
		name := s.Name
		recs, err := cmdlog.Load(cmdlog.Path(s.Dir), 0)
		if err != nil {
			return nil, err
		}
		for _, e := range cmdlog.Select(cmdlog.Fold(recs), f) {
			if e.Start.Session == "" {
				e.Start.Session = name
			}
			all = append(all, e)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Start.TS < all[j].Start.TS })
	return all, nil
}

type jsonEntry struct {
	ID      string `json:"id,omitempty"`
	TS      int64  `json:"ts"`
	Session string `json:"session,omitempty"`
	CWD     string `json:"cwd,omitempty"`
	Cmd     string `json:"cmd"`
	Sh      string `json:"sh,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Rec     string `json:"rec,omitempty"`
	Via     string `json:"via"`
	Cheat   string `json:"cheat,omitempty"`
	Exit    *int   `json:"exit,omitempty"`
	Dur     *int64 `json:"dur,omitempty"`
	Ended   bool   `json:"ended"`
}

func toJSON(e cmdlog.Entry) jsonEntry {
	j := jsonEntry{
		ID: e.Start.ID, TS: e.Start.TS, Session: e.Start.Session, CWD: e.Start.CWD, Cmd: e.Start.Cmd,
		Sh: e.Start.Sh, PID: e.Start.PID, Rec: e.Start.Rec, Via: e.Via, Ended: e.Ended(),
	}
	if e.Insert != nil {
		j.Cheat = e.Insert.Cheat
	}
	if e.End != nil {
		j.Exit, j.Dur = e.End.Exit, e.End.Dur
	}
	return j
}

func statusWord(e cmdlog.Entry) string {
	code, ok := e.ExitCode()
	switch {
	case !ok:
		return ".."
	case code == 0:
		return "ok"
	}
	return fmt.Sprintf("!%d", code)
}

func writeHistoryText(out io.Writer, entries []cmdlog.Entry, withSession bool, rd *redact.Redactor) {
	for _, e := range entries {
		sess := ""
		if withSession {
			sess = e.Start.Session + "  "
		}
		fmt.Fprintf(out, "%s  %-4s  %s%s\n", e.Time().Format("2006-01-02 15:04:05"), statusWord(e), sess, rd.Line(strings.Join(strings.Fields(e.Start.Cmd), " ")))
	}
}

func newHistoryCmd() *cobra.Command {
	var f historyFlags
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Search the commands you ran in a session",
		Long:  "Lists the commands you ran in the current session.",
		Example: "  ars history\n" +
			"  ars history --failed --since 2h\n" +
			"  ars history --list --redact   # as text, secrets hidden\n" +
			"  ars history --all -s nmap",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runHistory(cmd, f) },
	}
	cmd.Flags().StringVarP(&f.query, "search", "s", "", "start with `QUERY` in the search box")
	cmd.Flags().StringVar(&f.session, "session", "", "use the log of session `NAME` (default: the current session)")
	cmd.Flags().BoolVar(&f.all, "all", false, "search every session")
	cmd.Flags().BoolVar(&f.failed, "failed", false, "only commands that failed")
	cmd.Flags().StringVar(&f.since, "since", "", "only commands since `WHEN` (90m, 2h, 3d, 1w or a date)")
	cmd.Flags().StringVar(&f.cwd, "cwd", "", "only commands run in `DIR`")
	cmd.Flags().BoolVar(&f.allCommands, "all-commands", false, "include ars commands")
	cmd.Flags().BoolVarP(&f.print, "print", "p", false, "print the chosen command")
	cmd.Flags().BoolVar(&f.json, "json", false, "print as JSON lines")
	cmd.Flags().BoolVar(&f.list, "list", false, "print as text instead of opening the list")
	cmd.Flags().BoolVar(&f.redact, "redact", false, "hide passwords, tokens and hashes in the output")
	return cmd
}

func runHistory(cmd *cobra.Command, f historyFlags) error {
	var sessions []session.Info
	label, path := "", ""
	switch {
	case f.all:
		var err error
		if sessions, err = session.List(); err != nil {
			return err
		}
		label = "all sessions"
	default:
		var in session.Info
		var err error
		if f.session != "" {
			in, err = session.ByName(f.session)
		} else {
			in, err = hereSession()
		}
		if err != nil {
			return err
		}
		sessions, label = []session.Info{in}, in.Name
		path = cmdlog.Path(in.Dir)
	}
	dirs := map[string]string{}
	for _, s := range sessions {
		dirs[s.Name] = s.Dir
	}

	filter := cmdlog.Filter{Failed: f.failed, CWD: f.cwd, HideArs: !f.allCommands}
	if f.since != "" {
		t, err := parseSince(f.since, time.Now())
		if err != nil {
			return err
		}
		filter.Since = t
	}
	entries, err := historyEntries(sessions, filter)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	var rd *redact.Redactor
	if f.redact {
		if rd, err = newRedactor(); err != nil {
			return err
		}
	}

	if f.json {
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		for _, e := range entries {
			j := toJSON(e)
			j.Cmd, j.CWD = rd.Line(j.Cmd), rd.Line(j.CWD)
			if err := enc.Encode(j); err != nil {
				return err
			}
		}
		return nil
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), tui.Muted("No commands logged in "+label+" yet: run commands in a session folder (ars init <name>) to log them."))
		return nil
	}
	if f.list || !isInteractive() {
		writeHistoryText(out, entries, f.all, rd)
		return nil
	}

	opts := tui.HistoryOptions{Session: label, Path: path, Entries: entries, Query: f.query}
	if rd != nil {
		opts.Mask = rd.Text
	}
	res, err := runHistoryUI(opts)
	if err != nil {
		return fmt.Errorf("cannot open the history: it needs a terminal (%w)", err)
	}
	if res.Entry == nil {
		return nil
	}
	if res.Replay {
		return replayFromHistory(cmd, *res.Entry, dirs)
	}
	line := res.Entry.Start.Cmd
	_, d, err := deliverCommand(cmd, line, &searchFlags{print: f.print})
	if err != nil {
		return err
	}
	logInsert(cmd, line, "history", d)
	return nil
}

func replayFromHistory(cmd *cobra.Command, e cmdlog.Entry, dirs map[string]string) error {
	dir, ok := dirs[e.Start.Session]
	if !ok {
		return fmt.Errorf("session %s not found (ars sessions lists them)", e.Start.Session)
	}
	info, err := replay.Find(dir, cmdlog.Path(dir), e.Start.Rec)
	if err != nil {
		return fmt.Errorf("the recording of this command is not available: %w", err)
	}
	return playRecording(cmd, info, replayFlags{speed: 1, at: e.Start.TS})
}

func newRedactor() (*redact.Redactor, error) {
	extra, err := config.RedactPatterns()
	if err != nil {
		return nil, err
	}
	return redact.New(extra)
}
