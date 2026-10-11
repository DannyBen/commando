package app

import (
	"reflect"
	"strings"
	"testing"

	"gioui.org/io/key"
)

func TestDuplicateRenameAndRefreshUseSameFilenameOrder(t *testing.T) {
	v, root, _ := clipboardView(t)
	attachTestFileClipboard(v)
	writeBatchFiles(t, root, map[string]string{
		"create-folder.png": "original", "create-folder-large.png": "larger",
	})
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], "create-folder.png")
	v.panels[0].toggleMark(v.panels[0].selected)
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	assertFilenameOrder(t, &v.panels[0], []string{"create-folder.png", "create-folder - Copy.png", "create-folder-large.png"})
	if v.panels[0].items[v.panels[0].selected].name != "create-folder - Copy.png" {
		t.Fatal("sorting lost the new copy selection")
	}
	assertMarkedOriginal := func() {
		t.Helper()
		p := &v.panels[0]
		if len(p.marks) != 1 {
			t.Fatal("reordering changed original marks")
		}
		for row := range p.marks {
			if p.items[row].name != "create-folder.png" {
				t.Fatal("original mark moved to a different file")
			}
		}
	}
	assertMarkedOriginal()
	send(pressed(key.NameF2))
	send(replaceCreate(v, "create-folder-minimum.png"), pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	want := []string{"create-folder.png", "create-folder-large.png", "create-folder-minimum.png"}
	assertFilenameOrder(t, &v.panels[0], want)
	assertMarkedOriginal()
	if v.panels[0].items[v.panels[0].selected].name != "create-folder-minimum.png" {
		t.Fatal("sorting lost the renamed item selection")
	}
	send(refreshKey())
	waitLoaded(t, v, 0)
	send()
	assertFilenameOrder(t, &v.panels[0], want)
	if v.panels[0].items[v.panels[0].selected].name != "create-folder-minimum.png" {
		t.Fatal("refresh changed the current item after sorting")
	}
}

func assertFilenameOrder(t *testing.T, p *panel, want []string) {
	t.Helper()
	var got []string
	for _, item := range p.items {
		if strings.HasPrefix(item.name, "create-folder") {
			got = append(got, item.name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filename order %v, want %v", got, want)
	}
}
