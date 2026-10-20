package app

import (
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/windowshell"
)

func TestRegularDragCarriesMarkedSetOrOnlyDraggedItem(t *testing.T) {
	for _, row := range []int{1, 2} {
		v := menuTestView()
		v.cutClipboard = browser.FileClipboard{Paths: []string{"existing-cut"}, Move: true, Sequence: 1}
		v.clipboardSequence = func() uint32 { return 1 }
		clipboard := v.cutClipboard
		want := v.panels[0].selectedPaths()
		if !v.panels[0].marks[row] {
			want = []string{filepath.Join(v.panels[0].path, v.panels[0].items[row].file.Name)}
		}
		marks := v.panels[0].marks
		send, _ := panelInputFrames(v)
		height := v.panels[0].rows[row].size.Y
		point := f32.Pt(20, float32(68+row*height+height/2))
		send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point})
		if !v.interaction.idle() {
			t.Fatal("mouse press alone started a drag")
		}
		send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point.Add(f32.Pt(2, 0))})
		if !v.interaction.idle() {
			t.Fatal("minor mouse jitter started a drag")
		}
		send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point.Add(f32.Pt(12, 0))})
		s, ok := v.interaction.current.(*nativeSession)
		if !ok || s.request.action != shellDrag || !reflect.DeepEqual(s.request.paths, want) {
			t.Fatalf("row %d: ordinary drag captured wrong files or did not start: %T", row, v.interaction.current)
		}
		if v.panels[0].selected != row || !reflect.DeepEqual(marks, v.panels[0].marks) {
			t.Fatal("drag lost current row or independent marks")
		}
		if v.panels[0].feedback.Current() != nil {
			t.Fatal("regular drag started a footer animation")
		}
		if !reflect.DeepEqual(clipboard, v.cutClipboard) {
			t.Fatal("drag changed the clipboard")
		}
		v.dispatchNative(func() {})
		v.completeNative(<-v.shellResults)
		if !v.interaction.idle() || !reflect.DeepEqual(clipboard, v.cutClipboard) {
			t.Fatal("drag completion retained foreground or changed clipboard")
		}
	}
}

func TestDragUsesSharedOwnerPolicyAndRetainsFilterAfterResult(t *testing.T) {
	v := menuTestView()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops}
	owner := &testForeground{panel: 0}
	v.interaction.begin(owner)
	v.startDrag(gtx, 0, 1)
	if v.interaction.current != owner {
		t.Fatal("drag replaced unknown foreground owner")
	}
	v.interaction.finish(owner)
	v.panels[0].setFilter("go")
	v.panels[0].selected = 1
	v.startDrag(gtx, 0, 1)
	s := v.interaction.current.(*nativeSession)
	selected, position := v.panels[0].selected, v.panels[0].list.Position
	v.completeNative(shellCompletion{session: s, result: windowshell.Result{Effect: 1}})
	if v.panels[0].filter != "go" || v.panels[0].selected != selected || v.panels[0].list.Position != position || v.panels[0].feedback.Current().Text != "Drop accepted" {
		t.Fatal("drop disturbed browsing state or lost result")
	}
}

func TestDragReleaseDoesNotBecomeClickAndNextClickWorks(t *testing.T) {
	v := menuTestView()
	send, _ := panelInputFrames(v)
	height := v.panels[0].rows[1].size.Y
	point := f32.Pt(20, float32(68+height+height/2))
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point})
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point.Add(f32.Pt(12, 0))})
	s := v.interaction.current.(*nativeSession)
	v.completeNative(shellCompletion{session: s, result: windowshell.Result{Cancelled: true}})
	v.panels[0].selected = 2
	send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	if v.panels[0].selected != 2 {
		t.Fatal("drag release became an ordinary click")
	}
	point = f32.Pt(20, float32(68+3*height+height/2))
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	if v.panels[0].selected != 3 {
		t.Fatal("drag suppression swallowed the next ordinary click")
	}
}
