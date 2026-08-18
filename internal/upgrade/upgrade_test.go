package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceSelfCrossDevice(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sir")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	// force the old code path's temp dir onto a different filesystem (/dev/shm)
	t.Setenv("TMPDIR", "/dev/shm")
	if err := replaceSelf(target, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Fatalf("got %q", got)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0755 {
		t.Fatalf("mode %v", fi.Mode())
	}
}
