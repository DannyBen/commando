package browser

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
)

func TestArchiveDuplicatePreservesOriginalAndCompressedMembers(t *testing.T) {
	for _, test := range []struct{ folder, source, destination string }{
		{".", "a2.txt", "新しい.txt"}, {"folder/sub", "file.txt", "copy.txt"},
		{".", "folder", "documents"}, {".", "empty", "blank"}, {".", "a2.txt", "A2.txt"},
	} {
		filename := archiveFixture(t)
		comment, before, headers := zipMembers(t, filename)
		plan, err := PlanArchiveDuplicate(Location{Path: filename, Folder: test.folder}, []string{test.source})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := plan.Request(test.destination, 0); !errors.Is(err, fs.ErrInvalid) {
			t.Fatal("archive plan accepted native dispatch", err)
		}
		entry, err := DuplicateArchive(plan, test.destination)
		if err != nil || entry.Name != test.destination {
			t.Fatal("duplicate failed", entry, err)
		}
		nextComment, after, nextHeaders := zipMembers(t, filename)
		if nextComment != comment {
			t.Fatal("archive comment changed")
		}
		oldPath, newPath := path.Join(test.folder, test.source), path.Join(test.folder, test.destination)
		copies := 0
		for name, data := range before {
			if !bytes.Equal(data, after[name]) || !reflect.DeepEqual(headers[name], nextHeaders[name]) {
				t.Fatal("original member changed", name)
			}
			if name != oldPath && !strings.HasPrefix(name, oldPath+"/") {
				continue
			}
			copied := newPath + strings.TrimPrefix(name, oldPath)
			if !bytes.Equal(data, after[copied]) {
				t.Fatal("copy recompressed or lost member", copied)
			}
			expected := headers[name]
			expected.Name = copied
			expected.Flags |= 0x800
			expected.NonUTF8 = false
			if !reflect.DeepEqual(expected, nextHeaders[copied]) {
				t.Fatal("copy lost metadata", copied, expected, nextHeaders[copied])
			}
			copies++
		}
		if copies == 0 || len(after) != len(before)+copies {
			t.Fatal("wrong copied subtree", len(after), copies)
		}
		assertNoZIPTemporary(t, filename)
	}
}

func TestArchiveDuplicateBatchNamingAndAtomicRefusals(t *testing.T) {
	filename := archiveFixture(t)
	location := Location{Path: filename, Folder: "."}
	if _, _, err := CreateArchive(location, "a2 - Copy.txt", false, nil); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanArchiveDuplicate(location, []string{"a2.txt", "folder", "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Names, []string{"a2 - Copy (2).txt", "folder - Copy", "empty - Copy"}) {
		t.Fatal("wrong available suggestions", plan.Names)
	}
	before, _ := os.ReadFile(filename)
	plan.Names[1] = "empty" // One collision must reject the entire batch.
	if _, err := DuplicateArchive(plan, ""); !errors.Is(err, fs.ErrExist) {
		t.Fatal("collision accepted", err)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) {
		t.Fatal("failed batch changed ZIP")
	}
	plan.Names[1] = "folder - Copy"
	if _, err := DuplicateArchive(plan, ""); err != nil {
		t.Fatal(err)
	}
	_, members, _ := zipMembers(t, filename)
	for _, name := range []string{"a2 - Copy (2).txt", "folder - Copy/sub/file.txt", "empty - Copy/"} {
		if _, ok := members[name]; !ok {
			t.Fatal("batch member missing", name)
		}
	}
}

func TestArchiveDuplicateRetryInvalidNamesAndChangedSources(t *testing.T) {
	filename := archiveFixture(t)
	location := Location{Path: filename, Folder: "."}
	plan, err := PlanArchiveDuplicate(location, []string{"a2.txt"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filename)
	for _, name := range []string{"a10.txt", "folder", "../escape", "NUL"} {
		if _, err := DuplicateArchive(plan, name); err == nil {
			t.Fatal("invalid destination accepted", name)
		}
		after, _ := os.ReadFile(filename)
		if !bytes.Equal(before, after) {
			t.Fatal("refused duplicate changed ZIP")
		}
	}
	if _, err := DuplicateArchive(plan, "retry.txt"); err != nil {
		t.Fatal("valid retry failed", err)
	}
	if _, err := DuplicateArchive(plan, "second.txt"); !errors.Is(err, ErrArchiveClipboardChanged) {
		t.Fatal("stale duplicate plan accepted", err)
	}
	if _, err := PlanArchiveDuplicate(location, []string{"missing"}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing source accepted", err)
	}
}
