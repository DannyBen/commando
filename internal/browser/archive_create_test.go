package browser

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"reflect"
	"testing"
)

func TestArchiveCreatePreservesMembersAndCreatesNestedEntries(t *testing.T) {
	for _, folder := range []bool{false, true} {
		for _, base := range []string{".", "folder/sub", "empty"} {
			filename := archiveFixture(t)
			comment, before, headers := zipMembers(t, filename)
			location := Location{Path: filename, Folder: base}
			name := `new\sub\新しい.txt`
			data := []byte("new content")
			if folder {
				data = nil
			}
			parent, created, err := CreateArchive(location, name, folder, data)
			if err != nil {
				t.Fatal(err)
			}
			if parent.Folder != path.Join(base, "new/sub") || created.Name != "新しい.txt" || created.Directory != folder {
				t.Fatal("wrong created location/entry", parent, created)
			}
			nextComment, after, nextHeaders := zipMembers(t, filename)
			if nextComment != comment {
				t.Fatal("archive comment changed")
			}
			for name, data := range before {
				if !bytes.Equal(data, after[name]) || !reflect.DeepEqual(headers[name], nextHeaders[name]) {
					t.Fatal("existing compressed bytes or metadata changed", name)
				}
			}
			reader, err := zip.OpenReader(filename)
			if err != nil {
				t.Fatal(err)
			}
			target := path.Join(base, "new/sub/新しい.txt")
			if folder {
				target += "/"
			}
			found := false
			for _, member := range reader.File {
				if member.Name != target {
					continue
				}
				found = true
				stream, err := member.Open()
				if err != nil {
					t.Fatal(err)
				}
				contents, err := io.ReadAll(stream)
				stream.Close()
				if err != nil || !bytes.Equal(contents, data) || member.FileInfo().IsDir() != folder {
					t.Fatal("wrong new member", member.Name, err)
				}
			}
			reader.Close()
			if !found {
				t.Fatal("new member missing", target)
			}
			directory, err := ReadArchive(parent)
			if err != nil || len(directory.Entries) != 2 || directory.Entries[1].Name != created.Name {
				t.Fatal("implied parents not browsable", directory, err)
			}
			assertNoZIPTemporary(t, filename)
		}
	}
}

func TestArchiveCreateRejectsCollisionsAndInvalidPathsAtomically(t *testing.T) {
	for _, test := range []struct {
		base, name string
		want       error
	}{
		{".", "a2.txt", fs.ErrExist}, {".", "folder", fs.ErrExist}, {".", "empty", fs.ErrExist},
		{".", "a2.txt/child.txt", fs.ErrExist}, {".", "folder/sub/file.txt/child.txt", fs.ErrExist},
		{"missing", "new.txt", fs.ErrNotExist}, {".", "fresh/../bad", ErrInvalidName},
		{".", "fresh/CON.txt", ErrInvalidName}, {".", "/absolute", ErrInvalidName},
	} {
		for _, folder := range []bool{false, true} {
			filename := archiveFixture(t)
			before, _ := os.ReadFile(filename)
			_, entry, err := CreateArchive(Location{Path: filename, Folder: test.base}, test.name, folder, nil)
			if !errors.Is(err, test.want) || entry.Name != "" {
				t.Fatal("wrong refusal", test, folder, entry, err)
			}
			after, _ := os.ReadFile(filename)
			if !bytes.Equal(before, after) {
				t.Fatal("failed creation changed ZIP", test)
			}
			assertNoZIPTemporary(t, filename)
		}
	}
}

func TestArchiveCreateEmptyFileAndCaseSensitiveNames(t *testing.T) {
	filename := archiveFixture(t)
	_, created, err := CreateArchive(Location{Path: filename, Folder: "."}, "A2.txt", false, nil)
	if err != nil || !created.SizeKnown || created.Bytes != 0 {
		t.Fatal("empty file creation failed", created, err)
	}
	_, contents, _ := zipMembers(t, filename)
	if _, ok := contents["A2.txt"]; !ok {
		t.Fatal("case-distinct entry missing")
	}
	if _, ok := contents["a2.txt"]; !ok {
		t.Fatal("case-distinct original was lost")
	}
}
