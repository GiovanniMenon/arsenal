package insert

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type tmuxFake struct {
	calls   [][]string
	panes   string
	newPane string
	failOn  string
}

func (f *tmuxFake) install(t *testing.T) {
	t.Helper()
	old, oldSleep := tmuxRun, tmuxSleep
	tmuxRun = func(args ...string) (string, error) {
		f.calls = append(f.calls, args)
		if args[0] == f.failOn {
			return "", errors.New("boom")
		}
		switch args[0] {
		case "list-panes":
			return f.panes, nil
		case "split-window":
			return f.newPane, nil
		}
		return "", nil
	}
	tmuxSleep = func(time.Duration) {}
	t.Cleanup(func() { tmuxRun, tmuxSleep = old, oldSleep })
}

func (f *tmuxFake) verbs() []string {
	var v []string
	for _, c := range f.calls {
		v = append(v, c[0])
	}
	return v
}

func TestSendToPaneOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if err := SendToPane("ls", false); !errors.Is(err, ErrNotInTmux) {
		t.Errorf("err = %v", err)
	}
}

func TestSendToPaneSplitsASinglePaneWindow(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	t.Setenv("TMUX_PANE", "%0")
	f := &tmuxFake{panes: "%0", newPane: "%7"}
	f.install(t)

	if err := SendToPane("nmap -p- 10.10.10.5", false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.verbs(), []string{"list-panes", "split-window", "send-keys", "select-pane"}) {
		t.Fatalf("calls = %v", f.calls)
	}
	if !contains(f.calls[1], "-d") {
		t.Errorf("split-window = %v", f.calls[1])
	}
	if want := []string{"send-keys", "-t", "%7", "-l", "--", "nmap -p- 10.10.10.5"}; !reflect.DeepEqual(f.calls[2], want) {
		t.Errorf("send-keys = %v, want %v", f.calls[2], want)
	}
	if want := []string{"select-pane", "-t", "%7"}; !reflect.DeepEqual(f.calls[3], want) {
		t.Errorf("select-pane = %v", f.calls[3])
	}
}

func TestSendToPaneUsesTheLastOtherPane(t *testing.T) {
	t.Setenv("TMUX", "x")
	t.Setenv("TMUX_PANE", "%0")
	f := &tmuxFake{panes: "%0\n%1\n%2"}
	f.install(t)
	if err := SendToPane("id", false); err != nil {
		t.Fatal(err)
	}
	if contains(f.verbs(), "split-window") {
		t.Error("must not split when there is another pane")
	}
	if f.calls[1][2] != "%2" {
		t.Errorf("target = %v", f.calls[1])
	}

	t.Setenv("TMUX_PANE", "%2")
	f.calls = nil
	if err := SendToPane("id", false); err != nil {
		t.Fatal(err)
	}
	if f.calls[1][2] != "%1" {
		t.Errorf("target = %v", f.calls[1])
	}
}

func TestSendToPaneExecuteKeepsFocusAndPressesEnter(t *testing.T) {
	t.Setenv("TMUX", "x")
	t.Setenv("TMUX_PANE", "%0")
	f := &tmuxFake{panes: "%0\n%1"}
	f.install(t)
	if err := SendToPane("Enter C-c", true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.verbs(), []string{"list-panes", "send-keys", "send-keys"}) {
		t.Fatalf("calls = %v", f.calls)
	}
	if f.calls[1][5] != "Enter C-c" || !contains(f.calls[1], "-l") {
		t.Errorf("text must be literal: %v", f.calls[1])
	}
	if want := []string{"send-keys", "-t", "%1", "Enter"}; !reflect.DeepEqual(f.calls[2], want) {
		t.Errorf("enter = %v", f.calls[2])
	}
}

func TestSendToPanePropagatesTmuxErrors(t *testing.T) {
	t.Setenv("TMUX", "x")
	t.Setenv("TMUX_PANE", "%0")
	for _, verb := range []string{"list-panes", "send-keys"} {
		f := &tmuxFake{panes: "%0\n%1", failOn: verb}
		f.install(t)
		if err := SendToPane("id", false); err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("%s: err = %v", verb, err)
		}
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
