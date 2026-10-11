package browser

import (
	"cmp"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestFilenameSort(t *testing.T) {
	for _, tc := range []struct {
		name   string
		folder bool
		want   []string
	}{
		{"originals and copies", false, []string{
			"create-folder.png", "create-folder - Copy.png", "create-folder-large.png",
			"create-folder-large - Copy.png", "create-folder-minimum.png",
		}},
		{"stem before extension", false, []string{
			"report", "report.pdf", "report.txt", "report - Copy.pdf", "report-large.txt",
		}},
		{"numbers and leading zeros", false, []string{
			"file0.txt", "file00.txt", "file2.txt", "file02.txt", "file2 - Copy.txt", "file10.txt",
			"file9999999999999999999999999.txt", "file10000000000000000000000000.txt",
		}},
		{"multiple number segments", false, []string{"v01.2.txt", "v1.10.txt", "v2.1.txt"}},
		{"extensions", false, []string{"item.2", "item.10", "item.PDF", "item.txt"}},
		{"dotfiles", false, []string{".env", ".env.local", ".env - Copy", ".env-long"}},
		{"case and Unicode", false, []string{"FILE2.txt", "file2.txt", "file10.txt", "Résumé2.txt", "résumé10.txt"}},
		{"whole folder names", true, []string{"create-folder-large.v0", "create-folder.v1", "project.2", "project.10"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]Entry, len(tc.want))
			for i, name := range tc.want {
				items[len(items)-i-1] = Entry{Name: name, Directory: tc.folder}
			}
			slices.SortFunc(items, CompareEntries)
			var got []string
			for _, item := range items {
				got = append(got, item.Name)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("sorted names %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadDirectoryUsesNaturalFilenameSort(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"folder10", "folder2"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"create-folder-large.png", "create-folder - Copy.png", "create-folder.png", "file10.txt", "file2.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	directory, err := ReadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range directory.Entries {
		got = append(got, item.Name)
	}
	want := []string{"..", "folder2", "folder10", "create-folder.png", "create-folder - Copy.png", "create-folder-large.png", "file2.txt", "file10.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("directory order %v, want %v", got, want)
	}
}

func TestFilenameComparatorRemainsConsistent(t *testing.T) {
	var entries []Entry
	for _, name := range []string{"", ".env", ".env.local", "A1.png", "a01.txt", "a1.txt", "a1 - Copy.txt", "a2.txt", "a02.txt", "a10.txt", "v01.2", "v1.10", "Résumé2", "résumé10"} {
		entries = append(entries, Entry{Name: name}, Entry{Name: name, Directory: true})
	}
	entries = append(entries, Entry{Name: "..", Parent: true, Directory: true})
	for _, a := range entries {
		for _, b := range entries {
			ab := CompareEntries(a, b)
			if cmp.Compare(ab, 0) != -cmp.Compare(CompareEntries(b, a), 0) {
				t.Fatalf("asymmetric order: %+v %+v", a, b)
			}
			for _, c := range entries {
				if ab <= 0 && CompareEntries(b, c) <= 0 && CompareEntries(a, c) > 0 {
					t.Fatalf("nontransitive order: %+v %+v %+v", a, b, c)
				}
			}
		}
	}
}
