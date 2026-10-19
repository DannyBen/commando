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
	"slices"
	"testing"
	"time"
)

func zipMembers(t *testing.T, filename string) (string, map[string][]byte, map[string]zip.FileHeader) {
	t.Helper()
	r, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	contents, headers := map[string][]byte{}, map[string]zip.FileHeader{}
	for _, file := range r.File {
		raw, err := file.OpenRaw()
		if err != nil {
			t.Fatal(err)
		}
		contents[file.Name], err = io.ReadAll(raw)
		if err != nil {
			t.Fatal(err)
		}
		headers[file.Name] = file.FileHeader
	}
	return r.Comment, contents, headers
}

func TestDeleteArchivePreservesOtherCompressedMembersAndMetadata(t *testing.T) {
	filename := archiveFixture(t)
	beforeComment, before, headers := zipMembers(t, filename)
	result, err := DeleteArchive(Location{Path: filename, Folder: "."}, []string{"folder", "a2.txt"})
	if err != nil || !slices.Equal(result.Names, []string{"folder", "a2.txt"}) {
		t.Fatal("deletion failed", result, err)
	}
	comment, after, nextHeaders := zipMembers(t, filename)
	if comment != beforeComment {
		t.Fatal("archive comment changed")
	}
	delete(before, "folder/sub/file.txt")
	delete(before, "a2.txt")
	delete(headers, "folder/sub/file.txt")
	delete(headers, "a2.txt")
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(headers, nextHeaders) {
		t.Fatal("untouched compressed bytes or metadata changed")
	}
}

func TestDeleteArchiveSubfolderCaseSensitivityAndEmptyResult(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "case.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	writer.SetComment("keep archive comment")
	for _, name := range []string{"docs/A.txt", "docs/a.txt", "empty/"} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Comment: "member comment", Modified: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}
		header.SetMode(0640)
		w, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if name != "empty/" {
			if _, err := w.Write([]byte("contents")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := DeleteArchive(Location{Path: filename, Folder: "docs"}, []string{"A.txt"}); err != nil {
		t.Fatal(err)
	}
	comment, contents, headers := zipMembers(t, filename)
	if len(contents) != 2 || contents["docs/a.txt"] == nil || headers["docs/a.txt"].Comment != "member comment" || comment != "keep archive comment" {
		t.Fatal("case-distinct sibling or metadata lost", contents)
	}
	if _, err := DeleteArchive(Location{Path: filename, Folder: "."}, []string{"docs", "empty"}); err != nil {
		t.Fatal(err)
	}
	comment, contents, _ = zipMembers(t, filename)
	if len(contents) != 0 || comment != "keep archive comment" {
		t.Fatal("deleting all entries did not leave valid empty ZIP")
	}
	if _, err := ReadArchive(Location{Path: filename, Folder: "."}); err != nil {
		t.Fatal("empty ZIP cannot be browsed", err)
	}
}

func TestDeleteArchivePreflightsAllNames(t *testing.T) {
	for _, names := range [][]string{nil, {"folder", "missing"}, {".."}, {"folder/sub"}, {"folder", "folder"}} {
		filename := archiveFixture(t)
		before, _ := os.ReadFile(filename)
		result, err := DeleteArchive(Location{Path: filename, Folder: "."}, names)
		if err == nil || len(result.Names) != 0 {
			t.Fatal("invalid selection was committed", names, result, err)
		}
		after, _ := os.ReadFile(filename)
		if !bytes.Equal(before, after) {
			t.Fatal("failed selection changed original")
		}
		assertNoZIPTemporary(t, filename)
	}
}

func assertNoZIPTemporary(t *testing.T, filename string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".commando-zip-*"))
	if err != nil || len(matches) != 0 {
		t.Fatal("rewrite leaked temporary ZIP", matches, err)
	}
}

func TestZIPRewriteKeepsOriginalOnWriteOrReplacementFailure(t *testing.T) {
	for _, failure := range []string{"write", "replace"} {
		t.Run(failure, func(t *testing.T) {
			filename := archiveFixture(t)
			original, _ := os.ReadFile(filename)
			replaced := false
			err := rewriteZIPWith(filename, func(r *zip.Reader, w *zip.Writer) error {
				if failure == "write" {
					member, err := w.Create("partial.txt")
					if err != nil {
						return err
					}
					member.Write([]byte("partial"))
					return fs.ErrPermission
				}
				for _, f := range r.File {
					if err := w.Copy(f); err != nil {
						return err
					}
				}
				return nil
			}, func(temporary, target string) error {
				replaced = true
				if target != filename || filepath.Dir(temporary) != filepath.Dir(filename) {
					t.Error("replacement is not a sibling operation")
				}
				if r, err := zip.OpenReader(temporary); err != nil {
					t.Error("replacement received invalid ZIP", err)
				} else {
					r.Close()
				}
				return fs.ErrPermission
			})
			if !errors.Is(err, fs.ErrPermission) || replaced != (failure == "replace") {
				t.Fatal("failure path lost", err, replaced)
			}
			after, _ := os.ReadFile(filename)
			if !bytes.Equal(original, after) {
				t.Fatal("failed rewrite modified original")
			}
			assertNoZIPTemporary(t, filename)
		})
	}
}

func TestZIPRewriteRejectsConcurrentChanges(t *testing.T) {
	filename := archiveFixture(t)
	other := archiveFixture(t)
	replacement, _ := os.ReadFile(other)
	replacement = append(replacement, []byte("external update")...)
	err := rewriteZIPWith(filename, func(r *zip.Reader, w *zip.Writer) error {
		for _, f := range r.File {
			if err := w.Copy(f); err != nil {
				return err
			}
		}
		return os.WriteFile(filename, replacement, 0600)
	}, func(string, string) error { t.Error("concurrently changed archive replaced"); return nil })
	if !errors.Is(err, ErrArchiveChanged) {
		t.Fatal("concurrent modification not detected", err)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(replacement, after) {
		t.Fatal("external update overwritten")
	}
	assertNoZIPTemporary(t, filename)
}

func TestZIPRewriteRejectsInvalidAndPrefixedArchives(t *testing.T) {
	valid, _ := os.ReadFile(archiveFixture(t))
	for _, data := range [][]byte{[]byte("not a ZIP"), valid[:len(valid)-20], append([]byte("stub"), valid...)} {
		filename := filepath.Join(t.TempDir(), "bad.zip")
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		if result, err := DeleteArchive(Location{Path: filename, Folder: "."}, []string{"a2.txt"}); err == nil || len(result.Names) != 0 {
			t.Fatal("unsupported source changed", result, err)
		}
		after, _ := os.ReadFile(filename)
		if !bytes.Equal(data, after) {
			t.Fatal("invalid source changed")
		}
		assertNoZIPTemporary(t, filename)
	}
}
