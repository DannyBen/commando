package app

import (
	"errors"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestEmptyFolderCueDescribesCachedListing(t *testing.T) {
	parent := browser.Entry{Name: "..", Parent: true, Directory: true}
	for _, test := range []struct {
		name  string
		panel panel
		want  string
	}{
		{"empty", panel{path: "/folder", entries: []browser.Entry{parent}}, "Empty folder"},
		{"parent visible", panel{path: "/folder", showParentEntry: true}, ""},
		{"hidden entries", panel{path: "/folder", entries: []browser.Entry{{Name: ".hidden"}}}, "No visible entries"},
		{"filtered entries", panel{path: "/folder", filter: "other", entries: []browser.Entry{{Name: "file"}}}, "No matching entries"},
		{"empty with filter", panel{path: "/folder", filter: "other", entries: []browser.Entry{parent}}, "Empty folder"},
		{"populated", panel{path: "/folder", items: []entry{{name: "file"}}}, ""},
		{"drives", panel{drives: true}, ""},
		{"loading", panel{path: "/folder", loading: true}, ""},
		{"failed read", panel{path: "/folder", err: errors.New("unreadable")}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.panel.emptyFolderLabel(); got != test.want {
				t.Fatalf("cue = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEmptyFolderCueFocusesWithoutCreatingAnEntry(t *testing.T) {
	v := selectionView()
	p := &v.panels[1]
	p.items, p.entries, p.selected, p.showParentEntry = nil, nil, -1, false
	send, _ := panelInputFrames(v)
	// Click the cue itself, then try the normal entry actions.
	rowClick(send, f32.Pt(450, 65), time.Second, 0)
	send(pressed(key.NameReturn), pressed(key.NameDeleteForward), pressed(insertKey))
	if v.active != 1 || p.selected != -1 || len(p.items) != 0 || len(p.marks) != 0 || !v.interaction.idle() || p.loading {
		t.Fatal("passive cue failed to focus or became an actionable entry")
	}
	// Creating a file still uses the existing draft rather than the cue.
	send(modified("N", key.ModCtrl))
	if v.nameInteraction() == nil || !v.nameInteraction().draft() || len(p.items) != 0 {
		t.Fatal("empty-folder cue interfered with new-file creation")
	}
}
