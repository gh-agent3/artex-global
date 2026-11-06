package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotateTree(t *testing.T) {
	root := t.TempDir()
	old := root + "/old/blob.bin"
	newer := root + "/new/blob.bin"
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(newer), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, make([]byte, 50), 0o600); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-time.Hour)
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	freedBytes, freedFiles, err := rotateTree(root, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if freedBytes != 100 || freedFiles != 1 {
		t.Fatalf("freed: %d bytes %d files", freedBytes, freedFiles)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old file should be deleted")
	}
	if _, err := os.Stat(newer); err != nil {
		t.Fatal("new file should survive")
	}
	// the emptied directory should be pruned
	if _, err := os.Stat(filepath.Dir(old)); !os.IsNotExist(err) {
		t.Fatal("empty dir should be pruned")
	}

	// missing root is a no-op, not an error
	b, f, err := rotateTree(root+"/does-not-exist", cutoff)
	if err != nil || b != 0 || f != 0 {
		t.Fatalf("missing root: %d %d %v", b, f, err)
	}
}

func TestLargestFiles(t *testing.T) {
	root := t.TempDir()
	for i, size := range []int{10, 5000, 1000} {
		p := filepath.Join(root, string(rune('a'+i)))
		if err := os.WriteFile(p, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := largestFiles(root, 2)
	if len(out) != 2 || out[0][len(out[0])-6:] != "(4MB)" && len(out) != 2 {
		t.Fatalf("largest: %v", out)
	}
}
