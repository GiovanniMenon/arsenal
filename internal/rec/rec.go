//go:build darwin || linux

// Package rec records a shell running in a PTY to asciicast files.
package rec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/ynn3m/arsenal/internal/cast"
	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/session"
)

const (
	EnvRec   = "ARSENAL_REC"
	EnvRecID = "ARSENAL_REC_ID"
)

const DirName = "recordings"

const DefaultMaxSize int64 = 256 << 20

const exitGrace = 500 * time.Millisecond

const hangupGrace = 2 * time.Second

var ErrNested = errors.New("this terminal is already being recorded")

var ErrNotTerminal = errors.New("recording needs a terminal")

type Options struct {
	Session string
	Dir     string
	Shell   string
	Command []string
	WorkDir string
	NoLog   bool
	Input   bool
	MaxSize int64

	In, Out *os.File
	Msg     io.Writer
	Env     []string

	Signals <-chan os.Signal

	Announce func(castPath string)
}

type Result struct {
	ID       string
	Files    []string
	ExitCode int
	Duration time.Duration
	Bytes    int64
}

type sink struct {
	mu      sync.Mutex
	dir     string
	id      string
	hdr     cast.Header
	start   time.Time
	maxSize int64
	input   bool
	notice  func(string)

	f      *os.File
	w      *cast.Writer
	files  []string
	total  int64
	dead   bool
	paused bool
}

func (s *sink) path(n int) string {
	if n == 1 {
		return filepath.Join(s.dir, s.id+".cast")
	}
	return filepath.Join(s.dir, fmt.Sprintf("%s-%03d.cast", s.id, n))
}

func (s *sink) open(hdr cast.Header, start time.Time) error {
	p := s.path(len(s.files) + 1)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, config.FileMode)
	if err != nil {
		return err
	}
	if err := f.Chmod(config.FileMode); err != nil {
		f.Close()
		return err
	}
	w, err := cast.NewWriter(f, hdr, start)
	if err != nil {
		f.Close()
		return err
	}
	s.f, s.w = f, w
	s.files = append(s.files, p)
	return nil
}

func (s *sink) closeFile(at time.Time) {
	if s.w != nil {
		_ = s.w.Flush(at)
		s.total += s.w.Size()
		s.w = nil
	}
	if s.f != nil {
		_ = s.f.Close()
		s.f = nil
	}
}

func (s *sink) fail(err error) {
	if s.dead {
		return
	}
	s.dead = true
	s.notice(fmt.Sprintf("cannot write the recording (%v): the shell keeps working, but it is no longer recorded", err))
}

func (s *sink) rotate(at time.Time, cols, rows int) {
	if s.w == nil || s.w.Size() < s.maxSize {
		return
	}
	s.closeFile(at)
	hdr := s.hdr
	hdr.Width, hdr.Height = cols, rows
	hdr.Timestamp = at.Unix()
	if err := s.open(hdr, at); err != nil {
		s.fail(err)
		return
	}
	s.notice(fmt.Sprintf("this recording passed %d MB: continuing in %s", s.maxSize>>20, s.files[len(s.files)-1]))
}

func (s *sink) output(at time.Time, b []byte, cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead || s.w == nil || s.paused {
		return
	}
	if err := s.w.Output(at, b); err != nil {
		s.fail(err)
		return
	}
	s.rotate(at, cols, rows)
}

func (s *sink) inputEvent(at time.Time, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead || s.w == nil || !s.input || s.paused {
		return
	}
	if err := s.w.Input(at, b); err != nil {
		s.fail(err)
	}
}

func (s *sink) resize(at time.Time, cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead || s.w == nil {
		return
	}
	if err := s.w.Resize(at, cols, rows); err != nil {
		s.fail(err)
	}
}

func (s *sink) marker(at time.Time, label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead || s.w == nil || s.paused {
		return
	}
	if err := s.w.Marker(at, label); err != nil {
		s.fail(err)
	}
}

func (s *sink) setPaused(at time.Time, pause bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead || s.w == nil || s.paused == pause {
		return
	}
	label := "recording resumed"
	if pause {
		label = "recording paused"
		_ = s.w.Flush(at)
	}
	if err := s.w.Marker(at, label); err != nil {
		s.fail(err)
	}
	s.paused = pause
}

func (s *sink) finish(at time.Time) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeFile(at)
	return s.total
}

func newID(dir string, now time.Time) (string, error) {
	base := fmt.Sprintf("%s-%d", now.Format("20060102-150405"), os.Getpid())
	id := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, id+".cast")); errors.Is(err, os.ErrNotExist) {
			return id, nil
		}
		if i > 99 {
			return "", errors.New("cannot create a new recording file: 99 recordings already share this name (delete or move some)")
		}
		id = fmt.Sprintf("%s-r%d", base, i)
	}
}

func childEnv(base []string, name, dir, castPath, id string) []string {
	drop := map[string]bool{
		"ARSENAL_SESSION": true, "ARSENAL_SESSION_DIR": true, "ARSENAL_PAUSED": true,
		EnvRec: true, EnvRecID: true, "ARSENAL_CD_FILE": true, "ARSENAL_CMD_FILE": true,
	}
	var env []string
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			env = append(env, kv)
		}
	}
	return append(env,
		"ARSENAL_SESSION="+name, "ARSENAL_SESSION_DIR="+dir,
		EnvRec+"="+castPath, EnvRecID+"="+id,
	)
}

func Run(o Options) (Result, error) {
	var res Result
	if p := os.Getenv(EnvRec); p != "" {
		return res, fmt.Errorf("%w (recording to %s): type exit to leave it first", ErrNested, p)
	}
	if o.In == nil {
		o.In = os.Stdin
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Msg == nil {
		o.Msg = os.Stderr
	}
	if o.Env == nil {
		o.Env = os.Environ()
	}
	if o.MaxSize <= 0 {
		o.MaxSize = DefaultMaxSize
	}
	if !term.IsTerminal(int(o.In.Fd())) {
		return res, ErrNotTerminal
	}

	argv := o.Command
	if len(argv) == 0 {
		sh := o.Shell
		if sh == "" {
			sh = os.Getenv("SHELL")
		}
		if sh == "" {
			sh = "/bin/sh"
		}
		argv = []string{sh}
	}

	recDir := session.RecordingsDir(o.Dir)
	if err := os.MkdirAll(recDir, config.DirMode); err != nil {
		return res, fmt.Errorf("cannot create %s: %w", recDir, err)
	}
	if err := os.Chmod(recDir, config.DirMode); err != nil {
		return res, fmt.Errorf("cannot set permissions on %s: %w", recDir, err)
	}

	started := time.Now()
	id, err := newID(recDir, started)
	if err != nil {
		return res, err
	}
	res.ID = id

	size, err := pty.GetsizeFull(o.In)
	if err != nil || size.Cols == 0 || size.Rows == 0 {
		size = &pty.Winsize{Rows: 24, Cols: 80}
	}
	cols, rows := int(size.Cols), int(size.Rows)

	hdrEnv := map[string]string{}
	for _, k := range []string{"SHELL", "TERM"} {
		if v := os.Getenv(k); v != "" {
			hdrEnv[k] = v
		}
	}
	notice := func(s string) { _, _ = io.WriteString(o.Out, "\r\n"+s+"\r\n") }
	sk := &sink{
		dir: recDir, id: id, start: started, maxSize: o.MaxSize, input: o.Input, notice: notice,
		hdr: cast.Header{
			Width: cols, Height: rows, Timestamp: started.Unix(), Command: strings.Join(argv, " "),
			Title: "arsenal: " + o.Session, Env: hdrEnv,
		},
	}
	if err := sk.open(sk.hdr, started); err != nil {
		return res, fmt.Errorf("cannot create the recording: %w", err)
	}
	castPath := sk.files[0]

	logPath := cmdlog.Path(o.Dir)
	logRec := func(r cmdlog.Record) {
		if o.NoLog {
			return
		}
		r.Session, r.Rec, r.PID = o.Session, id, os.Getpid()
		_ = cmdlog.Append(logPath, r)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = o.WorkDir
	cmd.Env = childEnv(o.Env, o.Session, o.Dir, castPath, id)

	if o.Announce != nil {
		o.Announce(castPath)
	} else {
		fmt.Fprintf(o.Msg, "Recording session %s to %s (type exit to stop)\n", o.Session, castPath)
	}

	ptmx, err := pty.StartWithSize(cmd, size)
	if err != nil {
		sk.finish(time.Now())
		_ = os.Remove(castPath)
		return res, fmt.Errorf("cannot start %s: %w", argv[0], err)
	}
	logRec(cmdlog.Record{Ev: cmdlog.EvRecStart, TS: started.UnixMilli(), Cmd: strings.Join(argv, " "), CWD: cmd.Dir})

	oldState, err := term.MakeRaw(int(o.In.Fd()))
	if err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
		_ = ptmx.Close()
		sk.finish(time.Now())
		return res, fmt.Errorf("cannot prepare the terminal for recording: %w", err)
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			_ = term.Restore(int(o.In.Fd()), oldState)
		}
	}
	defer restore()

	sigs := o.Signals
	if sigs == nil {
		ch := make(chan os.Signal, 8)
		signal.Notify(ch, syscall.SIGWINCH, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
		defer signal.Stop(ch)
		sigs = ch
	}

	var sizeMu sync.Mutex
	curCols, curRows := cols, rows

	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := o.In.Read(buf)
			if n > 0 {
				_, _ = ptmx.Write(buf[:n])
				sk.inputEvent(time.Now(), buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		var filter oscFilter
		buf := make([]byte, 32<<10)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				for _, it := range filter.Process(buf[:n]) {
					if it.Cmd == nil {
						_, _ = o.Out.Write(it.Pass)
						sizeMu.Lock()
						c, r := curCols, curRows
						sizeMu.Unlock()
						sk.output(time.Now(), it.Pass, c, r)
						continue
					}
					switch it.Cmd.Verb {
					case VerbMarker:
						sk.marker(time.Now(), it.Cmd.Text)
					case VerbPause:
						sk.setPaused(time.Now(), true)
					case VerbResume:
						sk.setPaused(time.Now(), false)
					}
				}
			}
			if err != nil {
				if rest := filter.Flush(); len(rest) > 0 {
					_, _ = o.Out.Write(rest)
					sizeMu.Lock()
					c, r := curCols, curRows
					sizeMu.Unlock()
					sk.output(time.Now(), rest, c, r)
				}
				return
			}
		}
	}()

	var killTimer *time.Timer
loop:
	for {
		select {
		case <-exited:
			break loop
		case s := <-sigs:
			switch s {
			case syscall.SIGWINCH:
				if err := pty.InheritSize(o.In, ptmx); err == nil {
					if sz, err := pty.GetsizeFull(ptmx); err == nil && sz.Cols > 0 && sz.Rows > 0 {
						sizeMu.Lock()
						curCols, curRows = int(sz.Cols), int(sz.Rows)
						sizeMu.Unlock()
						sk.resize(time.Now(), int(sz.Cols), int(sz.Rows))
					}
				}
			default:
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
				if killTimer == nil {
					killTimer = time.AfterFunc(hangupGrace, func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
				}
			}
		}
	}
	if killTimer != nil {
		killTimer.Stop()
	}

	select {
	case <-readDone:
	case <-time.After(exitGrace):
	}
	_ = ptmx.Close()
	ended := time.Now()
	res.Bytes = sk.finish(ended)
	res.Files = append([]string(nil), sk.files...)
	res.Duration = ended.Sub(started)
	restore()

	res.ExitCode = exitCode(cmd, waitErr)
	exit := res.ExitCode
	dur := res.Duration.Milliseconds()
	logRec(cmdlog.Record{Ev: cmdlog.EvRecEnd, TS: ended.UnixMilli(), Exit: &exit, Dur: &dur})

	fmt.Fprintf(o.Msg, "Recording saved: %s (%s, %s)\n", res.Files[0], res.Duration.Round(time.Second), humanSize(res.Bytes))
	if len(res.Files) > 1 {
		fmt.Fprintf(o.Msg, "The recording continues in %d more files (same name, with a number)\n", len(res.Files)-1)
	}
	return res, nil
}

func exitCode(cmd *exec.Cmd, waitErr error) int {
	if cmd.ProcessState != nil {
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return cmd.ProcessState.ExitCode()
	}
	if waitErr != nil {
		return 1
	}
	return 0
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
