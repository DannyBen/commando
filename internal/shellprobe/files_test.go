package shellprobe

import (
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dannyben/commando/internal/browser"
)

func TestSampleFiles(t *testing.T) {
	root, err := sampleFiles()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	dir, err := browser.ReadDirectory(filepath.Join(root, "source"))
	if err != nil {
		t.Fatal(err)
	}
	var list fileList
	list.replace(dir.Entries)
	if len(list.items) != 5 {
		t.Fatalf("sample items=%v", list.items)
	}
	for _, name := range []string{"alpha.txt", "two words.txt", "café-日本語.txt", "Folder sample/inside.txt"} {
		data, err := os.ReadFile(filepath.Join(root, "source", filepath.FromSlash(name)))
		if err != nil || len(data) == 0 {
			t.Fatalf("sample %q: %v", name, err)
		}
	}
	f, err := os.Open(filepath.Join(root, "source", "image.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := png.Decode(f); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "destination"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("destination should be empty: %v %v", entries, err)
	}
}

func TestFileSubjectsAndRefresh(t *testing.T) {
	folder := t.TempDir()
	l := fileList{}
	l.replace([]browser.Entry{{Name: "..", Parent: true}, {Name: "a"}, {Name: "b"}, {Name: "c"}})
	assertPaths := func(names ...string) {
		t.Helper()
		var want []string
		for _, name := range names {
			want = append(want, filepath.Join(folder, name))
		}
		if got := l.paths(folder); !reflect.DeepEqual(got, want) {
			t.Fatalf("subjects=%v want=%v", got, want)
		}
	}
	assertPaths("a")
	l.selectItem("a", true)
	l.selectItem("b", true)
	l.selectItem("a", false)
	assertPaths("a", "b")
	l.selectItem("c", false)
	assertPaths("c")
	l.selectItem("b", true)
	l.selectItem("c", true)
	l.replace([]browser.Entry{{Name: "c"}, {Name: "b"}, {Name: "a"}})
	assertPaths("c", "b")
	if l.current != "c" {
		t.Fatal("reordering lost current identity")
	}
	l.replace([]browser.Entry{{Name: "b"}, {Name: "a"}})
	assertPaths("b")
	if l.current != "b" || l.marks["c"] {
		t.Fatal("refresh retained a vanished item")
	}
	l.replace(nil)
	l.selectItem("", true)
	assertPaths()
}

func TestActionSnapshotAndBusyGuard(t *testing.T) {
	v := newView(t.TempDir(), []browser.Entry{{Name: "a"}, {Name: "b"}}, log.New(io.Discard, "", 0))
	v.files.selectItem("a", true)
	v.files.selectItem("b", true)
	v.start("drag", false)
	if !v.busy || v.pending == nil || len(v.pending.paths) != 2 {
		t.Fatal("batch action was not queued")
	}
	paths := append([]string(nil), v.pending.paths...)
	v.files.selectItem("a", false)
	v.start("menu", true)
	if v.pending.kind != "drag" || !reflect.DeepEqual(paths, v.pending.paths) {
		t.Fatal("busy action or live selection replaced the snapshot")
	}
	v.complete(outcome{request: *v.pending, directory: browser.Directory{Entries: []browser.Entry{{Name: "b"}}}})
	if v.busy || len(v.files.items) != 1 || v.files.current != "b" {
		t.Fatal("completion did not refresh and release the busy guard")
	}
}
