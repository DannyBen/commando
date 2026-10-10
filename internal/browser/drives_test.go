package browser

import (
	"path/filepath"
	"testing"
)

func TestDriveEntries(t *testing.T) {
	entries := driveEntries((1 << 2) | (1 << 3) | (1 << 25))
	want := []string{"C:\\", "D:\\", "Z:\\"}
	if len(entries) != len(want) {
		t.Fatalf("entries=%+v", entries)
	}
	for i, entry := range entries {
		if entry.Name != want[i] || entry.Path != want[i] || !entry.Directory || entry.Parent || entry.SizeKnown {
			t.Fatalf("drive entry=%+v want %q", entry, want[i])
		}
	}
	if len(driveEntries(0)) != 0 {
		t.Fatal("zero mask returned fabricated drives")
	}
}

func TestReadDriveLocation(t *testing.T) {
	directory, err := ReadLocation("")
	if err != nil {
		t.Fatal(err)
	}
	if !directory.Drives {
		t.Fatal("drive location was read as a physical directory")
	}
	for _, entry := range directory.Entries {
		if !filepath.IsAbs(entry.Path) || !entry.Directory || entry.Parent || entry.Name == "" {
			t.Fatalf("invalid drive entry %+v", entry)
		}
	}
	directory, err = ReadLocation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if directory.Drives {
		t.Fatal("physical folder became a drive list")
	}
}
