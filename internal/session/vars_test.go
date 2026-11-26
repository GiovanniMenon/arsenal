package session

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ynn3m/arsenal/internal/config"
)

func TestParsePair(t *testing.T) {
	k, v, err := ParsePair("target=10.10.10.5")
	if err != nil || k != "target" || v != "10.10.10.5" {
		t.Fatalf("got %q %q %v", k, v, err)
	}
	k, v, err = ParsePair("url=http://example.local/?a=b")
	if err != nil || k != "url" || v != "http://example.local/?a=b" {
		t.Fatalf("got %q %q %v", k, v, err)
	}
	for _, bad := range []string{"novalue", "=x", "a b=c", "1a=x", "a-b=c", "empty=", "k=a\nb"} {
		if _, _, err := ParsePair(bad); err == nil {
			t.Errorf("ParsePair(%q) = nil error", bad)
		}
	}
}

func TestVarsRoundTripSortedAnd0600(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")

	v, err := LoadVars(acme.Dir)
	if err != nil || len(v) != 0 {
		t.Fatalf("empty: %v, %v", v, err)
	}

	if err := SetVars(acme.Dir, Vars{"target": "10.10.10.5", "domain": "example.local"}); err != nil {
		t.Fatal(err)
	}
	if err := SetVars(acme.Dir, Vars{"target": "10.10.10.6", "ports": "22,80"}); err != nil {
		t.Fatal(err)
	}

	got, err := LoadVars(acme.Dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Vars{"target": "10.10.10.6", "domain": "example.local", "ports": "22,80"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	p := MarkerPath(acme.Dir)
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "vars:\n    domain: example.local\n    ports: 22,80\n    target: 10.10.10.6\n") || !strings.Contains(string(b), "name: acme\n") {
		t.Fatalf("file content: %q", b)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", fi.Mode().Perm())
	}
}

func TestUnsetVars(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")
	if err := SetVars(acme.Dir, Vars{"a": "1", "b": "2"}); err != nil {
		t.Fatal(err)
	}
	missing, err := UnsetVars(acme.Dir, []string{"a", "zzz"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(missing, []string{"zzz"}) {
		t.Fatalf("missing = %v", missing)
	}
	got, _ := LoadVars(acme.Dir)
	if !reflect.DeepEqual(got, Vars{"b": "2"}) {
		t.Fatalf("got %v", got)
	}
}

func TestBrokenMarkerIsReportedAndNeverRewritten(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")
	p := MarkerPath(acme.Dir)
	if err := os.WriteFile(p, []byte("name: acme\nvars: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVars(acme.Dir); err == nil || !strings.Contains(err.Error(), ".arsenal") {
		t.Fatalf("expected an error naming the file, got %v", err)
	}
	if err := SetVars(acme.Dir, Vars{"x": "1"}); err == nil {
		t.Fatal("SetVars should fail on a broken marker")
	}
	if b, _ := os.ReadFile(p); string(b) != "name: acme\nvars: [unclosed\n" {
		t.Fatalf("file was modified: %q", b)
	}
}

func TestMarkerKeepsUnknownKeysAndNumericLookingValues(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")
	if err := os.WriteFile(MarkerPath(acme.Dir), []byte("name: acme\ncreated: 2026-11-24\nclient: example corp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetVars(acme.Dir, Vars{"ports": "22", "flag": "true", "target": "10.10.10.5"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(MarkerPath(acme.Dir))
	if !strings.Contains(string(b), "client: example corp") || !strings.Contains(string(b), "created: 2026-11-24") {
		t.Errorf("a key was lost:\n%s", b)
	}
	got, err := LoadVars(acme.Dir)
	if err != nil || !reflect.DeepEqual(got, Vars{"ports": "22", "flag": "true", "target": "10.10.10.5"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestSetVarsNeedsASessionFolder(t *testing.T) {
	isolate(t)
	if err := SetVars(filepath.Join(t.TempDir(), "ghost"), Vars{"a": "1"}); err == nil {
		t.Fatal("expected an error for a folder without a marker")
	}
}

func TestGlobalVarsAndResolve(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")

	r, err := Resolve(acme.Dir)
	if err != nil || len(r.Vars) != 0 {
		t.Fatalf("empty: %v, %v", r, err)
	}

	if err := SetGlobalVars(Vars{"target": "10.10.10.1", "proxy": "p"}); err != nil {
		t.Fatal(err)
	}
	if err := SetVars(acme.Dir, Vars{"target": "10.10.10.5", "ports": "22"}); err != nil {
		t.Fatal(err)
	}

	r, err = Resolve(acme.Dir)
	if err != nil {
		t.Fatal(err)
	}
	wantVars := Vars{"target": "10.10.10.5", "proxy": "p", "ports": "22"}
	if !reflect.DeepEqual(r.Vars, wantVars) {
		t.Fatalf("vars = %v, want %v", r.Vars, wantVars)
	}
	wantSrc := map[string]Source{"target": SourceSession, "proxy": SourceGlobal, "ports": SourceSession}
	if !reflect.DeepEqual(r.Source, wantSrc) {
		t.Fatalf("sources = %v, want %v", r.Source, wantSrc)
	}

	r, _ = Resolve("")
	if !reflect.DeepEqual(r.Vars, Vars{"target": "10.10.10.1", "proxy": "p"}) {
		t.Fatalf("globals only: %v", r.Vars)
	}

	p, _ := config.GlobalVarsPath()
	fi, err := os.Stat(p)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("global file: %v, %v", fi, err)
	}

	missing, err := UnsetGlobalVars([]string{"proxy", "nope"})
	if err != nil || !reflect.DeepEqual(missing, []string{"nope"}) {
		t.Fatalf("unset global: %v, %v", missing, err)
	}
	g, _ := LoadGlobalVars()
	if !reflect.DeepEqual(g, Vars{"target": "10.10.10.1"}) {
		t.Fatalf("globals = %v", g)
	}
}
