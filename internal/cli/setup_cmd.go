// ars setup is the wizard for the pentest folder, the cheats, recording and the shell line.

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal"
	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/setup"
	"github.com/ynn3m/arsenal/internal/tui"
)

var (
	runSteps     = tui.RunSteps
	officialPack = arsenal.Cheats()
	lookPath     = exec.LookPath
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Run the setup wizard",
		Long:  "Runs the setup wizard for Arsenal. Run it again to change the setup.",
		Example: "  ars setup       # first-time setup (it also starts by itself)\n" +
			"  ars init acme   # then start your first session",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isInteractive() {
				return errors.New("setup needs a terminal: run ars setup in an interactive shell")
			}
			return runSetup(cmd.OutOrStdout())
		},
	}
}

const (
	optStartEmpty    = "Start empty"
	optKeepAsIs      = "Keep as it is"
	optKeepThem      = "Keep them"
	optAddArsenal    = "Add the Arsenal commands (~1100)"
	optRemoveArsenal = "Remove the Arsenal commands (yours stay)"
	optSkip          = "Skip"
	optRecOnDemand   = "Only when I run ars rec"
	optRecAlways     = "Always, on every ars init"
)

const (
	cheatsKeep   = 0
	cheatsChange = 1
)

type setupPlan struct {
	home      string
	cheatsDir string
	arsenal   int
	own       int

	shell     string
	rc        string
	line      string
	pathDir   string
	rcState   setup.RCState
	shellNote string
	askShell  bool

	box tui.StepBox
}

func runSetup(out io.Writer) error {
	p, err := newSetupPlan(out)
	if err != nil {
		return err
	}
	answers, err := runSteps(p.box)
	if err != nil {
		return err
	}
	if err := p.apply(out, answers); err != nil {
		return err
	}
	fmt.Fprintln(out, "\nRestart the terminal, then:  "+tui.Title("ars init <name>"))
	return nil
}

func newSetupPlan(out io.Writer) (*setupPlan, error) {
	p := &setupPlan{}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	p.home = home

	current, err := config.PentestPath()
	if err != nil {
		return nil, err
	}
	if env := os.Getenv(config.EnvPentestPath); env != "" {
		fmt.Fprintln(out, tui.Warn("$PENTEST_PATH is set ("+env+") and overrides the folder chosen here. Unset it to use your choice."))
	}
	f, err := config.LoadFile()
	if err != nil {
		return nil, err
	}
	def := f.PentestPath
	if def == "" {
		def = tildePath(current)
	}

	if p.cheatsDir, err = config.CheatsDir(); err != nil {
		return nil, err
	}
	p.arsenal, p.own = cheats.Count(p.cheatsDir)
	firstRun := true
	if exists, err := config.FileExists(); err == nil {
		firstRun = !exists
	}
	cheatsHelp, cheatOptions := p.cheatsChoices(firstRun)

	recDef := 0
	if f.RecByDefault {
		recDef = 1
	}

	p.planShell()

	steps := []tui.Step{
		{Key: "folder", Title: "Pentest folder", Help: "every session is a folder under it", Kind: tui.StepPath,
			Default: def, Validate: validatePentestPath},
		{Key: "cheats", Title: "Cheatsheets", Help: cheatsHelp, Kind: tui.StepChoice,
			Options: cheatOptions},
		{Key: "rec", Title: "Recording", Help: "when to record your terminal", Kind: tui.StepChoice,
			Options: []string{optRecOnDemand, optRecAlways}, DefaultOption: recDef},
	}
	if p.askShell {
		steps = append(steps, p.shellStep())
	}
	p.box = tui.StepBox{Title: "Setup", Steps: steps, Summary: p.summary}
	return p, nil
}

func (p *setupPlan) cheatsChoices(firstRun bool) (help string, options []string) {
	switch {
	case p.arsenal > 0:
		return fmt.Sprintf("%s, %d of yours", plural(p.arsenal, "Arsenal command", "Arsenal commands"), p.own), []string{optKeepThem, optRemoveArsenal}
	case p.own > 0:
		return fmt.Sprintf("No Arsenal commands. %s of yours.", plural(p.own, "command", "commands")), []string{optKeepAsIs, optAddArsenal}
	case firstRun:
		return "No commands yet. Add your own later with ars add", []string{optStartEmpty, optAddArsenal}
	}
	return "No commands yet. Add your own later with ars add", []string{optKeepAsIs, optAddArsenal}
}

func (p *setupPlan) planShell() {
	p.shell = setup.DetectShell(os.Getenv("SHELL"))
	if p.shell == "" {
		p.shellNote = "shell not detected: add this line to your shell startup file: " + setup.HookLine("zsh")
		return
	}
	p.rc = setup.RCFile(p.shell, p.home, os.Getenv("ZDOTDIR"))
	p.line = setup.HookLine(p.shell)

	inst, found := setup.FindInstall(lookPath, p.home)
	if !found {
		p.shellNote = "not added: ars is not in your PATH. Run make install, then ars setup again"
		return
	}
	if !inst.InPath {
		p.pathDir = inst.Dir
	}
	content, err := setup.ReadRC(p.rc)
	if err != nil {
		p.shellNote = "cannot read " + tildePath(p.rc) + ": " + err.Error()
		return
	}
	p.rcState = setup.AnalyzeRC(content, p.shell)
	if !p.rcState.Needs() && setup.PlanRC(content, p.shell, p.pathDir, p.home) == content {
		p.shellNote = "already in " + tildePath(p.rc)
		return
	}
	p.askShell = true
}

func (p *setupPlan) shellStep() tui.Step {
	rc := tildePath(p.rc)
	if len(p.rcState.Old) > 0 {
		return tui.Step{Key: "shell", Title: "Shell integration", Kind: tui.StepChoice,
			Help:    "replaces an old line: " + strings.Join(p.rcState.Old, " ; ") + " (backup: " + rc + ".arsenal-bak)",
			Options: []string{"Replace in " + rc, optSkip}}
	}
	help := "adds: " + p.line
	if p.pathDir != "" {
		help += "  (and the PATH line for " + tildePath(p.pathDir) + ")"
	}
	return tui.Step{Key: "shell", Title: "Shell integration", Help: help, Kind: tui.StepChoice,
		Options: []string{"Add to " + rc, optSkip}}
}

func (p *setupPlan) summary(a tui.Answers) []string {
	cheatsText := a.Text("cheats")
	if a.Choice("cheats") == cheatsChange && p.arsenal > 0 {
		cheatsText = fmt.Sprintf("Remove %s, keep %d of yours", plural(p.arsenal, "command", "commands"), p.own)
	}
	shellText := p.shellNote
	if p.askShell {
		shellText = a.Text("shell")
		if a.Choice("shell") == 1 {
			shellText = "skipped (not added)"
		}
	}
	lines := tui.Rows(
		[2]string{"Pentest folder", a.Text("folder")},
		[2]string{"Cheats", cheatsText},
		[2]string{"Recording", a.Text("rec")},
		[2]string{"Shell", shellText},
	)
	return lines
}

func (p *setupPlan) apply(out io.Writer, a tui.Answers) error {
	folder := a.Text("folder")
	if err := config.SetFileKeys(map[string]any{"pentest_path": folder}); err != nil {
		return err
	}
	dir, err := config.ExpandHome(folder)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, config.DirMode); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}

	if a.Choice("cheats") == cheatsChange {
		if err := p.changeArsenal(out); err != nil {
			return err
		}
	}

	if err := config.SetFileKeys(map[string]any{"rec_by_default": a.Choice("rec") == 1}); err != nil {
		return err
	}

	if p.askShell && a.Choice("shell") == 0 {
		changed, err := setup.InstallRC(p.rc, p.shell, p.pathDir, p.home)
		if err != nil {
			return fmt.Errorf("cannot write %s: %w", p.rc, err)
		}
		if changed && len(p.rcState.Old) > 0 {
			fmt.Fprintln(out, tui.Muted("Old line replaced in "+tildePath(p.rc)+" (backup: "+tildePath(p.rc)+".arsenal-bak)"))
		}
	}
	return nil
}

func validatePentestPath(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("a folder is required")
	}
	if s != "~" && !strings.HasPrefix(s, "~/") && !filepath.IsAbs(s) {
		return errors.New("use an absolute path or one starting with ~/")
	}
	return nil
}

func (p *setupPlan) changeArsenal(out io.Writer) error {
	if p.arsenal > 0 {
		res, err := cheats.RemoveArsenal(p.cheatsDir)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, tui.Success(fmt.Sprintf("Removed %s, kept %d of yours", plural(res.Removed, "command", "commands"), res.Kept)))
		return nil
	}
	res, err := setup.AddOfficial(p.cheatsDir, officialPack)
	if err != nil {
		return fmt.Errorf("cannot add the Arsenal commands: %w", err)
	}
	fmt.Fprintln(out, tui.Success(fmt.Sprintf("Added %s to %s", plural(res.Added, "command", "commands"), plural(res.Files, "file", "files"))))
	if n := conflictsNotice(res.Conflicts); n != "" {
		fmt.Fprintln(out, tui.Warn(n))
	}
	return nil
}

func conflictsNotice(conflicts []string) string {
	if len(conflicts) == 0 {
		return ""
	}
	shown := conflicts[:min(len(conflicts), 3)]
	more := ""
	if len(conflicts) > len(shown) {
		more = fmt.Sprintf(" and %d more", len(conflicts)-len(shown))
	}
	return fmt.Sprintf("%d variable defaults differ from the one already in their file and could not be written in the commands (%s%s). The default of the file is used.",
		len(conflicts), strings.Join(shown, ", "), more)
}
