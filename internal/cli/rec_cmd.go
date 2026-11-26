// ars rec records a shell into the recordings of the session.

package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/rec"
	"github.com/ynn3m/arsenal/internal/session"
)

var runRecorder = rec.Run

type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func boundSession() (session.Info, bool) {
	if os.Getenv(rec.EnvRec) == "" {
		return session.Info{}, false
	}
	dir := os.Getenv("ARSENAL_SESSION_DIR")
	if dir == "" {
		return session.Info{}, false
	}
	in, ok, err := session.Find(dir)
	return in, ok && err == nil && in.Dir == dir
}

func hereSession() (session.Info, error) {
	if in, ok := boundSession(); ok {
		return in, nil
	}
	return requireSession()
}

func refuseInsideRecording(name string) error {
	if os.Getenv(rec.EnvRec) != "" && os.Getenv(EnvSession) != name {
		return fmt.Errorf("this terminal is recording session %s: type exit to stop, then run ars init %s", os.Getenv(EnvSession), name)
	}
	return nil
}

type recFlags struct {
	noLog    bool
	input    bool
	maxMB    int
	announce func(castPath string)
}

func startRecording(cmd *cobra.Command, sess session.Info, workDir string, command []string, f recFlags) error {
	var err error
	maxMB := f.maxMB
	if maxMB <= 0 {
		if maxMB, err = config.MaxCastMB(); err != nil {
			return err
		}
	}
	res, err := runRecorder(rec.Options{
		Session: sess.Name, Dir: sess.Dir, WorkDir: workDir, Command: command,
		NoLog: f.noLog, Input: f.input, MaxSize: int64(maxMB) << 20,
		Msg: cmd.ErrOrStderr(), Announce: f.announce,
	})
	switch {
	case errors.Is(err, rec.ErrNotTerminal):
		return errors.New("recording needs a terminal: run ars rec in an interactive shell")
	case err != nil:
		return err
	}
	if len(command) > 0 && res.ExitCode != 0 {
		return exitError{res.ExitCode}
	}
	return nil
}

func addRecFlags(cmd *cobra.Command, f *recFlags) {
	cmd.Flags().BoolVar(&f.noLog, "no-log", false, "do not add the recording to the command log")
	cmd.Flags().BoolVar(&f.input, "input", false, "also record what you type (including hidden passwords)")
	cmd.Flags().IntVar(&f.maxMB, "max-size", 0, "start a new recording file after `MB` megabytes (default 256)")
}

func newRecCmd() *cobra.Command {
	var f recFlags
	cmd := &cobra.Command{
		Use:   "rec [-- command args...]",
		Short: "Record your terminal so you can replay it",
		Long: "Starts a recorded shell in the current session, also while you are in an SSH or reverse shell.\n" +
			"Type exit to stop, then play it back with ars replay. Only what the terminal shows is recorded.",
		Example: "  ars rec\n" +
			"  ars rec -- nmap -sV 10.10.10.5   # record one command\n" +
			"  ars rec --input                  # also record your keystrokes",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := hereSession()
			if err != nil {
				return err
			}
			return startRecording(cmd, sess, "", args, f)
		},
	}
	addRecFlags(cmd, &f)
	return cmd
}
