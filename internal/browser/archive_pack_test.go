package browser

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPackZIPStreamsSelectedFilesAndFolders(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "docs", "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.txt", "docs/nested.txt", "unselected.txt"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := t.TempDir()
	entry, err := PackZIP(target, "bundle.zip", []string{filepath.Join(source, "report.txt"), filepath.Join(source, "docs")})
	if err != nil || entry.Name != "bundle.zip" || !entry.SizeKnown || entry.Bytes == 0 {
		t.Fatal("pack failed", entry, err)
	}
	r, err := zip.OpenReader(filepath.Join(target, "bundle.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	want := map[string]string{"report.txt": "report.txt", "docs/": "", "docs/empty/": "", "docs/nested.txt": "docs/nested.txt"}
	for _, file := range r.File {
		contents, ok := want[file.Name]
		if !ok {
			t.Fatal("unexpected archive member", file.Name)
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || string(data) != contents || !file.FileInfo().IsDir() && file.Method != zip.Deflate {
			t.Fatal("wrong member contents or compression", file.Name, err)
		}
		delete(want, file.Name)
	}
	if len(want) != 0 {
		t.Fatal("missing selected archive members", want)
	}
	for _, name := range []string{"report.txt", "docs/nested.txt", "unselected.txt"} {
		if data, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(name))); err != nil || string(data) != name {
			t.Fatal("packing changed source", name, err)
		}
	}
}

func TestPackZIPRejectsCollisionsInvalidSourcesAndSelfInclusion(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "source.txt")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		paths []string
		want  error
	}{
		{"source.txt", []string{file}, fs.ErrExist},
		{"new.zip", nil, fs.ErrInvalid},
		{"../escape.zip", []string{file}, ErrInvalidName},
		{"missing.zip", []string{filepath.Join(root, "missing")}, fs.ErrNotExist},
		{"duplicate.zip", []string{file, file}, fs.ErrExist},
		{"self.zip", []string{root}, ErrArchiveSelfCopy},
	} {
		_, err := PackZIP(root, tc.name, tc.paths)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed pack left output", entries, err)
	}
	if data, _ := os.ReadFile(file); string(data) != "original" {
		t.Fatal("collision overwrote destination")
	}
}
