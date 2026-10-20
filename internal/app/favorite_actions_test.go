package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func homeActionsView(t *testing.T) (*view, string) {
	t.Helper()
	v, target := favoriteTestView(t)
	for _, name := range []string{"Alpha", "Work"} {
		if _, err := v.loader.createFavorite(v.favoritesDirectory(), name, target); err != nil {
			t.Fatal(err)
		}
	}
	v.load(0, "", "Work.lnk")
	waitLoaded(t, v, 0)
	return v, target
}

func TestHomeFavoriteRenameRetainsShortcutAndTarget(t *testing.T) {
	v, target := homeActionsView(t)
	v.panels = append(v.panels, v.panels[0].clone())
	p := &v.panels[0]
	p.toggleMark(p.selected)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	if n := v.nameInteraction(); n == nil || n.path != v.favoritesDirectory() || v.nameEditor.Text() != "Work" || v.nameEditor.SelectedText() != "Work" {
		t.Fatal("Home rename did not edit the shortcut's friendly name")
	}
	send(replaceRename(v, "Alpha"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if n := v.nameInteraction(); n == nil || !errors.Is(n.err, fs.ErrExist) {
		t.Fatal("name collision did not retain the editor")
	}
	send(replaceRename(v, "Zed.notes"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if !v.interaction.idle() || !p.drives || p.items[p.selected].file.Name != "Zed.notes.lnk" || !p.marks[p.selected] || p.items[0].file.Name != "Alpha.lnk" || !p.items[1].file.Favorite || p.items[2].file.Favorite {
		t.Fatal("renaming lost Home order, selection, or marks", visibleNames(p))
	}
	link := filepath.Join(v.favoritesDirectory(), "Zed.notes.lnk")
	if p.items[p.selected].file.Path != link {
		t.Fatal("rename left the old shortcut path cached")
	}
	peer := &v.panels[1]
	if !peer.drives || peer.items[peer.selected].file.Name != "Zed.notes.lnk" || peer.items[peer.selected].file.Path != link {
		t.Fatal("rename did not reconcile another Home panel")
	}
	data, err := os.ReadFile(link)
	if err != nil || string(data) != target {
		t.Fatal("rename changed the shortcut target", err)
	}
	if _, err := os.Stat(filepath.Join(target, "child")); err != nil {
		t.Fatal("rename changed the target folder", err)
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if p.drives || p.path != target {
		t.Fatal("renamed shortcut no longer opens its target")
	}
}

func TestHomeRemovalCapturesOnlyShortcutFiles(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(map[bool]string{false: "recycle", true: "delete"}[permanent], func(t *testing.T) {
			v, target := homeActionsView(t)
			p := &v.panels[0]
			v.ownerWindow = 123
			v.loader.recycle = func(directory string, names []string, owner uintptr) (browser.RemovalResult, error) {
				if directory != v.favoritesDirectory() || owner != 123 || !slices.Equal(names, []string{"Alpha.lnk", "Work.lnk"}) {
					t.Errorf("wrong removal source: %s %v owner=%d", directory, names, owner)
					return browser.RemovalResult{}, fs.ErrPermission
				}
				if err := os.Remove(filepath.Join(directory, names[0])); err != nil {
					return browser.RemovalResult{}, err
				}
				return browser.RemovalResult{Directory: directory, Names: names[:1]}, fs.ErrPermission
			}
			v.loader.delete = v.loader.recycle
			guard := "guards.recycle_confirmation.multiple_entries"
			if permanent {
				guard = "guards.delete_confirmation.multiple_entries"
			}
			v.settingsValues.Set(settings.CatalogueDefault, guard, settings.Value{Text: "ON"})
			p.markAll()
			p.selected = len(p.items) - 1 // Marks take precedence over a highlighted drive.
			send, _ := panelInputFrames(v)
			modifiers := key.Modifiers(0)
			if permanent {
				modifiers = key.ModShift
			}
			send(modified(key.NameDeleteForward, modifiers))
			if v.interaction.idle() || p.loading {
				t.Fatal("removal skipped confirmation")
			}
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			send()
			if !p.drives || !slices.Contains(visibleNames(p), "Work.lnk") || slices.Contains(visibleNames(p), "Alpha.lnk") || len(p.marks) != 1 {
				t.Fatal("partial removal lost the surviving shortcut, drive, or mark", visibleNames(p))
			}
			if _, err := os.Stat(filepath.Join(target, "child")); err != nil {
				t.Fatal("removal touched the target", err)
			}
		})
	}
}

func TestHomeFavoriteClipboardAndNativeActionsUseLinks(t *testing.T) {
	v, _ := homeActionsView(t)
	p := &v.panels[0]
	p.markAll()
	want := []string{filepath.Join(v.favoritesDirectory(), "Alpha.lnk"), filepath.Join(v.favoritesDirectory(), "Work.lnk")}
	// Clipboard capture resolves parent aliases, while native selection retains
	// the displayed spelling. Windows TempDir can use an 8.3 user-directory name.
	backing, err := resolveFileLocation(v.favoritesDirectory())
	if err != nil {
		t.Fatal(err)
	}
	wantClipboard := []string{filepath.Join(backing, "Alpha.lnk"), filepath.Join(backing, "Work.lnk")}
	v.loader.writeClipboard = func(paths []string, move bool) (browser.FileClipboard, error) {
		if !slices.EqualFunc(paths, wantClipboard, samePath) {
			t.Errorf("clipboard captured %v, want shortcut paths %v", paths, wantClipboard)
		}
		return browser.FileClipboard{Paths: paths, Move: move}, nil
	}
	send, _ := panelInputFrames(v)
	for _, name := range []key.Name{"C", "X"} {
		send(modified(name, key.ModCtrl))
		waitLoaded(t, v, 0)
		send()
	}
	if !v.cutEntry(0, 0) || !v.cutEntry(0, 1) || v.cutEntry(0, 2) {
		t.Fatal("cut highlighting included drives or omitted links")
	}
	v.shellExecutor = func(shellRequest) shellCompletion { return shellCompletion{} }
	var ops op.Ops
	gtx := layout.Context{Ops: &ops}
	for _, action := range []shellAction{shellMenu, shellProperties, shellDrag} {
		if action == shellDrag {
			v.startDrag(gtx, 0, p.selected)
		} else {
			v.startItemShellAction(gtx, 0, p.selected, false, action)
		}
		s, ok := v.interaction.current.(*nativeSession)
		if !ok || !reflect.DeepEqual(s.request.paths, want) {
			t.Fatalf("action %v did not capture shortcut paths", action)
		}
		v.interaction.finish(s)
	}
	v.startContextMenu(gtx, 0, 0, true)
	s := v.interaction.current.(*nativeSession)
	if !slices.Equal(s.request.paths, want[:1]) {
		t.Fatal("mouse menu used marks instead of clicked link")
	}
	v.interaction.finish(s)
	p.clearMarks()
	p.selected = 2
	if removalReady(v) || clipboardReady(v) || panelSourceReady(v, true) || p.markable(2) || v.nativeItemsAvailable(0, 2, false, shellMenu) {
		t.Fatal("drive inherited shortcut mutation actions")
	}
}

func TestHomeRefreshReconcilesExternalShortcutChanges(t *testing.T) {
	v, target := homeActionsView(t)
	p := &v.panels[0]
	p.toggleMark(p.selected)
	if _, err := v.loader.createFavorite(v.favoritesDirectory(), "New", target); err != nil {
		t.Fatal(err)
	}
	p.refreshNeeded = true
	// Queue without rendering extra frames, which can already consume the
	// completion and clear refreshing on a fast worker.
	runAutomaticRefresh(v, time.Now())
	if !p.refreshing {
		t.Fatal("Home did not queue a quiet refresh")
	}
	finishAutomaticRefresh(t, v)
	if !p.drives || p.items[p.selected].file.Name != "Work.lnk" || !p.marks[p.selected] || len(v.favorites) != 3 || !slices.Contains(visibleNames(p), "New.lnk") {
		t.Fatal("Home refresh lost selection or did not reload shortcuts", visibleNames(p))
	}
}

func TestHomeFavoriteMoveToPanelUsesShortcutSource(t *testing.T) {
	v, target := homeActionsView(t)
	destination := t.TempDir()
	v.panels = append(v.panels, v.panels[0].clone())
	v.load(1, destination, "")
	waitLoaded(t, v, 1)
	source := filepath.Join(v.favoritesDirectory(), "Work.lnk")
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		if !slices.Equal(request.Paths, []string{source}) || request.Directory != destination || !request.Move {
			t.Errorf("transfer did not capture the link: %+v", request)
			return browser.TransferResult{}, fs.ErrInvalid
		}
		if err := os.Rename(source, filepath.Join(destination, "Work.lnk")); err != nil {
			return browser.TransferResult{}, err
		}
		return browser.TransferResult{Attempted: true, Directory: destination,
			Items: []browser.TransferredItem{{Source: source, Entry: browser.Entry{Name: "Work.lnk"}, Moved: true}},
		}, nil
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF6))
	if v.inlineInteraction() == nil {
		t.Fatal("Home did not offer the panel destination chooser")
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	if v.panels[0].refreshing {
		v.acceptLoad(<-v.loader.results)
	}
	if !v.panels[0].drives || slices.Contains(visibleNames(&v.panels[0]), "Work.lnk") || !slices.Contains(visibleNames(&v.panels[1]), "Work.lnk") {
		t.Fatalf("move did not reconcile Home and the destination: home=%v dest=%v refreshing=%v needed=%v feedback=%+v", visibleNames(&v.panels[0]), visibleNames(&v.panels[1]), v.panels[0].refreshing, v.panels[0].refreshNeeded, v.panels[0].feedback.Current())
	}
	data, err := os.ReadFile(filepath.Join(destination, "Work.lnk"))
	if err != nil || string(data) != target {
		t.Fatal("move changed the link target", err)
	}
	if _, err := os.Stat(filepath.Join(target, "child")); err != nil {
		t.Fatal("move touched the target directory", err)
	}
}
