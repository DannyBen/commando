package shellprobe

import (
	"image"
	"io"
	"log"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
)

func inputView() (*view, func(...event.Event)) {
	v := newView("demo", []browser.Entry{{Name: "folder", Directory: true}, {Name: "alpha.txt"}, {Name: "beta.txt"}}, log.New(io.Discard, "", 0))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(760, 540))})
		router.Frame(&ops)
	}
	frame()
	frame()
	return v, func(events ...event.Event) { router.Queue(events...); frame(); frame() }
}

func TestGioDragThresholdAndBatch(t *testing.T) {
	v, send := inputView()
	press := func(y float32, mod key.Modifiers) {
		send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: f32.Pt(60, y), Buttons: pointer.ButtonPrimary, Modifiers: mod})
	}
	release := func(y float32) {
		send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, y)})
	}
	press(220, key.ModCtrl)
	release(220)
	press(260, key.ModCtrl)
	release(260)
	if len(v.files.marks) != 2 {
		t.Fatalf("Ctrl-click did not mark the two file rows: %v", v.files.marks)
	}
	press(220, 0)
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(63, 220), Buttons: pointer.ButtonPrimary})
	if v.pending != nil {
		t.Fatal("minor pointer movement started a drag")
	}
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(100, 220), Buttons: pointer.ButtonPrimary})
	if v.pending == nil || v.pending.kind != "drag" {
		t.Fatal("Gio drag did not queue a native operation")
	}
	want := []string{filepath.Join("demo", "source", "alpha.txt"), filepath.Join("demo", "source", "beta.txt")}
	if !reflect.DeepEqual(v.pending.paths, want) {
		t.Fatalf("drag subjects=%v want=%v", v.pending.paths, want)
	}
	release(220)
	if v.pending.kind != "drag" {
		t.Fatal("pointer release changed the pending operation")
	}
}

func TestGioRightClickAndCancellation(t *testing.T) {
	v, send := inputView()
	pos := f32.Pt(60, 220)
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: pos, Buttons: pointer.ButtonSecondary, Modifiers: key.ModShift})
	if v.pending != nil {
		t.Fatal("menu opened before right button release")
	}
	send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos, Modifiers: key.ModShift})
	if v.pending == nil || v.pending.kind != "menu" || !v.pending.extended {
		t.Fatal("right-click did not queue the extended shell menu")
	}
	if len(v.pending.paths) != 1 || filepath.Base(v.pending.paths[0]) != "alpha.txt" {
		t.Fatalf("wrong context subject: %v", v.pending.paths)
	}

	v, send = inputView()
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: pos, Buttons: pointer.ButtonPrimary})
	send(pointer.Event{Kind: pointer.Cancel, Source: pointer.Mouse, Position: pos})
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(120, 220), Buttons: pointer.ButtonPrimary})
	if v.pending != nil {
		t.Fatal("cancelled pointer started a native operation")
	}
}

func TestGioKeyboardMenuAndEmptySelection(t *testing.T) {
	v, send := inputView()
	send(key.Event{Name: "A", State: key.Press, Modifiers: key.ModCtrl})
	send(key.Event{Name: "F10", State: key.Press, Modifiers: key.ModShift})
	if v.pending == nil || v.pending.kind != "menu" || len(v.pending.paths) != 3 {
		t.Fatalf("keyboard menu did not preserve the marked batch: pending=%+v marks=%v focused=%t", v.pending, v.files.marks, v.focus)
	}
	v, send = inputView()
	v.files.replace(nil)
	send(key.Event{Name: key.NameSpace, State: key.Press})
	send(key.Event{Name: "F10", State: key.Press, Modifiers: key.ModShift})
	if v.pending != nil || len(v.files.marks) != 0 {
		t.Fatal("empty file list queued a shell operation")
	}
}
