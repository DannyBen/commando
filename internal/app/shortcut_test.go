package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func shortcutView(t *testing.T) (*view, string, string) {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, "target folder")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "folder.LNK")
	if err := os.WriteFile(link, []byte("shortcut fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, root}, nil)
	t.Cleanup(v.finishOperations)
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	p := &v.panels[0]
	p.setFilter(".lnk")
	selectName(t, p, "folder.LNK")
	p.toggleMark(p.selected)
	if p.items[p.selected].file.Directory || len(p.selectedPaths()) != 1 || p.selectedPaths()[0] != link {
		t.Fatal("shortcut was reclassified or operations targeted its destination")
	}
	return v, link, target
}

func TestFolderShortcutNavigatesCurrentPanelWithoutStealingFocus(t *testing.T) {
	v, link, target := shortcutView(t)
	called, release := make(chan string, 1), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	v.loader.folderShortcutTarget = func(path string) (string, error) { called <- path; <-release; return target, nil }
	opened := make(chan string, 1)
	v.openFile = func(path string) error { opened <- path; return nil }
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	select {
	case path := <-called:
		if path != link {
			t.Fatalf("wrong shortcut %q", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shortcut resolution did not run")
	}
	send(pressed(key.NameRightArrow))
	if v.active != 1 || !v.panels[0].loading {
		t.Fatal("resolution blocked panel navigation or lost captured panel")
	}
	close(release)
	released = true
	waitLoaded(t, v, 0)
	send()
	if v.active != 1 || v.panels[0].path != target || v.panels[1].path != filepath.Dir(link) || v.panels[0].filter != "" || len(v.panels[0].marks) != 0 {
		t.Fatal("shortcut failed to navigate its panel through normal folder flow")
	}
	select {
	case path := <-opened:
		t.Fatalf("folder shortcut opened externally: %q", path)
	default:
	}
	if _, err := os.Stat(link); err != nil {
		t.Fatal("shortcut itself changed", err)
	}
}

func TestNonFolderShortcutsKeepOriginalNativeOpening(t *testing.T) {
	for _, resolveErr := range []error{nil, errors.New("broken or malformed shortcut")} {
		t.Run(fmt.Sprint(resolveErr), func(t *testing.T) {
			v, link, _ := shortcutView(t)
			v.loader.folderShortcutTarget = func(string) (string, error) { return "", resolveErr }
			opened := make(chan string, 1)
			v.openFile = func(path string) error { opened <- path; return nil }
			send, _ := panelInputFrames(v)
			p := &v.panels[0]
			selected, position, count := p.selected, p.list.Position, len(p.items)
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			send()
			select {
			case path := <-opened:
				if path != link {
					t.Fatal("native opening lost original shortcut arguments/working directory")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("shortcut did not fall back to native opening")
			}
			if p.path != filepath.Dir(link) || p.selected != selected || len(p.items) != count || len(p.marks) != 1 || !p.marks[selected] || p.filter != ".lnk" || p.list.Position.First != position.First || p.list.Position.Offset != position.Offset || p.navigating || p.err != nil || p.feedback.Current() != nil {
				t.Fatal("native fallback changed listing, marks, filter or feedback")
			}
		})
	}
}

func TestFolderShortcutReadFailureKeepsSourceListing(t *testing.T) {
	v, link, target := shortcutView(t)
	v.loader.folderShortcutTarget = func(string) (string, error) { return target, nil }
	v.loader.readDirectory = func(path string) (browser.Directory, error) {
		if path == target {
			return browser.Directory{}, os.ErrPermission
		}
		return browser.ReadLocation(path)
	}
	opened := make(chan string, 1)
	v.openFile = func(path string) error { opened <- path; return nil }
	send, _ := panelInputFrames(v)
	p := &v.panels[0]
	selected := p.selected
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if p.path != filepath.Dir(link) || p.selected != selected || !p.marks[selected] || !errors.Is(p.err, os.ErrPermission) {
		t.Fatal("failed folder shortcut changed source listing")
	}
	if message := p.feedback.Current(); message == nil || message.Text != "Access denied" {
		t.Fatal("folder read failure did not use normal footer feedback")
	}
	select {
	case <-opened:
		t.Fatal("folder read failure invoked native opener")
	default:
	}
}
