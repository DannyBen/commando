package browser

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStageArchiveSelectionSubfoldersAndCleanup(t *testing.T) {
	filename := archiveFixture(t)
	before, _ := os.ReadFile(filename)
	for _, tc := range []struct {
		folder                    string
		names, files, directories []string
	}{
		{".", []string{"folder", "empty", "a2.txt"}, []string{"folder/sub/file.txt", "a2.txt"}, []string{"empty"}},
		{"folder", []string{"sub"}, []string{"sub/file.txt"}, nil},
		{"folder/sub", []string{"file.txt"}, []string{"file.txt"}, nil},
	} {
		stage, err := StageArchive(Location{Path: filename, Folder: tc.folder}, tc.names)
		if err != nil {
			t.Fatal(err)
		}
		for i, name := range tc.names {
			if stage.Paths[i] != filepath.Join(stage.Directory, name) {
				t.Fatal("staged paths lost selected names")
			}
		}
		for _, name := range tc.files {
			data, err := os.ReadFile(filepath.Join(stage.Directory, filepath.FromSlash(name)))
			if err != nil || string(data) != "data" {
				t.Fatal("missing extracted contents", name, err)
			}
			info, err := os.Stat(filepath.Join(stage.Directory, filepath.FromSlash(name)))
			if err != nil || !info.ModTime().Equal(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)) {
				t.Fatal("member timestamp changed", err)
			}
		}
		for _, name := range tc.directories {
			info, err := os.Stat(filepath.Join(stage.Directory, name))
			if err != nil || !info.IsDir() {
				t.Fatal("empty directory lost", err)
			}
		}
		if _, err := os.Stat(filepath.Join(stage.Directory, "a10.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("unselected member extracted")
		}
		if err := stage.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(stage.Directory); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("staging was not cleaned up", err)
		}
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) {
		t.Fatal("copy changed archive")
	}
}

func TestStageArchiveRejectsInvalidAndMissingSelections(t *testing.T) {
	filename := archiveFixture(t)
	for _, names := range [][]string{nil, {".."}, {"/tmp"}, {"folder/sub"}, {`folder\sub`}, {"NUL"}, {"a2.txt", "A2.txt"}, {"missing"}} {
		stage, err := StageArchive(Location{Path: filename, Folder: "."}, names)
		if stage != nil {
			stage.Close()
		}
		if err == nil {
			t.Fatalf("accepted invalid selection %q", names)
		}
	}
	for _, folder := range []string{"", "..", "../folder", "/folder"} {
		if stage, err := StageArchive(Location{Path: filename, Folder: folder}, []string{"a2.txt"}); err == nil {
			stage.Close()
			t.Fatalf("accepted invalid folder %q", folder)
		}
	}
}

func TestStageArchiveRejectsUnsafeAndAmbiguousMembers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []*zip.FileHeader
	}{
		{"parent traversal", []*zip.FileHeader{{Name: "folder/../escape"}}},
		{"absolute child", []*zip.FileHeader{{Name: "folder//absolute"}}},
		{"Windows separator", []*zip.FileHeader{{Name: `folder/..\escape`}}},
		{"alternate stream", []*zip.FileHeader{{Name: "folder/file:stream"}}},
		{"reserved filename", []*zip.FileHeader{{Name: "folder/NUL.txt"}}},
		{"trailing dot", []*zip.FileHeader{{Name: "folder/file."}}},
		{"case collision", []*zip.FileHeader{{Name: "folder/A.txt"}, {Name: "folder/a.txt"}}},
		{"implied directory collision", []*zip.FileHeader{{Name: "folder/Sub/a.txt"}, {Name: "folder/sub/b.txt"}}},
		{"duplicate", []*zip.FileHeader{{Name: "folder/a.txt"}, {Name: "folder/a.txt"}}},
		{"file directory conflict", []*zip.FileHeader{{Name: "folder/a"}, {Name: "folder/a/b"}}},
		{"symlink", []*zip.FileHeader{{Name: "folder/link"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "symlink" {
				tc.headers[0].SetMode(os.ModeSymlink | 0777)
			}
			var buffer bytes.Buffer
			w := zip.NewWriter(&buffer)
			for _, header := range tc.headers {
				f, err := w.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.Write([]byte("payload")); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(t.TempDir(), "sample.zip")
			if err := os.WriteFile(filename, buffer.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			// A private temp parent lets us verify failure removes partially staged data.
			t.Setenv("TMPDIR", t.TempDir())
			t.Setenv("TMP", os.Getenv("TMPDIR"))
			t.Setenv("TEMP", os.Getenv("TMPDIR"))
			stage, err := StageArchive(Location{Path: filename, Folder: "."}, []string{"folder"})
			if stage != nil {
				stage.Close()
			}
			if err == nil {
				t.Fatal("unsafe archive reached transfer staging")
			}
			files, err := os.ReadDir(filepath.Join(os.TempDir(), "Commando", "session"))
			if err != nil || len(files) != 0 {
				t.Fatal("failed extraction left staging files", files, err)
			}
		})
	}
}

func TestStageArchiveVerifiesChecksum(t *testing.T) {
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	f, _ := w.CreateHeader(&zip.FileHeader{Name: "file.txt", Method: zip.Store})
	f.Write([]byte("unique-payload"))
	w.Close()
	data := bytes.Replace(buffer.Bytes(), []byte("unique-payload"), []byte("broken-payload"), 1)
	filename := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	stage, err := StageArchive(Location{Path: filename, Folder: "."}, []string{"file.txt"})
	if stage != nil {
		stage.Close()
	}
	if !errors.Is(err, zip.ErrChecksum) {
		t.Fatal("corrupt ZIP data accepted", err)
	}
}
