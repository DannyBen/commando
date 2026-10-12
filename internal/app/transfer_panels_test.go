package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestPartialCopyDeleteMoveRefreshesOpenChildrenAndRetainsCutFolder(t *testing.T) {
	for _, removedFolder := range []bool{false, true} {
		v, source, target := clipboardView(t)
		c := attachTestFileClipboard(v)
		folder := filepath.Join(source, "folder")
		if err := os.WriteFile(filepath.Join(folder, "nested", "stay"), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		v.load(2, filepath.Join(folder, "nested"), "")
		waitLoaded(t, v, 2)
		selectName(t, &v.panels[2], "file")
		v.panels[2].toggleMark(v.panels[2].selected)
		selectName(t, &v.panels[2], "stay")
		v.panels[2].toggleMark(v.panels[2].selected)
		selectName(t, &v.panels[0], "folder")
		v.panels[0].toggleMark(v.panels[0].selected)
		v.loader.executeTransfer = func(plan *browser.TransferPlan, move bool, policy browser.TransferPolicy) (browser.TransferResult, error) {
			// Reproduce a published cross-volume copy with deletion stopping
			// inside its tree. Browser tests verify how these results are made.
			result, err := browser.ExecuteTransfer(plan, false, policy)
			if err != nil {
				return result, err
			}
			child := filepath.Join(folder, "nested", "file")
			if err := os.Remove(child); err != nil {
				return result, err
			}
			result.Changes = append(result.Changes, browser.TransferredPath{Source: child, Destination: filepath.Join(target, "folder", "nested", "file"), Moved: true})
			if removedFolder {
				if err := os.Remove(filepath.Join(folder, "nested", "stay")); err != nil {
					return result, err
				}
				if err := os.Remove(filepath.Join(folder, "nested")); err != nil {
					return result, err
				}
				result.Changes = append(result.Changes, browser.TransferredPath{Source: filepath.Join(folder, "nested"), Destination: filepath.Join(target, "folder", "nested"), Directory: true, Moved: true})
			}
			return result, fs.ErrPermission
		}
		send, _ := panelInputFrames(v)
		send(modified("X", key.ModCtrl))
		waitLoaded(t, v, 0)
		send()
		send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
		waitLoaded(t, v, 1)
		send()
		p := &v.panels[2]
		wantPath := filepath.Join(folder, "nested")
		if removedFolder {
			wantPath = filepath.Join(target, "folder", "nested")
		}
		if p.path != wantPath || p.items[p.selected].name != "stay" || !p.marks[p.selected] || v.active != 1 {
			t.Fatal("partial move stranded child or lost current item, mark or focus")
		}
		if !removedFolder {
			for _, item := range p.items {
				if item.name == "file" {
					t.Fatal("removed child retained in cached listing")
				}
			}
			if len(p.marks) != 1 {
				t.Fatal("removed child retained its mark")
			}
		}
		clipboard, err := c.read()
		if err != nil || len(clipboard.Paths) != 1 || clipboard.Paths[0] != folder || !clipboard.Move {
			t.Fatalf("partly moved folder consumed from cut: %+v %v", clipboard, err)
		}
		if _, err := os.Stat(filepath.Join(target, "folder", "nested", "file")); errors.Is(err, fs.ErrNotExist) {
			t.Fatal("confirmed copied child missing")
		}
	}
}

func TestMergedMoveRefreshFailureStillFollowsConfirmedTarget(t *testing.T) {
	v, _, source, target := batchConflictView(t, true)
	v.load(2, filepath.Join(source, "folder", "nested"), "")
	waitLoaded(t, v, 2)
	selectName(t, &v.panels[2], "file")
	v.panels[2].toggleMark(v.panels[2].selected)
	v.loader.readDirectory = func(path string) (browser.Directory, error) {
		if path == filepath.Join(target, "folder", "nested") {
			return browser.Directory{}, fs.ErrPermission
		}
		return browser.ReadLocation(path)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	p := &v.panels[2]
	if p.path != filepath.Join(target, "folder", "nested") || p.items[p.selected].name != "file" || !p.marks[p.selected] {
		t.Fatal("failed listing refresh stranded a confirmed moved folder")
	}
	if message := p.feedback.Current(); message == nil || message.Kind != feedback.Warning || message.Text != "Cannot refresh folder" {
		t.Fatal("failed refresh lost its feedback")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Warning || message.Text != "3 transferred; refresh failed" {
		t.Fatal("successful transfer concealed a failed listing refresh")
	}
}
