package browser

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestArchiveAddFilesAndDirectoriesPreservesExistingMembers(t *testing.T) {
	for _, folder := range []string{".", "folder/sub", "empty"} {
		filename := archiveFixture(t)
		comment, before, headers := zipMembers(t, filename)
		source := t.TempDir()
		if err := os.MkdirAll(filepath.Join(source, "added", "empty"), 0700); err != nil {
			t.Fatal(err)
		}
		stamp := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
		for _, name := range []string{"新しい.txt", "added/nested.txt"} {
			target := filepath.Join(source, filepath.FromSlash(name))
			if err := os.WriteFile(target, []byte(name), 0640); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(target, stamp, stamp); err != nil {
				t.Fatal(err)
			}
		}
		if err := AddArchive(Location{Path: filename, Folder: folder}, []string{filepath.Join(source, "新しい.txt"), filepath.Join(source, "added")}); err != nil {
			t.Fatal(err)
		}
		nextComment, after, nextHeaders := zipMembers(t, filename)
		if nextComment != comment {
			t.Fatal("archive comment changed")
		}
		for name, data := range before {
			if !bytes.Equal(data, after[name]) || !reflect.DeepEqual(headers[name], nextHeaders[name]) {
				t.Fatal("existing member changed", name)
			}
		}
		reader, err := zip.OpenReader(filename)
		if err != nil {
			t.Fatal(err)
		}
		prefix := ""
		if folder != "." {
			prefix = folder + "/"
		}
		found := map[string]bool{}
		for _, member := range reader.File {
			for _, name := range []string{"新しい.txt", "added/nested.txt"} {
				if member.Name != prefix+name {
					continue
				}
				stream, err := member.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(stream)
				stream.Close()
				if err != nil || string(data) != name || !member.Modified.Equal(stamp) {
					t.Fatal("new member lost contents/timestamp", member.Name, err)
				}
				found[name] = true
			}
		}
		reader.Close()
		if len(found) != 2 {
			t.Fatal("new members missing", found)
		}
		if _, ok := after[prefix+"added/empty/"]; !ok {
			t.Fatal("empty folder was lost")
		}
		for _, name := range []string{"新しい.txt", "added/nested.txt"} {
			if data, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(name))); err != nil || string(data) != name {
				t.Fatal("source changed", err)
			}
		}
		assertNoZIPTemporary(t, filename)
	}
}

func TestArchiveAddRejectsWholeBatchOnConflictsAndMissingInputs(t *testing.T) {
	for _, conflict := range []string{"file", "implied folder", "explicit folder", "duplicate batch", "missing source", "missing destination"} {
		filename := archiveFixture(t)
		original, _ := os.ReadFile(filename)
		source := t.TempDir()
		fresh := filepath.Join(source, "fresh.txt")
		if err := os.WriteFile(fresh, []byte("fresh"), 0600); err != nil {
			t.Fatal(err)
		}
		location := Location{Path: filename, Folder: "."}
		paths := []string{fresh}
		want := fs.ErrExist
		switch conflict {
		case "file", "implied folder", "explicit folder":
			name := map[string]string{"file": "a2.txt", "implied folder": "folder", "explicit folder": "empty"}[conflict]
			if err := os.WriteFile(filepath.Join(source, name), nil, 0600); err != nil {
				t.Fatal(err)
			}
			paths = append(paths, filepath.Join(source, name))
		case "duplicate batch":
			paths = append(paths, fresh)
		case "missing source":
			paths = append(paths, filepath.Join(source, "missing"))
			want = fs.ErrNotExist
		case "missing destination":
			location.Folder = "gone"
			want = fs.ErrNotExist
		}
		if err := AddArchive(location, paths); !errors.Is(err, want) {
			t.Fatal("wrong copy-in failure", conflict, err)
		}
		after, _ := os.ReadFile(filename)
		if !bytes.Equal(original, after) {
			t.Fatal("failed batch changed ZIP", conflict)
		}
		assertNoZIPTemporary(t, filename)
	}
}

func TestArchiveAddRejectsSelfCopies(t *testing.T) {
	for _, directory := range []bool{false, true} {
		filename := archiveFixture(t)
		before, _ := os.ReadFile(filename)
		source := filename
		if directory {
			source = filepath.Dir(filename)
		}
		if err := AddArchive(Location{Path: filename, Folder: "."}, []string{source}); !errors.Is(err, ErrArchiveSelfCopy) {
			t.Fatal("archive copied into itself", directory, err)
		}
		after, _ := os.ReadFile(filename)
		if !bytes.Equal(before, after) {
			t.Fatal("self copy changed original")
		}
		assertNoZIPTemporary(t, filename)
	}
}

func TestArchiveAddRefusesLinksRatherThanFollowingThem(t *testing.T) {
	filename := archiveFixture(t)
	before, _ := os.ReadFile(filename)
	source, outside := t.TempDir(), filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "link")); err != nil {
		t.Skip("symlinks unavailable", err)
	}
	for _, selected := range []string{source, filepath.Join(source, "link")} {
		if err := AddArchive(Location{Path: filename, Folder: "."}, []string{selected}); !errors.Is(err, errors.ErrUnsupported) {
			t.Fatal("symlink source accepted", err)
		}
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected links changed ZIP")
	}
}

func TestArchiveAddDetectsChangedSourceIdentity(t *testing.T) {
	filename := archiveFixture(t)
	original, _ := os.ReadFile(filename)
	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	err := rewriteZIP(filename, func(reader *zip.Reader, writer *zip.Writer) error {
		plan, err := planArchiveAdd([]string{source}, filename)
		if err != nil {
			return err
		}
		defer plan.close()
		if err := os.Rename(source, source+".old"); err != nil {
			return err
		}
		if err := os.WriteFile(source, []byte("replaced"), 0600); err != nil {
			return err
		}
		stamp := plan.files[0].info.ModTime()
		if err := os.Chtimes(source, stamp, stamp); err != nil {
			return err
		}
		return writeArchiveInput(writer, plan.files[0])
	})
	if !errors.Is(err, ErrArchiveSourceChanged) {
		t.Fatal("replacement source accepted despite changed identity", err)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(original, after) {
		t.Fatal("changed source committed a partial ZIP")
	}
	assertNoZIPTemporary(t, filename)
}
