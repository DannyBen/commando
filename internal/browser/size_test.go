package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateSizeCountsFilesIncludingHiddenAndEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested", "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"a.txt": "abc", ".hidden": "12", "nested/b": "four", "nested/zero": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		paths []string
		want  SizeResult
	}{
		{[]string{root}, SizeResult{Bytes: 9, Files: 4}},
		{[]string{filepath.Join(root, "a.txt"), filepath.Join(root, "nested")}, SizeResult{Bytes: 7, Files: 3}},
		{[]string{filepath.Join(root, "nested", "empty")}, SizeResult{}},
	} {
		got, err := CalculateSize(context.Background(), tc.paths)
		if err != nil || got != tc.want {
			t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
		}
	}
}

func TestCalculateSizeRejectsPartialAndCancelledResults(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good")
	if err := os.WriteFile(good, []byte("123"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := CalculateSize(context.Background(), []string{good, filepath.Join(root, "missing")})
	if !errors.Is(err, os.ErrNotExist) || got != (SizeResult{}) {
		t.Fatalf("partial result: %+v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = CalculateSize(ctx, []string{root})
	if !errors.Is(err, context.Canceled) || got != (SizeResult{}) {
		t.Fatalf("cancelled result: %+v, %v", got, err)
	}
}

func TestCalculateSizeDoesNotFollowLinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "data"), []byte("not counted"), 0644); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"loop": root, "outside": outside, "file": filepath.Join(outside, "data"), "broken": filepath.Join(outside, "missing")} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	}
	got, err := CalculateSize(context.Background(), []string{root})
	if err != nil || got != (SizeResult{}) {
		t.Fatalf("followed links: %+v, %v", got, err)
	}
}
