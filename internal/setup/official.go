// Adds the cheatsheets of the original Arsenal, shipped with ars, to the user's collection.

package setup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ynn3m/arsenal/internal/cheats"
)

func AddOfficial(dir string, pack fs.FS) (cheats.AddResult, error) {
	tmp, err := os.MkdirTemp("", "arsenal-pack-")
	if err != nil {
		return cheats.AddResult{}, err
	}
	defer os.RemoveAll(tmp)
	if err := os.CopyFS(tmp, pack); err != nil {
		return cheats.AddResult{}, err
	}
	res := cheats.Load([]string{filepath.Clean(tmp)})
	if len(res.Cheats) == 0 {
		return cheats.AddResult{}, errors.New("the Arsenal cheatsheets have no commands in them")
	}
	return cheats.AddEntries(dir, cheats.Additions(res.Cheats, true))
}
