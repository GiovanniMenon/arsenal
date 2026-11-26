// Saves and deletes entries edited from the search screen.

package cli

import (
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/tui"
)

type entryEditor struct {
	dir  string
	list *[]*cheats.Cheat
}

func newEntryEditor(list *[]*cheats.Cheat) (*entryEditor, error) {
	dir, err := config.CheatsDir()
	if err != nil {
		return nil, err
	}
	return &entryEditor{dir: dir, list: list}, nil
}

func (e *entryEditor) editable(c *cheats.Cheat) bool {
	if c.Level != 2 || c.Line == 0 || !strings.EqualFold(filepath.Ext(c.Filename), ".md") {
		return false
	}
	rel, err := filepath.Rel(realPathOf(e.dir), realPathOf(c.Filename))
	return err == nil && !strings.HasPrefix(rel, "..")
}

func realPathOf(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func (e *entryEditor) readOnly(c *cheats.Cheat) bool { return !e.editable(c) }

func (e *entryEditor) reload() ([]*cheats.Cheat, error) {
	list, err := loadCheats(io.Discard)
	if err != nil {
		return nil, err
	}
	*e.list = list
	return list, nil
}

func (e *entryEditor) save(c *cheats.Cheat, entry cheats.Entry) (tui.SaveResult, error) {
	if !e.editable(c) {
		return tui.SaveResult{}, errors.New("this command is in a read-only folder")
	}
	if err := cheats.UpdateEntry(c, entry); err != nil {
		return tui.SaveResult{}, err
	}
	list, err := e.reload()
	if err != nil {
		return tui.SaveResult{}, err
	}
	res := tui.SaveResult{Message: "Saved in " + filepath.Base(c.Filename), Cheats: list}
	for _, ch := range list {
		if strings.TrimSpace(ch.Name) == strings.TrimSpace(entry.Title) && sameDir(filepath.Dir(ch.Filename), filepath.Dir(c.Filename)) &&
			filepath.Base(ch.Filename) == filepath.Base(c.Filename) {
			res.Saved = ch
		}
	}
	return res, nil
}

func (e *entryEditor) delete(c *cheats.Cheat) (tui.SaveResult, error) {
	if !e.editable(c) {
		return tui.SaveResult{}, errors.New("this command is in a read-only folder")
	}
	if err := cheats.DeleteEntry(c); err != nil {
		return tui.SaveResult{}, err
	}
	list, err := e.reload()
	if err != nil {
		return tui.SaveResult{}, err
	}
	return tui.SaveResult{Message: "Deleted " + strings.TrimSpace(c.Name) + " from " + filepath.Base(c.Filename), Cheats: list}, nil
}
