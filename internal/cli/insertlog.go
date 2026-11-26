// Logs the commands ars puts in the prompt to the session's command log.

package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

const (
	EnvSession = "ARSENAL_SESSION"
	EnvPaused  = "ARSENAL_PAUSED"
	EnvRecID   = "ARSENAL_REC_ID"
)

func logSession() (session.Info, bool) {
	if os.Getenv(EnvPaused) != "" {
		return session.Info{}, false
	}
	if in, ok := boundSession(); ok {
		return in, true
	}
	in, ok, err := session.Current()
	return in, ok && err == nil
}

func logInsert(cmd *cobra.Command, line, cheat string, d delivery) {
	emitMarker(d.How, cheat)
	sess, ok := logSession()
	if !ok {
		return
	}
	cwd, _ := os.Getwd()
	rec := cmdlog.Record{
		Ev:      cmdlog.EvInsert,
		TS:      time.Now().UnixMilli(),
		Session: sess.Name,
		CWD:     cwd,
		Cmd:     line,
		Rec:     os.Getenv(EnvRecID),
		Exit:    d.Exit,
		How:     d.How,
		Cheat:   cheat,
	}
	if err := cmdlog.AppendSession(sess.Name, sess.Dir, rec); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), tui.Warn("cannot write to the command log: "+err.Error()))
	}
}
