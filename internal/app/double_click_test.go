package app

import (
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/dannyben/commando/internal/browser"
)

func rowClick(send func(...event.Event), point f32.Point, at time.Duration, modifiers key.Modifiers) {
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point, Time: at, Modifiers: modifiers}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point, Time: at + time.Millisecond, Modifiers: modifiers})
}

func TestDoubleClickOpensFileAndPreservesMarks(t *testing.T) {
	v := selectionView()
	opened := make(chan string, 3)
	v.openFile = func(path string) error { opened <- path; return nil }
	v.panels[0].marks = map[int]bool{1: true, 3: true}
	send, _ := panelInputFrames(v)
	height := v.panels[0].rows[2].size.Y
	point := f32.Pt(20, float32(68+2*height+height/2))
	rowClick(send, point, time.Second, 0)
	if v.panels[0].selected != 2 || len(opened) != 0 {
		t.Fatal("single click failed to select or opened the file")
	}
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	select {
	case path := <-opened:
		if path != filepath.Join("/folder", "alpha beta.txt") {
			t.Fatal("wrong file opened", path)
		}
	case <-time.After(time.Second):
		t.Fatal("double click did not open file")
	}
	assertMarks(t, &v.panels[0], 1, 3)
	rowClick(send, point, time.Second+200*time.Millisecond, 0)
	if len(opened) != 0 {
		t.Fatal("triple click opened file again")
	}
}

func TestDoubleClickNavigatesFoldersParentsAndDrives(t *testing.T) {
	for _, tc := range []struct {
		name  string
		row   int
		drive bool
		want  string
	}{{"folder", 1, false, filepath.Join("/folder", "alpha")}, {"parent", 0, false, filepath.Dir("/folder")}, {"drive", 1, true, "/drive"}} {
		t.Run(tc.name, func(t *testing.T) {
			v := selectionView()
			v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
			if tc.drive {
				v.panels[0].drives = true
				v.panels[0].items[1].file = browser.Entry{Name: "Drive", Directory: true, Path: tc.want}
			}
			send, _ := panelInputFrames(v)
			height := v.panels[0].rows[tc.row].size.Y
			point := f32.Pt(20, float32(68+tc.row*height+height/2))
			rowClick(send, point, time.Second, 0)
			if len(v.loader.requests) != 0 {
				t.Fatal("single click navigated")
			}
			rowClick(send, point, time.Second+100*time.Millisecond, 0)
			select {
			case request := <-v.loader.requests:
				if request.panel != 0 || request.path != tc.want {
					t.Fatal("wrong navigation", request)
				}
			default:
				t.Fatal("double click did not navigate")
			}
		})
	}
}

func TestModifiedClicksDoNotOpenOrSeedDoubleClick(t *testing.T) {
	for _, modifier := range []key.Modifiers{key.ModCtrl, key.ModShift} {
		v := selectionView()
		v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
		send, _ := panelInputFrames(v)
		height := v.panels[0].rows[1].size.Y
		point := f32.Pt(20, float32(68+height+height/2))
		rowClick(send, point, time.Second, modifier)
		rowClick(send, point, time.Second+100*time.Millisecond, modifier)
		rowClick(send, point, time.Second+200*time.Millisecond, 0)
		if len(v.loader.requests) != 0 {
			t.Fatal("marking clicks or next single click opened folder")
		}
		if modifier == key.ModCtrl {
			assertMarks(t, &v.panels[0])
		} else {
			assertMarks(t, &v.panels[0], 1)
		}
		rowClick(send, point, time.Second+300*time.Millisecond, 0)
		if len(v.loader.requests) != 1 {
			t.Fatal("fresh unmodified double click did not open folder")
		}
	}
}

func TestFirstClickAfterDragDoesNotOpen(t *testing.T) {
	v := selectionView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
	send, _ := panelInputFrames(v)
	height := v.panels[0].rows[1].size.Y
	point := f32.Pt(20, float32(68+height+height/2))
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point.Add(f32.Pt(12, 0)), Time: time.Second + 20*time.Millisecond})
	send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point, Time: time.Second + 40*time.Millisecond})
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	if len(v.loader.requests) != 0 {
		t.Fatal("first click after drag opened entry")
	}
	rowClick(send, point, time.Second+200*time.Millisecond, 0)
	if len(v.loader.requests) != 1 {
		t.Fatalf("double click after drag did not open folder: clicks=%d suppressed=%v loading=%v current=%T", v.panels[0].rows[1].clicks, v.panels[0].rows[1].suppressClick, v.panels[0].loading, v.interaction.current)
	}
}
