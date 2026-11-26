package arsenal

import (
	"testing"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/setup"
)

func TestBundledCheatsAllLoad(t *testing.T) {
	dir := t.TempDir()
	res, err := setup.AddOfficial(dir, Cheats())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added < 1000 {
		t.Errorf("%+v", res)
	}
	loaded := cheats.Load([]string{dir})
	if len(loaded.Warnings) > 0 || len(loaded.Cheats) != res.Added {
		t.Errorf("%d cheats, %v", len(loaded.Cheats), loaded.Warnings)
	}
}
