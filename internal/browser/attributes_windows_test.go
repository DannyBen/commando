package browser

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsCreationTimeSurvivesReadsCreationAndTransferMetadata(t *testing.T) {
	root := t.TempDir()
	item, err := CreateFile(root, "dates.txt")
	if err != nil || item.Created.IsZero() {
		t.Fatal("new file lost creation time", err)
	}
	folder, err := CreateFolder(root, "folder")
	if err != nil || folder.Created.IsZero() {
		t.Fatal("new folder lost creation time", err)
	}
	path := filepath.Join(root, item.Name)
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2020, time.May, 6, 12, 0, 0, 0, time.UTC)
	stamp := windows.NsecToFiletime(created.UnixNano())
	err = windows.SetFileTime(windows.Handle(file.Fd()), &stamp, nil, nil)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	directory, err := ReadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range directory.Entries {
		if entry.Name == item.Name {
			found = true
			if !entry.Created.Equal(created) || entry.Modified.Equal(created) {
				t.Fatal("directory listing confused creation and modification time")
			}
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !entryFromInfo(path, info).Created.Equal(created) {
		t.Fatal("creation time missing from listing or transfer metadata")
	}
}

func TestWindowsDirectoryReadsHiddenAndSystemAttributes(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name      string
		directory bool
		hidden    bool
		system    bool
	}{
		{"ordinary.txt", false, false, false},
		{".env", false, false, false},
		{"hidden.txt", false, true, false},
		{"hidden-folder", true, true, false},
		{"system-only.txt", false, false, true},
		{"$RECYCLE.BIN", true, true, true},
	} {
		path := filepath.Join(root, tc.name)
		if tc.directory {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			t.Fatal(err)
		}
		attributes := uint32(windows.FILE_ATTRIBUTE_NORMAL)
		if tc.hidden || tc.system {
			attributes = 0
		}
		if tc.hidden {
			attributes |= windows.FILE_ATTRIBUTE_HIDDEN
		}
		if tc.system {
			attributes |= windows.FILE_ATTRIBUTE_SYSTEM
		}
		if err := windows.SetFileAttributes(name, attributes); err != nil {
			t.Fatal(err)
		}
		directory, err := ReadDirectory(root)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range directory.Entries {
			if item.Name == tc.name {
				found = true
				if item.Hidden != tc.hidden || item.System != tc.system || item.Directory != tc.directory {
					t.Fatalf("directory attributes: %+v", item)
				}
			}
		}
		if !found {
			t.Fatal("directory worker discarded an entry before pane filtering")
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		item := entryFromInfo(path, info)
		if item.Hidden != tc.hidden || item.System != tc.system {
			t.Fatal("transfer/duplicate metadata lost Windows attributes")
		}
	}
}

func TestWindowsVisibleLinkToHiddenFolderKeepsItsOwnAttributes(t *testing.T) {
	root := t.TempDir()
	target, link := filepath.Join(root, "hidden-target"), filepath.Join(root, "visible-link")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	directory, err := ReadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range directory.Entries {
		if item.Name == "visible-link" {
			if !item.Directory || item.Hidden {
				t.Fatal("folder-link navigation inherited the target's hidden attribute")
			}
			return
		}
	}
	t.Fatal("visible link missing from directory read")
}
