package app

import (
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func nameAliasView(t *testing.T, descendants bool, sourceAlias bool) (*view, string, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"a.txt", "middle.txt", "z.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	child := filepath.Join(root, "folder", "nested")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skip("directory aliases unavailable:", err)
	}
	paths := []string{root, alias, t.TempDir()}
	if sourceAlias {
		paths[0], paths[1] = paths[1], paths[0]
	}
	if descendants {
		paths[2] = filepath.Join(alias, "folder", "nested")
	}
	v := newBrowserView(paths, nil)
	t.Cleanup(v.loader.close)
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	return v, root, alias
}

func TestCreateUpdatesDirectoryAliasesWithoutChangingOtherCurrentItems(t *testing.T) {
	for _, sourceAlias := range []bool{false, true} {
		for _, folder := range []bool{false, true} {
			v, _, _ := nameAliasView(t, false, sourceAlias)
			for pi := 0; pi < 2; pi++ {
				selectName(t, &v.panels[pi], "middle.txt")
				v.panels[pi].toggleMark(v.panels[pi].selected)
			}
			other := &v.panels[1]
			other.list.Position.First = other.selected
			other.list.Position.Offset = 7
			send, _ := panelInputFrames(v)
			ke := modified("N", key.ModCtrl)
			if folder {
				ke = newFolderKey()
			}
			send(ke)
			send(replaceCreate(v, "b-created"))
			send(pressed(key.NameReturn))
			other.list.Position.First, other.list.Position.Offset = other.selected, 7
			waitCreated(t, v)
			// Assert the viewport before rendering can consume its scroll offset.
			if other.items[other.list.Position.First].name != "middle.txt" || other.list.Position.Offset != 7 {
				t.Fatal("creation shifted the alias viewport")
			}
			send()
			if v.active != 0 || v.panels[0].items[v.panels[0].selected].name != "b-created" || other.items[other.selected].name != "middle.txt" || !other.marks[other.selected] {
				t.Fatal("creation lost source selection or alias current item/mark")
			}
			selectName(t, other, "b-created")
			if other.items[other.selected].file.Directory != folder {
				t.Fatal("alias received wrong created type")
			}
		}
	}
}

func TestRenameUpdatesDirectoryAliasesAndOpenDescendants(t *testing.T) {
	for _, sourceAlias := range []bool{false, true} {
		v, root, _ := nameAliasView(t, true, sourceAlias)
		selectName(t, &v.panels[0], "folder")
		selectName(t, &v.panels[1], "folder")
		v.panels[1].toggleMark(v.panels[1].selected)
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF2))
		send(replaceRename(v, "renamed"))
		send(pressed(key.NameReturn))
		waitRenamed(t, v)
		send()
		for pi := 0; pi < 2; pi++ {
			p := &v.panels[pi]
			if p.items[p.selected].name != "renamed" || pi == 1 && !p.marks[p.selected] {
				t.Fatal("alias lost renamed current item or mark")
			}
		}
		if resolved, err := resolveFileLocation(v.panels[2].path); err != nil || resolved != filepath.Join(root, "renamed", "nested") {
			t.Fatalf("open alias descendant stranded: %s %v", v.panels[2].path, err)
		}
	}
}

func TestRenameFileRetainsAliasSelectionMarksAndViewportAfterReordering(t *testing.T) {
	v, _, _ := nameAliasView(t, false, false)
	selectName(t, &v.panels[0], "a.txt")
	other := &v.panels[1]
	selectName(t, other, "middle.txt")
	other.toggleMark(other.selected)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "zz-last.txt"))
	other.list.Position.First, other.list.Position.Offset = other.selected, 7
	send(pressed(key.NameReturn))
	// The submission frame may lay out the list, so set its anchor afterward.
	other.list.Position.First, other.list.Position.Offset = other.selected, 7
	waitRenamed(t, v)
	if other.items[other.selected].name != "middle.txt" || !other.marks[other.selected] || other.items[other.list.Position.First].name != "middle.txt" || other.list.Position.Offset != 7 {
		t.Fatal("rename shifted alias current item, marks or viewport")
	}
	selectName(t, other, "zz-last.txt")
}

func TestNameCompletionIgnoresPanelWhichLeftCapturedLocation(t *testing.T) {
	v, root, _ := nameAliasView(t, false, false)
	// Capture the operation before navigating, then deliver its completion
	// afterward. A fast worker must not finish before the scenario is arranged.
	request := loadRequest{panel: 0, path: v.panels[0].path, locations: v.fileOperationLocations()}
	result := loadResult{request: request, locations: resolveFileLocations(request.locations),
		mutationDirectory: root, created: browser.Entry{Name: "created", SizeKnown: true}}
	v.panels[1].path = t.TempDir()
	v.applyCreatedEntry(result)
	if p := &v.panels[0]; p.items[p.selected].name != "created" {
		t.Fatal("completion did not update the panel still showing the directory")
	}
	for _, item := range v.panels[1].items {
		if item.name == "created" {
			t.Fatal("completion inserted into a panel which left the directory")
		}
	}
}
