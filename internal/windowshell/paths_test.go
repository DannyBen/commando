package windowshell

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSiblingPaths(t *testing.T) {
	base := t.TempDir()
	valid := []string{filepath.Join(base, "two words.txt"), filepath.Join(base, "café-日本語.txt")}
	got, err := siblingPaths(valid)
	if err != nil || !reflect.DeepEqual(got, valid) {
		t.Fatalf("valid siblings=%v %v", got, err)
	}
	for _, paths := range [][]string{
		nil, {"relative.txt"}, {filepath.Join(base, "bad") + "\x00"},
		{filepath.Join(base, "a"), filepath.Join(base, "other", "b")},
	} {
		if _, err := siblingPaths(paths); err == nil {
			t.Fatalf("accepted invalid selection %q", paths)
		}
	}
}
