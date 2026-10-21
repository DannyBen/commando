package app

import (
	"image"
	"maps"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/windowshell"
)

type testForeground struct{ panel int }

func (s *testForeground) interactionPanel() int { return s.panel }

func menuTestView() *view {
	v := newPreviewView()
	v.shellExecutor = func(shellRequest) shellCompletion {
		return shellCompletion{result: windowshell.Result{Cancelled: true}}
	}
	return v
}

func TestContextMenuUsesSharedForegroundAndBusyPolicy(t *testing.T) {
	v := menuTestView()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops}
	owner := &testForeground{panel: 0}
	v.interaction.begin(owner)
	v.startContextMenu(gtx, 0, 1, true)
	if v.interaction.current != owner {
		t.Fatal("menu replaced an unknown foreground owner")
	}
	v.interaction.finish(owner)
	for _, blocked := range []func(){
		func() { v.transferPending = true },
		func() { v.panels[0].loading = true },
		func() { v.pendingNameDraft = &loadResult{} },
	} {
		blocked()
		v.startContextMenu(gtx, 0, 1, true)
		if !v.interaction.idle() {
			t.Fatal("menu ignored shared busy policy")
		}
		v.transferPending, v.panels[0].loading, v.pendingNameDraft = false, false, nil
	}
	v.startContextMenu(gtx, 0, 1, true)
	if v.interaction.idle() || v.actionAllowed(actionMutation) || v.actionAllowed(actionClipboard) || v.requestClose() {
		t.Fatal("native foreground did not hold competing operations and close requests")
	}
	if v.cancelInteraction(gtx) {
		t.Fatal("app cancellation ended a native-owned interaction")
	}
	before := v.panels[0].selected
	v.browseKey(gtx, pressed(key.NameDownArrow))
	if v.panels[0].selected != before {
		t.Fatal("browsing leaked into native interaction")
	}
}

func TestContextMenuKeyboardAndPaletteUseMarkedSnapshot(t *testing.T) {
	for _, palette := range []bool{false, true} {
		v := menuTestView()
		want := v.panels[0].selectedPaths()
		send, _ := panelInputFrames(v)
		if palette {
			send(pressed("`"))
			send(replacePalette(v, "Context menu"), pressed(key.NameReturn))
		} else {
			send(modified("/", key.ModAlt))
		}
		s, ok := v.interaction.current.(*nativeSession)
		if !ok || !reflect.DeepEqual(s.request.paths, want) || s.request.options.ClientPoint == nil {
			t.Fatalf("palette=%v: menu did not capture marks and keyboard anchor", palette)
		}
		v.panels[0].clearMarks()
		if !reflect.DeepEqual(s.request.paths, want) {
			t.Fatal("live selection changed menu snapshot")
		}
		v.dispatchNative(func() {})
		completion := <-v.shellResults
		v.completeNative(completion)
		if !v.interaction.idle() {
			t.Fatal("menu completion did not release foreground")
		}
	}
}

func TestContextMenuRightClickUsesMarksOnlyWhenClickedItemIsMarked(t *testing.T) {
	for _, marked := range []bool{false, true} {
		v := menuTestView()
		p := &v.panels[1]
		p.selected, p.marks = 3, map[int]bool{2: true, 3: true}
		if marked {
			p.marks[1] = true
		}
		marks := maps.Clone(p.marks)
		want := []string{filepath.Join(p.path, p.items[1].file.Name)}
		if marked {
			want = p.selectedPaths()
		}
		send, _ := panelInputFrames(v)
		point := f32.Pt(285, 110) // Second panel, first child after the parent row.
		send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: point},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
		s, ok := v.interaction.current.(*nativeSession)
		if !ok || v.active != 1 || p.selected != 1 {
			t.Fatalf("marked=%v: right click did not select its row: active=%d selected=%d owner=%T", marked, v.active, p.selected, v.interaction.current)
		}
		if !reflect.DeepEqual(s.request.paths, want) || s.request.options.ClientPoint != nil || !maps.Equal(p.marks, marks) {
			t.Fatalf("marked=%v: wrong menu selection or disturbed marks: got=%v want=%v", marked, s.request.paths, want)
		}
		p.clearMarks()
		if !reflect.DeepEqual(s.request.paths, want) {
			t.Fatal("live marks changed the captured mouse selection")
		}
	}
}

func TestContextMenuCompletionRefreshesQuietlyAndRejectsStaleOwner(t *testing.T) {
	v := menuTestView()
	var ops op.Ops
	v.panels[0].setFilter("go")
	v.startContextMenu(layout.Context{Ops: &ops}, 0, 1, true)
	s := v.interaction.current.(*nativeSession)
	before := v.panels[0].list.Position
	selected := v.panels[0].selected
	if !v.completeNative(shellCompletion{session: s}) {
		t.Fatal("completion rejected owner")
	}
	if !v.panels[0].refreshNeeded || !v.panels[1].refreshNeeded || v.panels[0].filter != "go" || v.panels[0].list.Position != before || v.panels[0].selected != selected {
		t.Fatal("shell completion lost browsing state or missed refresh")
	}
	newOwner := &testForeground{panel: 1}
	v.interaction.begin(newOwner)
	if v.completeNative(shellCompletion{session: s}) || v.interaction.current != newOwner {
		t.Fatal("stale native completion ended a newer interaction")
	}
}

func TestContextMenuAnchorStaysByCurrentPanel(t *testing.T) {
	v := menuTestView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameRightArrow), modified("/", key.ModAlt))
	anchor := v.panels[1].selectionAnchor
	if anchor.X < 267 || anchor.X > 533 || anchor.Y < 68 || anchor.Y > 450 || anchor == (image.Point{}) {
		t.Fatalf("keyboard menu anchor outside current panel listing: %v", anchor)
	}
}

func TestContextMenuNativeKeyUsesSameHandler(t *testing.T) {
	v := menuTestView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	send, _ := panelInputFrames(v)
	v.nativeKeys.push(pressed(contextMenuKey))
	send()
	if _, ok := v.interaction.current.(*nativeSession); !ok {
		t.Fatal("native menu key did not enter the shared menu handler")
	}
}

func TestContextMenuDragOutsideRowDoesNotOpenMenu(t *testing.T) {
	v := menuTestView()
	send, _ := panelInputFrames(v)
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: f32.Pt(20, 110)},
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: f32.Pt(20, 200)},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(20, 200)})
	if !v.interaction.idle() {
		t.Fatal("releasing outside the row opened its menu")
	}
}

func TestBackgroundContextMenuPreservesPanelState(t *testing.T) {
	for _, empty := range []bool{false, true} {
		v := selectionView()
		v.shellExecutor = menuTestView().shellExecutor
		p := &v.panels[1]
		if empty {
			p.items, p.selected = nil, -1
		}
		send, _ := panelInputFrames(v)
		selected, marks, position := p.selected, maps.Clone(p.marks), p.list.Position
		point := f32.Pt(450, 350)
		send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: point})
		if !v.interaction.idle() {
			t.Fatal("background menu opened before release")
		}
		send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
		s, ok := v.interaction.current.(*nativeSession)
		if !ok || s.panel != 1 || v.active != 1 || !s.request.options.Background || s.request.options.ClientPoint != nil || !reflect.DeepEqual(s.request.paths, []string{p.path}) {
			t.Fatalf("empty=%v: background click did not target its folder: owner=%#v", empty, v.interaction.current)
		}
		if p.selected != selected || !maps.Equal(p.marks, marks) || p.list.Position.First != position.First || p.list.Position.Offset != position.Offset {
			t.Fatal("background menu changed selection, marks or viewport")
		}
		if v.requestClose() || v.actionAllowed(actionMutation) {
			t.Fatal("background menu did not hold shared native ownership")
		}
		v.dispatchNative(func() {})
		v.completeNative(<-v.shellResults)
		if !v.interaction.idle() {
			t.Fatal("background completion retained ownership")
		}
	}
}

func TestBackgroundContextMenuRejectsUnavailablePanels(t *testing.T) {
	for _, block := range []func(*view){
		func(v *view) { v.shellExecutor = nil },
		func(v *view) { v.panels[1].drives = true },
		func(v *view) { v.panels[1].archiveFolder = "nested" },
		func(v *view) { v.panels[1].path = "" },
		func(v *view) { v.panels[1].loading = true },
		func(v *view) { v.transferPending = true },
		func(v *view) { v.pendingNameDraft = &loadResult{} },
	} {
		v := menuTestView()
		block(v)
		var ops op.Ops
		v.startBackgroundContextMenu(layout.Context{Ops: &ops}, 1)
		if !v.interaction.idle() || v.active != 0 {
			t.Fatal("unavailable background menu changed foreground or panel")
		}
	}
}

func TestBackgroundContextMenuReleaseOutsideDoesNotOpen(t *testing.T) {
	v := selectionView()
	v.shellExecutor = menuTestView().shellExecutor
	send, _ := panelInputFrames(v)
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: f32.Pt(450, 350)},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(20, 110)})
	if !v.interaction.idle() {
		t.Fatal("release outside the empty space opened a menu")
	}
}
