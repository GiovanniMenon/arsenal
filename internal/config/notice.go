// Warns when XDG_CONFIG_HOME hides a config that lives in ~/.config/arsenal.

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func OtherConfigNotice() string {
	x := os.Getenv("XDG_CONFIG_HOME")
	home, err := os.UserHomeDir()
	if x == "" || err != nil {
		return ""
	}
	def := filepath.Join(home, ".config", appDirName)
	cur := filepath.Join(x, appDirName)
	if filepath.Clean(def) == filepath.Clean(cur) {
		return ""
	}
	for _, name := range []string{"config.yml", "vars"} {
		if _, err := os.Stat(filepath.Join(def, name)); err == nil {
			return fmt.Sprintf("Found a config in %s, but XDG_CONFIG_HOME points to %s: ars uses %s. Move the files yourself or unset XDG_CONFIG_HOME.", def, x, cur)
		}
	}
	return ""
}

func FileExists() (bool, error) {
	p, err := FilePath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
