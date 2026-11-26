package cmdlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ynn3m/arsenal/internal/session"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PENTEST_PATH", filepath.Join(t.TempDir(), "pentest"))
}

func ip(n int) *int      { return &n }
func i64(n int64) *int64 { return &n }

func TestMarshalKeyOrderAndVersion(t *testing.T) {
	b, err := Marshal(Record{Ev: EvStart, ID: "42.1", TS: 1795874531123, Session: "acme", CWD: "/w", Cmd: "nmap -sV 10.10.10.5", Sh: "zsh", PID: 42})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"ev":"start","id":"42.1","ts":1795874531123,"session":"acme","cwd":"/w","cmd":"nmap -sV 10.10.10.5","sh":"zsh","pid":42}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	b, _ = Marshal(Record{Ev: EvEnd, ID: "42.1", TS: 1, Exit: ip(0), Dur: i64(0)})
	if !strings.Contains(string(b), `"exit":0`) || !strings.Contains(string(b), `"dur":0`) {
		t.Errorf("exit 0 / dur 0 lost: %s", b)
	}
	b, _ = Marshal(Record{Ev: EvInsert, TS: 1})
	if strings.Contains(string(b), "exit") {
		t.Errorf("exit written for an insert line: %s", b)
	}
	if _, err := Marshal(Record{}); err == nil {
		t.Error("record without ev must be rejected")
	}
}

func TestMarshalTruncatesAtCharacterBoundary(t *testing.T) {
	cmd := strings.Repeat("a", MaxCmdBytes-1) + "é" + "tail"
	b, err := Marshal(Record{Ev: EvStart, TS: 1, Cmd: cmd})
	if err != nil {
		t.Fatal(err)
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Trunc != 1 || !utf8.ValidString(r.Cmd) || len(r.Cmd) > MaxCmdBytes {
		t.Errorf("trunc=%d valid=%v len=%d", r.Trunc, utf8.ValidString(r.Cmd), len(r.Cmd))
	}
	short, _ := Marshal(Record{Ev: EvStart, TS: 1, Cmd: "ls"})
	if strings.Contains(string(short), "trunc") {
		t.Error("short command marked truncated")
	}
}

func TestEnsureAndAppendModes(t *testing.T) {
	isolate(t)
	dir, err := session.Create("acme")
	if err != nil {
		t.Fatal(err)
	}
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	p, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v after Ensure, want 0600", fi.Mode().Perm())
	}
	if err := AppendSession("acme", dir, Record{Ev: EvInsert, TS: 5, Cmd: "id", How: HowPrompt}); err != nil {
		t.Fatal(err)
	}
	recs, err := Load(p, 0)
	if err != nil || len(recs) != 1 || recs[0].Session != "acme" || recs[0].How != HowPrompt {
		t.Fatalf("recs=%+v err=%v", recs, err)
	}
	if Path(dir) != p {
		t.Errorf("Path(%s) = %s, want %s", dir, Path(dir), p)
	}
}

func TestAppendCreatesFileWith0600(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	p := filepath.Join(t.TempDir(), FileName)
	if err := Append(p, Record{Ev: EvInsert, TS: 1}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestConcurrentAppendsKeepEveryLineWhole(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	const writers, each = 16, 150
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				cmd := strings.Repeat("x", 100+i*15)
				if err := Append(p, Record{Ev: EvStart, ID: "p.1", TS: int64(w*1000 + i), Cmd: cmd}); err != nil {
					t.Error(err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	raw, _ := os.ReadFile(p)
	recs, bad := Parse(raw)
	if bad != 0 || len(recs) != writers*each {
		t.Errorf("records=%d bad=%d, want %d and 0", len(recs), bad, writers*each)
	}
}

func TestParseSkipsDamagedLines(t *testing.T) {
	text := `{"v":1,"ev":"start","id":"1.1","ts":1,"cmd":"ls"}` + "\n" +
		"not json\n\n" +
		`{"v":1,"ev":"end","id":"1.1","ts":2,"exit":0,"dur":1}` + "\n" +
		`{"no":"ev"}` + "\n" +
		`{"v":1,"ev":"start","id":"1.2","ts":3,"cmd":"nc -lvnp 4444`
	recs, bad := Parse([]byte(text))
	if len(recs) != 2 || bad != 3 {
		t.Errorf("records=%d bad=%d, want 2 and 3", len(recs), bad)
	}
}

func TestLoadTailWindowDropsCutFirstLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	for i := 0; i < 200; i++ {
		if err := Append(p, Record{Ev: EvStart, ID: "9.1", TS: int64(i), Cmd: strings.Repeat("y", 80)}); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := Load(p, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) == 0 || len(recs) >= 200 {
		t.Fatalf("window returned %d records", len(recs))
	}
	if recs[len(recs)-1].TS != 199 {
		t.Errorf("newest record missing: last ts %d", recs[len(recs)-1].TS)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "none"), 0); err != nil {
		t.Errorf("missing file must not be an error: %v", err)
	}
}

func TestFoldJoinsStartAndEnd(t *testing.T) {
	recs := []Record{
		{V: 1, Ev: EvEnd, ID: "old.1", TS: 1, Exit: ip(0)},
		{V: 1, Ev: EvStart, ID: "7.1", TS: 10, Cmd: "nmap 10.10.10.5", Session: "acme"},
		{V: 1, Ev: EvStart, ID: "7.2", TS: 20, Cmd: "nc -lvnp 4444", Session: "acme"},
		{V: 1, Ev: EvEnd, ID: "7.1", TS: 30, Exit: ip(1), Dur: i64(20)},
		{V: 1, Ev: EvInsert, TS: 5, Cmd: "nmap 10.10.10.5", How: HowPrompt},
	}
	es := Fold(recs)
	if len(es) != 2 {
		t.Fatalf("entries = %d, want 2", len(es))
	}
	if !es[0].Ended() || !es[0].Failed() || es[0].Start.Cmd != "nmap 10.10.10.5" {
		t.Errorf("first entry wrong: %+v", es[0])
	}
	if d, ok := es[0].Duration(); !ok || d != 20*time.Millisecond {
		t.Errorf("duration %v %v", d, ok)
	}
	if es[1].Ended() || es[1].Failed() {
		t.Errorf("running command reported as ended/failed: %+v", es[1])
	}
	if _, ok := es[1].ExitCode(); ok {
		t.Error("exit code reported for a command without end line")
	}
	if n := len(Inserts(recs)); n != 1 {
		t.Errorf("inserts = %d", n)
	}
}

func TestFilter(t *testing.T) {
	now := time.Now()
	mk := func(cmd, sess string, ago time.Duration, exit int) Entry {
		return Entry{
			Start: Record{Ev: EvStart, ID: cmd, TS: now.Add(-ago).UnixMilli(), Cmd: cmd, Session: sess, CWD: "/w"},
			End:   &Record{Ev: EvEnd, ID: cmd, Exit: ip(exit)},
		}
	}
	es := []Entry{
		mk("nmap 10.10.10.5", "acme", 3*time.Hour, 0),
		mk("ars set target=10.10.10.5", "acme", 2*time.Hour, 0),
		mk("curl http://example.local", "acme", time.Minute, 7),
		mk("ls", "other", time.Minute, 0),
	}
	cases := []struct {
		name string
		f    Filter
		want int
	}{
		{"all", Filter{}, 4},
		{"session", Filter{Session: "acme"}, 3},
		{"since", Filter{Since: now.Add(-time.Hour)}, 2},
		{"failed", Filter{Failed: true}, 1},
		{"hide ars", Filter{HideArs: true}, 3},
		{"contains, case-insensitive", Filter{Contains: "CURL"}, 1},
		{"cwd", Filter{CWD: "/nowhere"}, 0},
	}
	for _, c := range cases {
		if got := len(Select(es, c.f)); got != c.want {
			t.Errorf("%s: %d entries, want %d", c.name, got, c.want)
		}
	}
	for _, c := range []struct {
		cmd  string
		want bool
	}{{"ars -s nmap", true}, {"command ars start x", true}, {"arsenal", true}, {"cars", false}, {"echo ars", false}, {"", false}} {
		if IsArsCommand(c.cmd) != c.want {
			t.Errorf("IsArsCommand(%q) != %v", c.cmd, c.want)
		}
	}
}

func TestFoldDerivesViaFromInsertLines(t *testing.T) {
	h := ViaWindow.Milliseconds()
	recs := []Record{
		{V: 1, Ev: EvInsert, TS: 100, Session: "acme", Cmd: "nmap 10.10.10.5", How: HowPrompt},
		{V: 1, Ev: EvStart, ID: "1.1", TS: 200, Session: "acme", Cmd: "nmap 10.10.10.5"},
		{V: 1, Ev: EvStart, ID: "1.2", TS: 300, Session: "acme", Cmd: "nmap 10.10.10.5"},
		{V: 1, Ev: EvInsert, TS: 400, Session: "acme", Cmd: "curl http://example.local"},
		{V: 1, Ev: EvStart, ID: "1.3", TS: 500, Session: "acme", Cmd: "curl -k http://example.local"},
		{V: 1, Ev: EvInsert, TS: 600, Session: "other", Cmd: "id"},
		{V: 1, Ev: EvStart, ID: "1.4", TS: 700, Session: "acme", Cmd: "id"},
		{V: 1, Ev: EvInsert, TS: 800, Session: "acme", Cmd: "whoami"},
		{V: 1, Ev: EvStart, ID: "1.5", TS: 800 + h + 1, Session: "acme", Cmd: "whoami"},
		{V: 1, Ev: EvStart, ID: "1.6", TS: 50, Session: "acme", Cmd: "nmap 10.10.10.5"},
	}
	want := map[string]string{"1.1": ViaArs, "1.2": ViaUser, "1.3": ViaUser, "1.4": ViaUser, "1.5": ViaUser, "1.6": ViaUser}
	for _, e := range Fold(recs) {
		if e.Via != want[e.Start.ID] {
			t.Errorf("%s (%q): via %q, want %q", e.Start.ID, e.Start.Cmd, e.Via, want[e.Start.ID])
		}
		if (e.Via == ViaArs) != (e.Insert != nil) {
			t.Errorf("%s: via %q but insert %v", e.Start.ID, e.Via, e.Insert)
		}
		if e.Insert != nil && e.Insert.How != HowPrompt {
			t.Errorf("%s: insert line not carried: %+v", e.Start.ID, e.Insert)
		}
	}
}
