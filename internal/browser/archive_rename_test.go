package browser

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArchiveRenamePreservesCompressedContentsAndMetadata(t *testing.T) {
	for _, tc := range []struct{ folder, oldName, newName, oldMember, newMember string }{
		{".", "a2.txt", "résumé.md", "a2.txt", "résumé.md"},
		{"folder/sub", "file.txt", "report.txt", "folder/sub/file.txt", "folder/sub/report.txt"},
		{".", "folder", "documents", "folder/sub/file.txt", "documents/sub/file.txt"},
		{".", "empty", "blank", "empty/", "blank/"},
		{".", "a2.txt", "A2.txt", "a2.txt", "A2.txt"},
	} {
		t.Run(tc.folder+"/"+tc.newName, func(t *testing.T) {
			filename := archiveFixture(t)
			comment, contents, headers := zipMembers(t, filename)
			if err := RenameArchive(Location{Path: filename, Folder: tc.folder}, tc.oldName, tc.newName); err != nil {
				t.Fatal(err)
			}
			afterComment, after, afterHeaders := zipMembers(t, filename)
			if afterComment != comment || len(after) != len(contents) {
				t.Fatal("rename changed archive comment or member count")
			}
			for name, data := range contents {
				next := name
				if name == tc.oldMember {
					next = tc.newMember
				}
				if !bytes.Equal(data, after[next]) {
					t.Fatal("compressed data changed", name, next)
				}
				before, changed := headers[name], afterHeaders[next]
				if name == tc.oldMember {
					before.Name = next
					before.Flags |= 0x800
					before.NonUTF8 = false
				}
				if !reflect.DeepEqual(before, changed) {
					t.Fatal("rename lost member metadata", name, before, changed)
				}
			}
			if _, exists := after[tc.oldMember]; exists {
				t.Fatal("old member retained")
			}
			assertNoZIPTemporary(t, filename)
		})
	}
}

func TestArchiveRenameConflictsAndInvalidNamesKeepOriginal(t *testing.T) {
	for _, tc := range []struct {
		oldName, newName string
		want             error
	}{
		{"a2.txt", "a10.txt", fs.ErrExist}, {"a2.txt", "folder", fs.ErrExist},
		{"folder", "empty", fs.ErrExist}, {"folder", "a2.txt", fs.ErrExist},
		{"missing", "new", fs.ErrNotExist}, {"..", "new", ErrInvalidName},
		{"a2.txt", "../new", ErrInvalidName}, {"a2.txt", "NUL", ErrInvalidName},
	} {
		filename := archiveFixture(t)
		before, _ := os.ReadFile(filename)
		if err := RenameArchive(Location{Path: filename, Folder: "."}, tc.oldName, tc.newName); !errors.Is(err, tc.want) {
			t.Fatal("wrong rename error", tc, err)
		}
		after, _ := os.ReadFile(filename)
		if !bytes.Equal(before, after) {
			t.Fatal("failed rename changed ZIP")
		}
		assertNoZIPTemporary(t, filename)
	}
}

func TestArchiveRenameDropsStaleUnicodePathExtra(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "unicode.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	oldName := "old.txt"
	extra := make([]byte, 9)
	binary.LittleEndian.PutUint16(extra, 0x7075)
	binary.LittleEndian.PutUint16(extra[2:], uint16(5+len(oldName)))
	extra[4] = 1
	binary.LittleEndian.PutUint32(extra[5:], crc32.ChecksumIEEE([]byte(oldName)))
	extra = append(extra, oldName...)
	extra = append(extra, 0xfe, 0xca, 1, 0, 42) // Unrelated metadata must survive.
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: oldName, Extra: extra, Method: zip.Deflate, Comment: "member comment"})
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("contents"))
	writer.Close()
	file.Close()
	if err := RenameArchive(Location{Path: filename, Folder: "."}, oldName, "新しい.txt"); err != nil {
		t.Fatal(err)
	}
	_, _, headers := zipMembers(t, filename)
	header := headers["新しい.txt"]
	if header.Flags&0x800 == 0 || header.NonUTF8 || header.Comment != "member comment" || !bytes.Equal(header.Extra, []byte{0xfe, 0xca, 1, 0, 42}) {
		t.Fatal("renamed Unicode filename has stale encoding metadata", header)
	}
}

func TestArchiveRenameOnlyChangesExactSubtreeAndNoOpKeepsOriginal(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "folders.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{"docs/", "docs/nested/file", "docs2/file", "other/docs/file"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if name[len(name)-1] != '/' {
			entry.Write([]byte(name))
		}
	}
	writer.Close()
	file.Close()
	before, _ := os.ReadFile(filename)
	if err := RenameArchive(Location{Path: filename, Folder: "."}, "docs", "docs"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) {
		t.Fatal("no-op rename rewrote ZIP")
	}
	if err := RenameArchive(Location{Path: filename, Folder: "."}, "docs", "renamed"); err != nil {
		t.Fatal(err)
	}
	_, members, _ := zipMembers(t, filename)
	for _, name := range []string{"renamed/", "renamed/nested/file", "docs2/file", "other/docs/file"} {
		if _, found := members[name]; !found {
			t.Fatal("rename crossed a subtree boundary", name, members)
		}
	}
}

func TestArchiveRenamePreservesLegacyCommentsWithoutMixingEncodings(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "legacy.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: "old.txt", Comment: "legacy\xff", NonUTF8: true})
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("data"))
	writer.Close()
	file.Close()
	original, _ := os.ReadFile(filename)
	if err := RenameArchive(Location{Path: filename, Folder: "."}, "old.txt", "新.txt"); !errors.Is(err, ErrArchiveEncoding) {
		t.Fatal("mixed filename/comment encodings accepted", err)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(original, after) {
		t.Fatal("failed encoding conversion changed ZIP")
	}
	if err := RenameArchive(Location{Path: filename, Folder: "."}, "old.txt", "new.txt"); err != nil {
		t.Fatal(err)
	}
	_, _, headers := zipMembers(t, filename)
	if headers["new.txt"].Comment != "legacy\xff" || !headers["new.txt"].NonUTF8 {
		t.Fatal("ASCII rename changed legacy comment encoding")
	}
}
