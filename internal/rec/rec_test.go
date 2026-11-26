//go:build darwin || linux

package rec

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/ynn3m/arsenal/internal/cast"
	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/session"
)

type fakeTerminal struct {
	t      *testing.T
	master *os.File
	slave  *os.File
	mu     sync.Mutex
	screen bytes.Buffer
}

func newTerminal(t *testing.T) *fakeTerminal {
	t.Helper()
	m, s, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := pty.Setsize(m, &pty.Winsize{Rows: 30, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	ft := &fakeTerminal{t: t, master: m, slave: s}
	go func() { _, _ = io.Copy(ft, m) }()
	t.Cleanup(func() { _ = m.Close(); _ = s.Close() })
	return ft
}

func (f *fakeTerminal) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.screen.Write(p)
}

func (f *fakeTerminal) shown() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.screen.String()
}

func (f *fakeTerminal) type_(s string) {
	f.t.Helper()
	if _, err := io.WriteString(f.master, s); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeTerminal) waitFor(sub string) {
	f.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(f.shown(), sub) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatalf("timeout waiting for %q on the screen:\n%s", sub, f.shown())
}

func setup(t *testing.T) (*fakeTerminal, Options) {
	t.Helper()
	t.Setenv(EnvRec, "")
	dir := t.TempDir()
	if err := os.MkdirAll(session.MetaDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	ft := newTerminal(t)
	return ft, Options{
		Session: "acme", Dir: dir, In: ft.slave, Out: ft.slave, Msg: io.Discard,
		Env: append(os.Environ(), "TERM=xterm", "PS1=$ "),
	}
}

func run(t *testing.T, o Options) Result {
	t.Helper()
	type out struct {
		r   Result
		err error
	}
	ch := make(chan out, 1)
	go func() {
		r, err := Run(o)
		ch <- out{r, err}
	}()
	select {
	case v := <-ch:
		if v.err != nil {
			t.Fatalf("Run: %v", v.err)
		}
		return v.r
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not finish")
		return Result{}
	}
}

func readCast(t *testing.T, path string) (cast.Header, []cast.Event) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := cast.NewReader(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return r.Header, r.ReadAll()
}

func outputOf(evs []cast.Event) string {
	var b strings.Builder
	for _, e := range evs {
		if e.Code == cast.CodeOutput {
			b.WriteString(e.Data)
		}
	}
	return b.String()
}

func TestRecordsOutputAndPropagatesExitStatus(t *testing.T) {
	ft, o := setup(t)
	o.Command = []string{"/bin/sh", "-c", "echo hello-from-the-shell; exit 7"}
	res := run(t, o)

	if res.ExitCode != 7 {
		t.Errorf("exit code = %d, want 7", res.ExitCode)
	}
	if len(res.Files) != 1 || !strings.HasSuffix(res.Files[0], res.ID+".cast") {
		t.Fatalf("files = %v", res.Files)
	}
	h, evs := readCast(t, res.Files[0])
	if h.Version != 2 || h.Width != 100 || h.Height != 30 || h.Title != "arsenal: acme" || h.Timestamp == 0 || h.Command != "/bin/sh -c echo hello-from-the-shell; exit 7" {
		t.Errorf("header = %+v", h)
	}
	if !strings.Contains(outputOf(evs), "hello-from-the-shell") {
		t.Errorf("cast output = %q", outputOf(evs))
	}
	ft.waitFor("hello-from-the-shell")
	for _, e := range evs {
		if e.Code == cast.CodeInput {
			t.Error("keystrokes recorded without being asked")
		}
	}

	fi, _ := os.Stat(res.Files[0])
	di, _ := os.Stat(session.RecordingsDir(o.Dir))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Errorf("modes: file %v folder %v, want 0600 and 0700", fi.Mode().Perm(), di.Mode().Perm())
	}

	recs, _ := cmdlog.Load(cmdlog.Path(o.Dir), 0)
	if len(recs) != 2 || recs[0].Ev != cmdlog.EvRecStart || recs[1].Ev != cmdlog.EvRecEnd {
		t.Fatalf("command log = %+v", recs)
	}
	if recs[0].Rec != res.ID || recs[0].Session != "acme" || recs[1].Exit == nil || *recs[1].Exit != 7 || recs[1].Dur == nil {
		t.Errorf("rec lines: %+v / %+v", recs[0], recs[1])
	}
}

func TestChildSeesThePinAndTheRecordingIDs(t *testing.T) {
	_, o := setup(t)
	o.Env = append(o.Env, "ARSENAL_PAUSED=1", "ARSENAL_SESSION=other", "ARSENAL_CD_FILE=/tmp/stale")
	o.Command = []string{"/bin/sh", "-c", `echo "S=$ARSENAL_SESSION D=$ARSENAL_SESSION_DIR R=$ARSENAL_REC I=$ARSENAL_REC_ID P=[$ARSENAL_PAUSED] C=[$ARSENAL_CD_FILE]"`}
	res := run(t, o)
	_, evs := readCast(t, res.Files[0])
	got := outputOf(evs)
	for _, want := range []string{"S=acme", "D=" + o.Dir, "R=" + res.Files[0], "I=" + res.ID, "P=[]", "C=[]"} {
		if !strings.Contains(got, want) {
			t.Errorf("child environment lacks %q: %q", want, got)
		}
	}
}

func TestKeystrokesGoToTheShellButAreRecordedOnlyOnRequest(t *testing.T) {
	for _, withInput := range []bool{false, true} {
		ft, o := setup(t)
		o.Input = withInput
		o.Command = []string{"/bin/cat"}
		done := make(chan Result, 1)
		go func() { r, _ := Run(o); done <- r }()
		time.Sleep(300 * time.Millisecond)
		ft.type_("abc-typed\n")
		ft.waitFor("abc-typed")
		ft.type_("\x04")
		var res Result
		select {
		case res = <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("recording did not end after Ctrl-D")
		}
		_, evs := readCast(t, res.Files[0])
		var inputs int
		for _, e := range evs {
			if e.Code == cast.CodeInput {
				inputs++
			}
		}
		if !strings.Contains(outputOf(evs), "abc-typed") {
			t.Errorf("input=%v: the echo of the typed text is not in the cast", withInput)
		}
		if withInput && inputs == 0 || !withInput && inputs != 0 {
			t.Errorf("input=%v but %d input events", withInput, inputs)
		}
	}
}

func TestResizeIsForwardedAndRecorded(t *testing.T) {
	ft, o := setup(t)
	sigs := make(chan os.Signal, 1)
	o.Signals = sigs
	o.Command = []string{"/bin/sh", "-c", "sleep 0.6; stty size"}
	done := make(chan Result, 1)
	go func() { r, _ := Run(o); done <- r }()
	time.Sleep(250 * time.Millisecond)
	if err := pty.Setsize(ft.master, &pty.Winsize{Rows: 45, Cols: 140}); err != nil {
		t.Fatal(err)
	}
	sigs <- syscall.SIGWINCH
	res := <-done
	_, evs := readCast(t, res.Files[0])
	var found bool
	for _, e := range evs {
		if c, r, ok := e.Resize(); ok && c == 140 && r == 45 {
			found = true
		}
	}
	if !found {
		t.Errorf("no resize event 140x45 in %+v", evs)
	}
	if !strings.Contains(outputOf(evs), "45 140") {
		t.Errorf("the shell did not see the new size: %q", outputOf(evs))
	}
}

func TestHangupEndsTheShellAndKeepsAValidCast(t *testing.T) {
	_, o := setup(t)
	sigs := make(chan os.Signal, 1)
	o.Signals = sigs
	o.Command = []string{"/bin/sh", "-c", "echo before-hangup; sleep 30"}
	done := make(chan Result, 1)
	go func() { r, _ := Run(o); done <- r }()
	time.Sleep(400 * time.Millisecond)
	sigs <- syscall.SIGHUP
	var res Result
	select {
	case res = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the recorder did not end after SIGHUP")
	}
	if res.ExitCode != 128+int(syscall.SIGHUP) {
		t.Errorf("exit code = %d, want %d", res.ExitCode, 128+int(syscall.SIGHUP))
	}
	_, evs := readCast(t, res.Files[0])
	if !strings.Contains(outputOf(evs), "before-hangup") {
		t.Errorf("output lost: %q", outputOf(evs))
	}
	recs, _ := cmdlog.Load(cmdlog.Path(o.Dir), 0)
	if len(recs) != 2 || recs[1].Ev != cmdlog.EvRecEnd {
		t.Errorf("rec_end missing: %+v", recs)
	}
}

func TestBackgroundProcessHoldingTheTerminalDoesNotHangTheRecorder(t *testing.T) {
	_, o := setup(t)
	o.Command = []string{"/bin/sh", "-c", "(sleep 30 &) ; echo shell-done"}
	start := time.Now()
	res := run(t, o)
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
	_, evs := readCast(t, res.Files[0])
	if !strings.Contains(outputOf(evs), "shell-done") {
		t.Errorf("output = %q", outputOf(evs))
	}
}

func TestBigRecordingRotatesWithoutLosingOutput(t *testing.T) {
	_, o := setup(t)
	o.MaxSize = 3000
	o.Command = []string{"/bin/sh", "-c", `i=0; while [ $i -lt 400 ]; do echo "line-$i-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"; i=$((i+1)); done`}
	res := run(t, o)
	if len(res.Files) < 2 {
		t.Fatalf("files = %v, want a rotation", res.Files)
	}
	var all strings.Builder
	for i, p := range res.Files {
		h, evs := readCast(t, p)
		if h.Width != 100 || h.Height != 30 {
			t.Errorf("%s: header %+v", p, h)
		}
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v", p, fi.Mode().Perm())
		}
		if i > 0 && !strings.HasSuffix(p, fmt.Sprintf("-%03d.cast", i+1)) {
			t.Errorf("file name %s", p)
		}
		all.WriteString(outputOf(evs))
	}
	for i := 0; i < 400; i++ {
		if !strings.Contains(all.String(), "line-"+itoa(i)+"-xxxx") {
			t.Fatalf("line %d is missing from the recording", i)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

func TestRefusesNestedRecordingAndNonTerminal(t *testing.T) {
	_, o := setup(t)
	t.Setenv(EnvRec, "/x/y.cast")
	if _, err := Run(o); err == nil || !strings.Contains(err.Error(), "already being recorded") || !strings.Contains(err.Error(), "/x/y.cast") {
		t.Errorf("nested: %v", err)
	}
	t.Setenv(EnvRec, "")
	f, _ := os.Open(os.DevNull)
	defer f.Close()
	o.In = f
	if _, err := Run(o); err != ErrNotTerminal {
		t.Errorf("not a terminal: %v", err)
	}
	if _, err := os.Stat(session.RecordingsDir(o.Dir)); err == nil {
		t.Error("a folder was created although nothing was recorded")
	}
}

func TestNoLogWritesNoLinesAndTerminalIsRestored(t *testing.T) {
	ft, o := setup(t)
	o.NoLog = true
	before, err := unix.IoctlGetTermios(int(ft.slave.Fd()), ioctlGet)
	if err != nil {
		t.Fatal(err)
	}
	o.Command = []string{"/bin/sh", "-c", "true"}
	run(t, o)
	after, err := unix.IoctlGetTermios(int(ft.slave.Fd()), ioctlGet)
	if err != nil {
		t.Fatal(err)
	}
	if *before != *after {
		t.Errorf("terminal settings changed:\nbefore %+v\nafter  %+v", before, after)
	}
	if _, err := os.Stat(cmdlog.Path(o.Dir)); err == nil {
		t.Error("commands.jsonl written although NoLog is set")
	}
}

func TestInteractiveShellIsRecordedLikeAUserWouldUseIt(t *testing.T) {
	ft, o := setup(t)
	o.Shell = "/bin/sh"
	done := make(chan Result, 1)
	go func() { r, _ := Run(o); done <- r }()
	time.Sleep(400 * time.Millisecond)
	ft.type_("echo $((6*7)) in-the-shell\n")
	ft.waitFor("42 in-the-shell")
	ft.type_("exit 3\n")
	var res Result
	select {
	case res = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("did not end after exit")
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code %d", res.ExitCode)
	}
	_, evs := readCast(t, res.Files[0])
	if !strings.Contains(outputOf(evs), "42 in-the-shell") {
		t.Errorf("output = %q", outputOf(evs))
	}
}

func TestMarkersAndPauseAreConsumedByTheRecorder(t *testing.T) {
	ft, o := setup(t)
	script := `echo before-pause; printf '` + shEscape(Sequence(VerbMarker, "ars: prompt: scan specific ports")) + `'; ` +
		`printf '` + shEscape(Sequence(VerbPause, "")) + `'; echo hidden-from-the-cast; ` +
		`printf '` + shEscape(Sequence(VerbResume, "")) + `'; echo after-resume`
	o.Command = []string{"/bin/sh", "-c", script}
	res := run(t, o)
	_, evs := readCast(t, res.Files[0])
	got := outputOf(evs)
	if !strings.Contains(got, "before-pause") || !strings.Contains(got, "after-resume") {
		t.Errorf("cast output = %q", got)
	}
	if strings.Contains(got, "hidden-from-the-cast") {
		t.Error("output written while paused")
	}
	if strings.Contains(got, "7777") || strings.Contains(ft.shown(), "7777") {
		t.Errorf("our private sequence leaked: cast %q screen %q", got, ft.shown())
	}
	ft.waitFor("hidden-from-the-cast")
	var labels []string
	for _, e := range evs {
		if e.Code == cast.CodeMarker {
			labels = append(labels, e.Data)
		}
	}
	want := "ars: prompt: scan specific ports|recording paused|recording resumed"
	if strings.Join(labels, "|") != want {
		t.Errorf("markers = %q, want %q", labels, want)
	}
}

func shEscape(s string) string {
	var b strings.Builder
	for _, r := range []byte(s) {
		switch {
		case r == 0x1b:
			b.WriteString(`\033`)
		case r == 0x07:
			b.WriteString(`\007`)
		case r == '\'' || r == '%' || r == '\\':
			b.WriteString(`\` + string(rune('0'+(r>>6)&7)) + string(rune('0'+(r>>3)&7)) + string(rune('0'+r&7)))
		default:
			b.WriteByte(r)
		}
	}
	return b.String()
}
