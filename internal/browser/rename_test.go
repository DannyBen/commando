package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateRenameName(t *testing.T) {
	for _, name := range []string{"notes.txt", "New folder", ".gitignore", ".config.json", "Ångström.md", "com10.txt", "auxiliary"} {
		if err := ValidateName(name); err != nil {
			t.Errorf("valid name %q: %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "../file", `child\file`, `C:thing`, "x/y", "x?", "x*", `x"`, "x|", "x<", "x>", "x\x00", "x\n", "name.", "name ", "CON", "con.txt", "NUL.tar.gz", "PRN", "AUX", "COM1.log", "LPT9", "COM¹", "LPT².txt", "CONIN$", "CONOUT$"} {
		if !errors.Is(ValidateName(name), ErrInvalidName) {
			t.Errorf("accepted invalid name %q", name)
		}
	}
}

func TestRenameWithoutReplacement(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"source.txt", "existing.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Rename(root, "source.txt", "existing.txt"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("collision: %v", err)
	}
	for _, name := range []string{"source.txt", "existing.txt"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != name {
			t.Fatalf("collision changed %q: %q %v", name, data, err)
		}
	}
	if err := Rename(root, "source.txt", "../outside.txt"); !errors.Is(err, ErrInvalidName) {
		t.Fatal(err)
	}
	if err := Rename(root, "source.txt", "SOURCE.txt"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "SOURCE.txt")); err != nil || string(data) != "source.txt" {
		t.Fatalf("case rename: %q %v", data, err)
	}
	if err := Rename(root, "SOURCE.txt", "SOURCE.txt"); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, "missing.txt", "new.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing source: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "folder", "child.txt"), []byte("child"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, "folder", "renamed folder"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed folder", "child.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestRenameDoesNotReplaceBrokenLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks can require Windows privileges")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "existing")); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, "source", "existing"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("symlink collision: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(root, "existing")); err != nil || target != "missing" {
		t.Fatalf("symlink replaced: %q %v", target, err)
	}
}
