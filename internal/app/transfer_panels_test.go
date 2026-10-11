package app

import (
	"io/fs"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

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
