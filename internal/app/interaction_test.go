package app

import (
	"errors"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
)

func TestForegroundSessionRejectsCompetingInteractions(t *testing.T) {
	v := newPreviewView()
	v.panels[0].selected = 1
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	owner := v.nameInteraction()
	text, selection := v.nameEditor.Text(), v.nameEditor.SelectedText()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops}
	v.startLocation(gtx)
	v.startInlineActions(0, 2, "", "", []string{"Choose"}, nil, nil)
	v.beginName(gtx, &nameSession{panel: 0, kind: nameCreateFile}, "competing", false)
	v.browseKey(gtx, modified("A", key.ModCtrl))
	if owner == nil || v.nameInteraction() != owner || v.locationEditing() || v.inlineInteraction() != nil ||
		v.nameEditor.Text() != text || v.nameEditor.SelectedText() != selection {
		t.Fatal("competing interaction replaced or disturbed the active editor")
	}
	send(pressed(key.NameEscape), modified("D", key.ModAlt))
	if !v.locationEditing() || v.nameInteraction() != nil {
		t.Fatal("cancel did not release the session for the next interaction")
	}
	if v.interaction.finish(owner) || !v.locationEditing() {
		t.Fatal("stale session ended a newer interaction")
	}
	send(pressed(key.NameEscape), pressed(key.NameDownArrow))
	if !v.interaction.idle() || v.panels[0].selected != 2 {
		t.Fatal("leaving the editor did not restore browsing focus")
	}
}

func TestSubmittedNameCannotLeaveBeforeReconciliation(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	v.panels[0].selected = 1
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "new-name"), pressed(key.NameReturn))
	owner := v.nameInteraction()
	var ops op.Ops
	if owner == nil || !owner.pending || v.cancelInteraction(layout.Context{Ops: &ops}) ||
		v.nameInteraction() != owner || !v.mutationsBusy() {
		t.Fatal("submitted name mutation lost its foreground or operation state")
	}
	request := <-v.loader.requests
	v.acceptLoad(loadResult{request: request, err: errors.New("rename failed")})
	if v.nameInteraction() != owner || owner.pending || v.mutationsBusy() || v.nameEditor.Text() != "new-name" {
		t.Fatal("failed completion did not retain an editable draft and release the worker")
	}
	send(pressed(key.NameEscape))
	if !v.interaction.idle() {
		t.Fatal("reconciled draft could not be cancelled")
	}
}

func TestBackgroundReadPreservesForegroundSession(t *testing.T) {
	for _, kind := range []string{"location", "choice"} {
		t.Run(kind, func(t *testing.T) {
			v := newPreviewView()
			send, _ := panelInputFrames(v)
			if kind == "location" {
				send(modified("D", key.ModAlt))
			} else {
				v.startInlineActions(0, 2, "", "", []string{"Choose"}, nil, nil)
			}
			owner, active := v.interaction.current, v.active
			text, selection := v.location.Text(), v.location.SelectedText()
			v.panels[1].loading = true
			v.acceptLoad(loadResult{request: loadRequest{panel: 1, refresh: true, action: actionBrowse},
				directory: browser.Directory{Path: v.panels[1].path, Entries: []browser.Entry{{Name: "external.txt"}}}})
			if v.interaction.current != owner || v.active != active || v.panels[1].loading ||
				v.location.Text() != text || v.location.SelectedText() != selection {
				t.Fatal("background refresh changed foreground state, text or focus")
			}
			if len(v.panels[1].items) != 1 || v.panels[1].items[0].file.Name != "external.txt" {
				t.Fatal("foreground interaction blocked an unrelated panel's completed read")
			}
		})
	}
}
