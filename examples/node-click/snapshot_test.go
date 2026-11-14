package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// snapshot writes what tt shows to $EXAMPLES_SNAPSHOTS/<test>.png when that
// variable is set, to look at the example without opening a window.
func snapshot(t *testing.T, tt *ui.Tester) {
	dir := os.Getenv("EXAMPLES_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, filepath.Base(t.Name())+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
