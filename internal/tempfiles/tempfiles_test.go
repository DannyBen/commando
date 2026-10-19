package tempfiles

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupSessionAndExpiredOpenCopies(t *testing.T) {
	scratch := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, scratch)
	}
	now := time.Now()
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	recent, err := NewOpen()
	if err != nil {
		t.Fatal(err)
	}
	expired, err := NewOpen()
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(scratch, "commando-archive-old")
	unrelated := filepath.Join(scratch, "other-application")
	for _, directory := range []string{session, recent, expired, legacy, unrelated} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(directory, "file.txt")
		if err := os.WriteFile(file, []byte("contents"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(file, 0400); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = Remove(scratch) })
	for _, directory := range []string{expired, legacy, unrelated} {
		old := now.Add(-openRetention - time.Hour)
		if err := os.Chtimes(directory, old, old); err != nil {
			t.Fatal(err)
		}
	}
	clean(now)
	for _, directory := range []string{session, expired, legacy} {
		if _, err := os.Stat(directory); !os.IsNotExist(err) {
			t.Fatalf("temporary directory survived cleanup: %s: %v", directory, err)
		}
	}
	for _, directory := range []string{recent, unrelated} {
		if _, err := os.ReadFile(filepath.Join(directory, "file.txt")); err != nil {
			t.Fatal("cleanup removed a retained or unrelated copy", err)
		}
	}
	clean(now.Add(openRetention + time.Hour))
	if _, err := os.Stat(recent); !os.IsNotExist(err) {
		t.Fatal("retained copy never expired", err)
	}
}
