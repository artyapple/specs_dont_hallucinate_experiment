package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRejectsIncompleteRunDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "incomplete"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := load(filepath.Join("..", ".."), dir); err == nil {
		t.Fatal("incomplete run directory must fail")
	}
}
