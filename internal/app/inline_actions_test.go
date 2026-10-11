package app

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/feedback"
)

func TestInlineActionsKeepUnderlyingRowsMarksAndSelection(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	before := slices.Clone(v.panels[0].items)
	selected, position := v.panels[0].selected, v.panels[0].list.Position
	marks := map[int]bool{}
	for row, value := range v.panels[0].marks {
		marks[row] = value
	}
	choice := -1
	cancelled := false
	v.startInlineActions(0, 4, "Copy: report.pdf", "File already exists", []string{"Overwrite", "Cancel"}, func(index int) { choice = index }, func() { cancelled = true })
	send()
	for row := 4; row < 8; row++ {
		if v.storedRow(0, row) != -1 {
			t.Fatal("action row became a stored entry")
		}
	}
	if v.storedRow(0, 8) != 4 || v.storedRow(0, 3) != 3 {
		t.Fatal("action rows shifted file identity incorrectly")
	}
	send(pressed(key.NameUpArrow), pressed(key.NameDownArrow), pressed(key.NameDownArrow), pressed(key.NameDownArrow))
	if v.actions.selected != 1 || v.panels[0].selected != selected || !slices.Equal(v.panels[0].items, before) || !reflect.DeepEqual(v.panels[0].marks, marks) {
		t.Fatal("choice navigation changed real files or wrapped")
	}
	send(pressed(key.NameEscape))
	if v.actions != nil || !cancelled || choice != -1 || v.panels[0].list.Position != position {
		t.Fatal("cancel did not restore temporary list state")
	}
}

func TestInlineActionClickSelectsAndEnterChooses(t *testing.T) {
	v := newPreviewView()
	choice := -1
	v.startInlineActions(0, 4, "Copy: report.pdf", "File already exists", []string{"Overwrite", "Cancel"}, func(index int) { choice = index }, nil)
	send, _ := panelInputFrames(v)
	v.panels[0].rows[7].Click()
	send()
	if v.actions == nil || v.actions.selected != 1 || choice != -1 {
		t.Fatal("click executed a choice or selected wrong option")
	}
	send(pressed(key.NameReturn))
	if v.actions != nil || choice != 1 {
		t.Fatal("Enter did not choose clicked option")
	}
}

func TestRealRowClickCancelsInlineActionsAndMapsBelowGroup(t *testing.T) {
	v := newPreviewView()
	cancelled := false
	v.startInlineActions(0, 4, "Copy: report.pdf", "File already exists", []string{"Overwrite", "Cancel"}, nil, func() { cancelled = true })
	v.actions.subjectRow = 3
	send, _ := panelInputFrames(v)
	v.panels[0].rows[9].Click()
	send()
	if v.actions != nil || !cancelled || v.panels[0].selected != 5 {
		t.Fatal("click below temporary actions did not cancel and select real row")
	}
}

func TestInlineActionsPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if directory == "" {
		t.Skip("optional headless action-row preview")
	}
	for _, fixture := range []struct {
		name     string
		row      int
		options  []string
		selected int
		message  string
	}{
		{"inline-overwrite.png", 6, []string{"Overwrite", "Cancel"}, 0, "File exists"},
		{"inline-cancel.png", 6, []string{"Overwrite", "Cancel"}, 1, "File exists"},
		{"batch-overwrite.png", 5, []string{"Overwrite all", "Skip existing", "Cancel"}, 0, "3 files exist"},
		{"batch-skip.png", 5, []string{"Overwrite all", "Skip existing", "Cancel"}, 1, "3 files exist"},
		{"batch-cancel.png", 5, []string{"Overwrite all", "Skip existing", "Cancel"}, 2, "3 files exist"},
	} {
		v := newPreviewView()
		v.active = 1
		v.startInlineActions(1, fixture.row, "", "", fixture.options, nil, nil)
		v.actions.subjectRow = fixture.row - 1
		v.actions.selected = fixture.selected
		v.showFeedback(1, feedback.Warning, fixture.message)
		// Capture the held footer text without waiting for its typing animation.
		v.panels[1].feedback.Frame(time.Now().Add(-time.Minute), v.appearance.Feedback)
		if err := renderViewPNG(filepath.Join(directory, fixture.name), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInlineSingleActionUsesSameListInteraction(t *testing.T) {
	v := newPreviewView()
	chosen := false
	v.startInlineActions(0, 4, "", "", []string{"Copy here"}, func(int) { chosen = true }, nil)
	send, _ := panelInputFrames(v)
	if row, count := v.transientRows(0); row != 4 || count != 1 {
		t.Fatal("single action could not use one temporary row")
	}
	send(pressed(key.NameReturn))
	if !chosen || v.actions != nil {
		t.Fatal("single action did not use shared Enter behavior")
	}
}

func TestInlineActionsStayOpenWhenPanelSwitchHitsOuterEdge(t *testing.T) {
	for _, tc := range []struct {
		panel int
		event key.Event
	}{
		{0, pressed(key.NameLeftArrow)},
		{0, modified(key.NameTab, key.ModShift)},
		{2, pressed(key.NameRightArrow)},
		{2, pressed(key.NameTab)},
	} {
		v := newPreviewView()
		v.active = tc.panel
		cancelled := false
		v.startInlineActions(tc.panel, 4, "", "", []string{"Overwrite", "Cancel"}, nil, func() { cancelled = true })
		send, _ := panelInputFrames(v)
		send(tc.event)
		if v.active != tc.panel || v.actions == nil || cancelled {
			t.Fatal("blocked panel switch cancelled the choice without leaving")
		}
	}
}

func TestCompactInlineActionsKeepSubjectAndChoicesVisible(t *testing.T) {
	for _, size := range []unit.Sp{15, 32, 48} {
		v := newPreviewView()
		v.appearance.TextSize = size
		v.startInlineActions(0, 4, "", "", []string{"Overwrite", "Cancel"}, nil, nil)
		v.actions.subjectRow = 3
		var router input.Router
		var ops op.Ops
		frame := func() {
			ops.Reset()
			v.layout(layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now(),
				Source: router.Source(), Constraints: layout.Exact(image.Pt(720, 360))})
			router.Frame(&ops)
		}
		frame()
		frame()
		assertVisible := func() {
			t.Helper()
			pos := v.panels[0].list.Position
			for row := v.actions.subjectRow; row < v.actions.row+v.actions.count(); row++ {
				if row < pos.First || row >= pos.First+pos.Count ||
					(row == pos.First && pos.Offset > 0) ||
					(row == pos.First+pos.Count-1 && pos.OffsetLast < 0) {
					t.Fatalf("font %v hid subject or action: row=%d position=%+v", size, row, pos)
				}
			}
		}
		assertVisible()
		for _, name := range []key.Name{key.NameHome, key.NameDownArrow, key.NameEnd, key.NameUpArrow, key.NameHome} {
			router.Queue(key.Event{Name: name, State: key.Press})
			frame()
			frame()
			assertVisible()
		}
		for _, delta := range []float32{1000, -1000} {
			router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(150, 220), Scroll: f32.Pt(0, delta)})
			frame()
			frame()
			assertVisible()
		}
	}
}

func TestClosingInlineActionsSelectsAndRevealsOffscreenSubject(t *testing.T) {
	for _, name := range []key.Name{key.NameReturn, key.NameEscape} {
		v := newPreviewView()
		send, _ := panelInputFrames(v)
		p := &v.panels[0]
		p.selected = 0
		subject := len(p.items) - 1
		v.startInlineActions(0, subject+1, "", "", []string{"Overwrite", "Cancel"}, nil, nil)
		v.actions.subjectRow = subject
		send()
		send(pressed(name))
		pos := p.list.Position
		if v.actions != nil || p.selected != subject || subject < pos.First || subject >= pos.First+pos.Count ||
			(subject == pos.First && pos.Offset > 0) ||
			(subject == pos.First+pos.Count-1 && pos.OffsetLast < 0) {
			t.Fatalf("%s did not select and reveal subject: selected=%d position=%+v", name, p.selected, pos)
		}
	}
}
