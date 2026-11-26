// ars replay lists, plays and exports the recordings of a session.

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/replay"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

var (
	chooseRecording = tui.AskChoice
	openPlayer      = defaultPlayer
)

type player struct {
	out        io.Writer
	keys       <-chan byte
	done       func()
	cols, rows int
}

func defaultPlayer(cmd *cobra.Command) (player, error) {
	if !term.IsTerminal(os.Stdout.Fd()) {
		return player{}, errors.New("playing a recording needs a terminal: use --text to read it as text")
	}
	p := player{out: os.Stdout, done: func() {}}
	if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil {
		p.cols, p.rows = w, h
	}
	if term.IsTerminal(os.Stdin.Fd()) {
		state, err := term.MakeRaw(os.Stdin.Fd())
		if err == nil {
			keys := replay.Keys(os.Stdin)
			p.keys = keys
			p.done = func() { _ = term.Restore(os.Stdin.Fd(), state) }
		}
	}
	return p, nil
}

type replayFlags struct {
	session   string
	speed     float64
	idleLimit float64
	from      time.Duration
	at        int64
	text      bool
	markers   bool
	raw       bool
	list      bool
	redact    bool
}

func newReplayCmd() *cobra.Command {
	var f replayFlags
	cmd := &cobra.Command{
		Use:   "replay [id|file]",
		Short: "Play back a recording",
		Long: "Lists the recordings of the session and plays the one you pick, or the one you name.\n" +
			"Use --text to read a recording as plain text.",
		Example: "  ars replay                          # pick from the list\n" +
			"  ars replay --speed 2 --idle-limit 1   # faster, short pauses\n" +
			"  ars replay --text > out.txt",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runReplay(cmd, args, f) },
	}
	cmd.Flags().StringVar(&f.session, "session", "", "use the recordings of session `NAME` (default: the current session)")
	cmd.Flags().Float64Var(&f.speed, "speed", 1, "playback speed (2 = twice as fast)")
	cmd.Flags().Float64Var(&f.idleLimit, "idle-limit", 0, "cap pauses at `SECONDS`")
	cmd.Flags().DurationVar(&f.from, "from", 0, "start at a time in the recording, e.g. 90s or 1m30s")
	cmd.Flags().Int64Var(&f.at, "at", 0, "start at a Unix time in milliseconds (as in ars history --json)")
	cmd.Flags().BoolVar(&f.text, "text", false, "write the recording as plain text")
	cmd.Flags().BoolVar(&f.markers, "markers", false, "list the markers (commands from ars, pauses) with their time")
	cmd.Flags().BoolVar(&f.raw, "raw", false, "play the recording unfiltered (it can change your terminal or clipboard)")
	cmd.Flags().BoolVar(&f.list, "list", false, "only list the recordings")
	cmd.Flags().BoolVar(&f.redact, "redact", false, "with --text: hide passwords, tokens and hashes")
	return cmd
}

func runReplay(cmd *cobra.Command, args []string, f replayFlags) error {
	if f.redact && !f.text {
		return errors.New("--redact only works with --text")
	}
	var sess session.Info
	var err error
	if f.session != "" {
		sess, err = session.ByName(f.session)
	} else {
		sess, err = hereSession()
	}
	if err != nil {
		return err
	}
	name, dir := sess.Name, sess.Dir
	logPath := cmdlog.Path(dir)
	out := cmd.OutOrStdout()

	var info replay.Info
	if len(args) == 1 {
		if info, err = replay.Find(dir, logPath, args[0]); err != nil {
			return err
		}
	} else {
		list, err := replay.List(dir, logPath)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintln(out, tui.Muted("No recordings in session "+name+". Start one with ars rec."))
			return nil
		}
		printRecordings(out, name, list)
		if f.list || !isInteractive() {
			return nil
		}
		labels := make([]string, len(list))
		for i, in := range list {
			labels[i] = recordingLabel(in)
		}
		i, err := chooseRecording("Play which recording?", "newest first", labels)
		if err != nil {
			return err
		}
		info = list[i]
	}

	switch {
	case f.markers:
		at, labels, err := info.Markers()
		if err != nil {
			return err
		}
		if len(labels) == 0 {
			fmt.Fprintln(out, tui.Muted("No markers in this recording."))
		}
		for i := range labels {
			fmt.Fprintf(out, "%s  %s\n", replay.Clock(at[i]), labels[i])
		}
		return nil
	case f.text:
		text, err := info.Text()
		if err != nil {
			return err
		}
		if f.redact {
			rd, err := newRedactor()
			if err != nil {
				return err
			}
			text = rd.Text(text)
		}
		_, err = io.WriteString(out, text)
		return err
	}
	return playRecording(cmd, info, f)
}

func recordingLabel(in replay.Info) string {
	s := fmt.Sprintf("%s  %s  %s", in.ID, replay.Clock(in.Duration), humanBytes(in.Size))
	if in.State == replay.StateInterrupted {
		s += "  (interrupted)"
	}
	return s
}

func printRecordings(out io.Writer, name string, list []replay.Info) {
	fmt.Fprintln(out, tui.Heading("Recordings of "+name))
	fmt.Fprintf(out, "  %-26s %-17s %-9s %-10s %s\n", "ID", "STARTED", "LENGTH", "SIZE", "STATE")
	for _, in := range list {
		state := in.State
		switch state {
		case replay.StateInterrupted:
			state = tui.WarnStyle.Render("interrupted")
		case replay.StateUnknown:
			state = tui.Muted("-")
		}
		parts := ""
		if n := len(in.Parts); n > 1 {
			parts = fmt.Sprintf(" (%d files)", n)
		}
		fmt.Fprintf(out, "  %-26s %-17s %-9s %-10s %s%s\n", in.ID, in.Started.Format("2006-01-02 15:04"), replay.Clock(in.Duration), humanBytes(in.Size), state, parts)
	}
}

func playRecording(cmd *cobra.Command, info replay.Info, f replayFlags) error {
	from := f.from
	if f.at > 0 {
		from = time.UnixMilli(f.at).Sub(info.Started)
		if from < 0 {
			from = 0
		}
	}
	if from > info.Duration {
		return fmt.Errorf("the recording is only %s long", replay.Clock(info.Duration))
	}
	p, err := openPlayer(cmd)
	if err != nil {
		return err
	}
	defer p.done()

	msg := cmd.ErrOrStderr()
	fmt.Fprintf(msg, "%s %s (%s, recorded at %dx%d)  space pause, +/- speed, q stop\r\n",
		tui.Title("Playing"), info.ID, replay.Clock(info.Duration), info.Header.Width, info.Header.Height)
	if p.cols > 0 && (p.cols < info.Header.Width || p.rows < info.Header.Height) {
		fmt.Fprintf(msg, "%s\r\n", tui.Warn(fmt.Sprintf("your terminal is %dx%d, smaller than the recording: lines may wrap", p.cols, p.rows)))
	}
	if strings.Contains(info.State, replay.StateInterrupted) {
		fmt.Fprintf(msg, "%s\r\n", tui.Warn("this recording was interrupted: it may end abruptly"))
	}

	err = info.Play(replay.PlayOptions{
		Out:       p.out,
		Keys:      p.keys,
		Speed:     f.speed,
		IdleLimit: time.Duration(f.idleLimit * float64(time.Second)),
		From:      from,
		Raw:       f.raw,
	})
	if errors.Is(err, replay.ErrStopped) {
		return nil
	}
	return err
}
