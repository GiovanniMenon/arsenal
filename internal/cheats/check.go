// Checks that the program each command runs exists on this machine, like upstream --check.

package cheats

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type CheckStatus int

const (
	CheckOK CheckStatus = iota
	CheckMissing
	CheckSkipped
)

type CheckResult struct {
	Binary string
	Status CheckStatus
	Detail string
}

var checkSkipFiles = map[string]bool{"powershell.md": true, "windows.md": true, "laps.md": true, "reverse.md": true}

type Finder func(binary string) (string, bool)

func Check(list []*Cheat, find Finder) []CheckResult {
	var results []CheckResult
	seen := map[string]bool{}
	var jobs []int
	for _, c := range list {
		if checkSkipFiles[filepath.Base(c.Filename)] {
			continue
		}
		full := c.PrintableCommand()
		binary, _, _ := strings.Cut(full, " ")
		if seen[binary] {
			continue
		}
		seen[binary] = true
		if strings.Contains(binary, "<") {
			continue
		}
		if (len(binary) > 4 && strings.HasSuffix(binary, ".exe")) || strings.Contains(c.Name, "powershell") {
			results = append(results, CheckResult{Binary: binary, Status: CheckSkipped, Detail: full})
			continue
		}
		results = append(results, CheckResult{Binary: binary, Detail: full})
		jobs = append(jobs, len(results)-1)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, i := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			if where, ok := find(results[i].Binary); ok {
				results[i].Status, results[i].Detail = CheckOK, where
			} else {
				results[i].Status = CheckMissing
			}
		}(i)
	}
	wg.Wait()
	return results
}

func ShellFinder(binary string) (string, bool) {
	for _, sh := range []string{"zsh", "bash"} {
		path, err := exec.LookPath(sh)
		if err != nil {
			continue
		}
		out, err := exec.Command(path, "-i", "-c", "type -- "+shellQuote(binary)).Output()
		if err != nil {
			return "", false
		}
		first, _, _ := strings.Cut(string(out), "\n")
		return first, true
	}
	if p, err := exec.LookPath(binary); err == nil {
		return p, true
	}
	return "", false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
