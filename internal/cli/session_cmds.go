// ars init creates or resumes a session, and ars sessions lists them.

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

func newInitCmd() *cobra.Command {
	var resume, withRec, noRec, verbose bool
	var rf recFlags
	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Create or resume a session",
		Long:  "Creates a folder for the engagement under your pentest folder and goes into it, or resumes it if it exists.",
		Example: "  ars init acme            # new session\n" +
			"  ars init acme --rec      # also record the terminal\n" +
			"  ars init acme -r         # resume without asking",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				if !isInteractive() {
					return errors.New("session name required: run ars init <name>")
				}
				var err error
				name, err = askText("Session name", "Creates a folder under "+pentestPathHint(), "e.g. acme-internal", session.ValidateName)
				if err != nil {
					return err
				}
			}
			if err := session.ValidateName(name); err != nil {
				return err
			}
			if name == "zsh" || name == "bash" {
				return fmt.Errorf("your shell startup file runs an old command (ars init %s): run ars setup to fix it", name)
			}
			if err := refuseInsideRecording(name); err != nil {
				return err
			}
			wantRec := withRec
			if !withRec && !noRec {
				on, err := config.RecByDefault()
				if err != nil {
					return err
				}
				wantRec = on
			}
			if noRec {
				wantRec = false
			}

			rf.announce = recAnnouncer(out, verbose)
			if migratedNow[name] {
				resume = true
			}

			folder, isSession, err := session.FolderExists(name)
			if err != nil {
				return err
			}
			if folder {
				return resumeExisting(cmd, name, isSession, resume, wantRec, rf)
			}

			dir, err := session.Create(name)
			if errors.Is(err, session.ErrExists) {
				return resumeExisting(cmd, name, true, resume, wantRec, rf)
			}
			if err != nil {
				return err
			}
			ensureCommandLog(cmd, dir)
			if err := showInit(out, initInfo{New: true, Name: name, Dir: dir, RecOn: wantRec}); err != nil {
				return err
			}
			if err := requestCD(dir); err != nil {
				return err
			}
			if wantRec {
				return startRecording(cmd, session.Info{Name: name, Dir: dir}, dir, nil, rf)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&resume, "resume", "r", false, "resume an existing session without asking")
	cmd.Flags().BoolVar(&withRec, "rec", false, "record the terminal in this session")
	cmd.Flags().BoolVar(&noRec, "no-rec", false, "do not record, even if recording is on by default")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show where the recording is saved")
	addRecFlags(cmd, &rf)
	return cmd
}

func showInit(out io.Writer, in initInfo) error {
	hints, err := config.Hints()
	if err != nil {
		return err
	}
	in.Hints = hints
	in.Integration = os.Getenv(EnvCDFile) != ""
	in.RCInstalled = !in.Integration && rcHasIntegration()
	return printInit(out, in)
}

func recAnnouncer(out io.Writer, verbose bool) func(string) {
	return func(castPath string) {
		if verbose {
			fmt.Fprintln(out, tui.Muted("Recording file: "+castPath))
		}
	}
}

func ensureCommandLog(cmd *cobra.Command, dir string) {
	if _, err := cmdlog.Ensure(dir); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), tui.Warn("cannot create the command log: "+err.Error()))
	}
}

func pentestPathHint() string {
	if p, err := config.PentestPath(); err == nil {
		return p
	}
	return "$PENTEST_PATH"
}

func resumeExisting(cmd *cobra.Command, name string, isSession, resume, wantRec bool, rf recFlags) error {
	out := cmd.OutOrStdout()
	dir, err := session.Dir(name)
	if err != nil {
		return err
	}
	if !resume {
		if !isInteractive() {
			if !isSession {
				return fmt.Errorf("%s exists but is not a session: run ars init %s --resume to use it (only a %s folder is added)", dir, name, session.MetaDirName)
			}
			return fmt.Errorf("session %s already exists: cd %s to use it, or pass --resume", name, dir)
		}
		q, d := fmt.Sprintf("Session %q already exists. Resume it?", name), "Its contents will not be changed."
		if !isSession {
			q, d = fmt.Sprintf("%s exists but is not a session. Use it as one?", dir), "Only a "+session.MetaDirName+" folder is added; nothing else in it is changed."
		}
		yes, err := confirm(q, d)
		if err != nil {
			return err
		}
		if !yes {
			fmt.Fprintln(out, tui.Muted("Nothing changed."))
			return nil
		}
	}
	if !isSession {
		if _, err := session.Adopt(name); err != nil {
			return err
		}
	}
	info, err := session.ByName(name)
	if err != nil {
		return err
	}
	ensureCommandLog(cmd, info.Dir)
	if err := showInit(out, initInfo{Name: info.Name, Dir: info.Dir, RecOn: wantRec}); err != nil {
		return err
	}
	if err := requestCD(info.Dir); err != nil {
		return err
	}
	if wantRec {
		return startRecording(cmd, info, info.Dir, nil, rf)
	}
	return nil
}

func newSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sessions",
		Short: "List your sessions",
		Long:  "Lists every session and marks the current one.",
		Example: "  ars sessions\n" +
			"  ars init acme   # create another one",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			list, err := session.List()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Fprintln(out, tui.Muted("No sessions yet: create one with ars init <name>"))
				return nil
			}
			here, _, err := session.Current()
			if err != nil {
				return err
			}
			fmt.Fprintln(out, tui.Heading("Sessions"))
			for _, s := range list {
				if here.Dir != "" && sameDir(s.Dir, here.Dir) {
					fmt.Fprintf(out, "%s %s %s\n", tui.ActiveStyle.Render(">"), tui.ActiveStyle.Render(s.Name), tui.Muted("(this folder)"))
				} else {
					fmt.Fprintf(out, "  %s\n", s.Name)
				}
			}
			return nil
		},
	}
}

func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}
