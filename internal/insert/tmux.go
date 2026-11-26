// Sends a command to the other tmux pane, for the -t flag.

package insert

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var ErrNotInTmux = errors.New("not inside tmux: run ars in a tmux window to use -t")

var tmuxRun = func(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s != "" {
			return s, fmt.Errorf("tmux %s: %s", args[0], s)
		}
		return s, fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return s, nil
}

var tmuxSleep = time.Sleep

func SendToPane(line string, execute bool) error {
	if getenv("TMUX") == "" {
		return ErrNotInTmux
	}
	self := getenv("TMUX_PANE")
	var target []string
	if self != "" {
		target = []string{"-t", self}
	}

	list, err := tmuxRun(append([]string{"list-panes", "-F", "#{pane_id}"}, target...)...)
	if err != nil {
		return err
	}
	var panes []string
	for _, p := range strings.Fields(list) {
		if p != self {
			panes = append(panes, p)
		}
	}

	var pane string
	if len(panes) == 0 {
		pane, err = tmuxRun(append([]string{"split-window", "-d", "-P", "-F", "#{pane_id}"}, target...)...)
		if err != nil {
			return err
		}
		tmuxSleep(300 * time.Millisecond)
	} else {
		pane = panes[len(panes)-1]
	}

	if _, err := tmuxRun("send-keys", "-t", pane, "-l", "--", line); err != nil {
		return err
	}
	if execute {
		_, err = tmuxRun("send-keys", "-t", pane, "Enter")
		return err
	}
	_, err = tmuxRun("select-pane", "-t", pane)
	return err
}
