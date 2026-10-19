package windowshell

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSiblingPaths(t *testing.T) {
	base := t.TempDir()
	valid := []string{filepath.Join(base, "two words.txt"), filepath.Join(base, "café-日本語.txt")}
	got, err := siblingPaths(valid, false)
	if err != nil || !reflect.DeepEqual(got, valid) {
		t.Fatalf("valid siblings=%v %v", got, err)
	}
	for _, paths := range [][]string{
		nil, {"relative.txt"}, {filepath.Join(base, "bad") + "\x00"},
		{filepath.Join(base, "a"), filepath.Join(base, "other", "b")},
	} {
		if _, err := siblingPaths(paths, false); err == nil {
			t.Fatalf("accepted invalid selection %q", paths)
		}
	}
}

func TestOnlyPropertiesAcceptsSingleDriveRoot(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	if got, err := siblingPaths([]string{root}, true); err != nil || !reflect.DeepEqual(got, []string{root}) {
		t.Fatal("properties refused a drive root", got, err)
	}
	if _, err := siblingPaths([]string{root}, false); err == nil {
		t.Fatal("ordinary shell actions accepted a drive root")
	}
	if _, err := siblingPaths([]string{root, root}, true); err == nil {
		t.Fatal("properties accepted multiple drive roots")
	}
}
