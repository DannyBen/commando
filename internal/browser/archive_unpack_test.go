package browser

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func unpackFixture(t *testing.T, names ...string) string {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(name, "/") {
			if _, err := member.Write([]byte("data")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "sample.zip")
	if err := os.WriteFile(filename, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestUnpackZIPPreservesStructureAndSource(t *testing.T) {
	filename := unpackFixture(t, "a2.txt", "a10.txt", "folder/sub/file.txt", "empty/")
	before, _ := os.ReadFile(filename)
	for _, folder := range []string{"", "extracted.zip"} {
		target := t.TempDir()
		created, err := UnpackArchive(filename, target, folder)
		if err != nil || created.Name != folder || folder != "" && !created.Directory {
			t.Fatal("unpack failed", created, err)
		}
		for _, name := range []string{"a2.txt", "a10.txt", "folder/sub/file.txt"} {
			data, err := os.ReadFile(filepath.Join(target, folder, filepath.FromSlash(name)))
			if err != nil || string(data) != "data" {
				t.Fatal("archive structure or data changed", name, err)
			}
		}
		if info, err := os.Stat(filepath.Join(target, folder, "empty")); err != nil || !info.IsDir() {
			t.Fatal("empty directory lost", err)
		}
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) {
		t.Fatal("unpacking modified the source ZIP")
	}
}

func TestUnpackZIPRefusesExistingRootsWithoutWriting(t *testing.T) {
	for _, name := range []string{"folder", "FOLDER", "a2.txt", "A2.TXT"} {
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			if err := os.WriteFile(filepath.Join(target, name), []byte("existing"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := UnpackArchive(unpackFixture(t, "a2.txt", "a10.txt", "folder/sub/file.txt", "empty/"), target, ""); !errors.Is(err, fs.ErrExist) {
				t.Fatal("collision accepted", err)
			}
			entries, _ := os.ReadDir(target)
			if len(entries) != 1 {
				t.Fatal("preflight wrote output before refusing collision", entries)
			}
			if data, _ := os.ReadFile(filepath.Join(target, name)); string(data) != "existing" {
				t.Fatal("existing file overwritten")
			}
		})
	}
}

func TestUnpackZIPValidatesEntireArchiveBeforeCreatingFolder(t *testing.T) {
	for _, names := range [][]string{
		{"good.txt", "../escape"}, {"good.txt", "/absolute"}, {"A/file", "a/other"}, {"file", "file/child"}, {"file", "file"},
	} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, name := range names {
			member, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := member.Write([]byte("payload")); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(t.TempDir(), "unsafe.zip")
		if err := os.WriteFile(filename, buffer.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		if _, err := UnpackArchive(filename, target, "new"); err == nil {
			t.Fatal("unsafe ZIP accepted", names)
		}
		entries, _ := os.ReadDir(target)
		if len(entries) != 0 {
			t.Fatal("invalid ZIP created output", entries)
		}
	}
}
