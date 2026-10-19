package browser

import (
	"archive/zip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func archiveFixture(t *testing.T) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "sample.ZIP")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, name := range []string{"folder/sub/file.txt", "empty/", "a10.txt", "a2.txt", ".hidden", "../escape", "/absolute", `bad\entry`, "drive:name"} {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)}
		entry, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if name[len(name)-1] != '/' {
			if _, err := entry.Write([]byte("data")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestArchiveDirectoryProjection(t *testing.T) {
	filename := archiveFixture(t)
	for _, tc := range []struct {
		folder string
		names  []string
	}{
		{".", []string{"..", "empty", "folder", ".hidden", "a2.txt", "a10.txt"}},
		{"folder", []string{"..", "sub"}},
		{"folder/sub", []string{"..", "file.txt"}},
		{"empty", []string{".."}},
	} {
		t.Run(tc.folder, func(t *testing.T) {
			d, err := ReadArchive(Location{Path: filename, Folder: tc.folder})
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, len(d.Entries))
			for i, entry := range d.Entries {
				names[i] = entry.Name
				if entry.Name == "a2.txt" && (!entry.SizeKnown || entry.Bytes != 4 || entry.Modified.IsZero() || !entry.Created.IsZero()) {
					t.Fatal("archive metadata is missing or fabricated", entry)
				}
			}
			if !slices.Equal(names, tc.names) || d.Path != filename || d.ArchiveFolder != tc.folder || !d.Entries[0].Parent {
				t.Fatalf("listing = %+v, want %v", d, tc.names)
			}
		})
	}
	if _, err := ReadArchive(Location{Path: filename, Folder: "missing"}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing archive folder was not reported", err)
	}
	for _, folder := range []string{"", "..", "../folder", "/folder", "folder//sub"} {
		if _, err := ReadArchive(Location{Path: filename, Folder: folder}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("invalid folder %q was accepted: %v", folder, err)
		}
	}
}

func TestArchiveLocationsAndCapabilities(t *testing.T) {
	filename := archiveFixture(t)
	root := Location{Path: filename, Folder: "."}
	child := root.Child("folder").Child("sub")
	parent, selected := child.Parent()
	if parent != root.Child("folder") || selected != "sub" || child.String() != filepath.Join(filename, "folder", "sub") {
		t.Fatal("archive child/parent paths lost their boundary")
	}
	parent, selected = root.Parent()
	if parent.InArchive() || parent.Path != filepath.Dir(filename) || selected != filepath.Base(filename) {
		t.Fatal("archive root did not exit to its containing folder")
	}
	if root.Capabilities() != (Capabilities{Browse: true, Read: true, Add: true, Delete: true, Rename: true}) || !parent.Capabilities().NativePaths {
		t.Fatal("archive capabilities advertised unsupported operations")
	}
	resolved, ok := ResolveArchiveLocation(child.String())
	if !ok || resolved != child {
		t.Fatal("displayed archive path did not resolve", resolved)
	}
	realDirectory := filepath.Join(t.TempDir(), "folder.zip")
	if err := os.Mkdir(realDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if _, ok := ResolveArchiveLocation(realDirectory); ok {
		t.Fatal("physical directory named .zip became an archive")
	}
	if ArchiveFormat("file.7z") != nil || ArchiveFormat("file.tar.gz") != nil {
		t.Fatal("unimplemented formats advertised support")
	}
}

func TestInvalidArchiveRetainsReadError(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "broken.zip")
	if err := os.WriteFile(filename, []byte("not a zip"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadArchive(Location{Path: filename, Folder: "."}); !errors.Is(err, ErrInvalidArchive) {
		t.Fatal("corrupt zip did not report an archive error", err)
	}
}
