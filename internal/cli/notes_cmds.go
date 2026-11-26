// ars note adds a note to the session, and ars notes browses, edits and deletes them.

package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

var (
	now      = time.Now
	runNotes = tui.RunNotes
)

func newNoteCmd() *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "note [text ...]",
		Short: "Add a note to the session",
		Long:  "Adds a note to the current session. Markdown works, and the text can also come from a pipe.",
		Example: "  ars note \"admin panel at /admin, default creds rejected\"\n" +
			"  ars note -t \"Admin panel\" \"default creds rejected, try the reset form\"\n" +
			"  nmap -sV 10.10.10.5 | ars note -t \"nmap scan\"\n" +
			"  ars note",
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := requireSession()
			if err != nil {
				return err
			}
			body := strings.TrimSpace(strings.Join(args, " "))
			if body == "" && isInteractive() {
				return noteBox(sess, title)
			}
			if body == "" {
				if title, body, err = noteFromPipe(cmd, title); err != nil {
					return err
				}
			}
			header, err := session.AddNote(sess, title, body, now())
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), tui.Success("Note added to "+sess.Name+": "+strings.TrimPrefix(header, "## ")))
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "title (default: the start of the text)")
	return cmd
}

func noteBox(sess session.Info, title string) error {
	var at time.Time
	stamp := func() time.Time {
		if at.IsZero() {
			at = now()
		}
		return at
	}
	lines := func(a tui.Answers, head string) []string {
		t := strings.TrimSpace(a.Text("title"))
		if t == "" {
			t = "(no title: the start of the text is used)"
		}
		return append(tui.Rows([2]string{"Title", t}), "", head)
	}
	box := tui.StepBox{
		Title: "Add a note",
		Steps: []tui.Step{
			{Key: "title", Title: "Title", Help: "optional", Kind: tui.StepText, Default: title},
			{Key: "text", Title: "Text", Help: "Markdown; several lines are fine", Kind: tui.StepMultiline,
				Validate: func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("the note is empty: write some text")
					}
					return nil
				}},
		},
		Summary: func(a tui.Answers) []string {
			return lines(a, "Adds a note to "+sess.Name+" at "+now().Format("15:04"))
		},
		Confirmed: func(a tui.Answers) []string {
			return lines(a, "Note added to "+sess.Name+" at "+stamp().Format("15:04"))
		},
	}
	answers, err := runSteps(box)
	if err != nil {
		return err
	}
	_, err = session.AddNote(sess, answers.Text("title"), answers.Text("text"), stamp())
	return err
}

func noteFromPipe(cmd *cobra.Command, title string) (string, string, error) {
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", "", err
	}
	body := strings.TrimSpace(string(b))
	if body == "" && strings.TrimSpace(title) == "" {
		return "", "", errors.New("no note text: run ars note \"<text>\" or pipe text in")
	}
	return title, body, nil
}

func newNotesCmd() *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "Browse, search, edit and delete the notes of the session",
		Long:  "Lists the notes of the current session, where you can search, edit and delete them.",
		Example: "  ars notes\n" +
			"  ars notes -s admin",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := requireSession()
			if err != nil {
				return err
			}
			md, err := session.ReadNotes(sess.Dir)
			if err != nil && !errors.Is(err, session.ErrNoNotes) {
				return err
			}
			if !isInteractive() {
				_, err := io.WriteString(cmd.OutOrStdout(), md)
				return err
			}
			reload := func(message string, sel int) (tui.NotesResult, error) {
				md, err := session.ReadNotes(sess.Dir)
				if err != nil && !errors.Is(err, session.ErrNoNotes) {
					return tui.NotesResult{}, err
				}
				return tui.NotesResult{Message: message, Notes: session.ParseNotes(md), Select: sel}, nil
			}
			return runNotes(tui.NotesOptions{
				Session: sess.Name,
				Path:    session.NotesPath(sess.Dir),
				Notes:   session.ParseNotes(md),
				Query:   query,
				Save: func(n session.Note, title, body string) (tui.NotesResult, error) {
					if err := session.UpdateNote(sess.Dir, n, title, body); err != nil {
						return tui.NotesResult{}, err
					}
					return reload("Note saved", n.Index)
				},
				Delete: func(n session.Note) (tui.NotesResult, error) {
					if err := session.DeleteNote(sess.Dir, n); err != nil {
						return tui.NotesResult{}, err
					}
					return reload("Note deleted", -1)
				},
			})
		},
	}
	cmd.Flags().StringVarP(&query, "search", "s", "", "start with `QUERY` in the search box")
	return cmd
}
