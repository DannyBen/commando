package app

import (
	"context"
	"errors"
	"image"
	"io/fs"
	"path/filepath"
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
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestCalculateSizeProgressRequestsItsOwnFrame(t *testing.T) {
	v := newPreviewView()
	v.helpHidden = true // No unrelated animation should wake the window.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v.interaction.begin(&sizeSession{panel: 0, ctx: ctx, cancel: cancel})
	start := time.Now()
	deadline := start.Add(v.appearance.FeedbackProgressDelay)
	v.panels[0].feedback.QuietProgressAt("Calculating size…", deadline)
	var router input.Router
	var ops op.Ops
	frame := func(now time.Time) {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))})
		router.Frame(&ops)
	}
	frame(start)
	frame(start) // Settle initial focus and widget events.
	if v.panels[0].feedback.Current() != nil {
		t.Fatal("progress appeared before its delay")
	}
	wake, ok := router.WakeupTime()
	if !ok || !wake.Equal(deadline) {
		t.Fatalf("progress redraw = %v, %v; want %v", wake, ok, deadline)
	}
	frame(wake)
	if m := v.panels[0].feedback.Current(); m == nil || m.Text != "Calculating size…" {
		t.Fatal("scheduled frame did not reveal progress")
	}
}

func TestCalculateSizePaletteAndBoundShortcut(t *testing.T) {
	for _, route := range []string{"palette", "shortcut"} {
		t.Run(route, func(t *testing.T) {
			v := selectionView()
			v.loader = newDirectoryLoader(nil, nil)
			t.Cleanup(v.finishOperations)
			started, release := make(chan []string, 1), make(chan struct{})
			v.loader.calculateSize = func(ctx context.Context, paths []string) (browser.SizeResult, error) {
				started <- paths
				select {
				case <-release:
					return browser.SizeResult{Files: 1234, Bytes: 23 * 1024 * 1024}, nil
				case <-ctx.Done():
					return browser.SizeResult{}, ctx.Err()
				}
			}
			p := &v.panels[0]
			if route == "palette" {
				p.toggleMark(1)
				p.toggleMark(2)
			}
			setTestShortcut(t, v, "entry.calculate_size", 0, "Ctrl+F9")
			send, advance := panelInputFrames(v)
			if route == "palette" {
				send(pressed("`"))
				send(replacePalette(v, "Entry Calculate size"))
				s := v.paletteInteraction()
				if len(s.results) != 1 || s.results[0].command.id != "entry.calculate_size" {
					t.Fatal("command missing from palette")
				}
				send(pressed(key.NameReturn))
			} else {
				send(modified(key.NameF9, key.ModCtrl))
			}
			select {
			case paths := <-started:
				want := []string{filepath.Join(p.path, "alpha")}
				if route == "palette" {
					want = append(want, filepath.Join(p.path, "alpha beta.txt"))
				}
				if !slices.Equal(paths, want) {
					t.Fatalf("targets=%v, want %v", paths, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not start")
			}
			owner := v.interaction.current
			if id, _ := defaultHelp.resolve(v.helpContextIDs()...); id != "calculating_size" {
				t.Fatal("calculation cancellation help missing")
			}
			selected := p.selected
			send(pressed(key.NameTab), pressed(key.NameDownArrow), modified("W", key.ModCtrl))
			send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(330, 110)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(330, 110)})
			if v.active != 0 || p.selected != selected || v.interaction.current != owner || len(v.panels) != 2 || v.actionAllowed(actionMutation) {
				t.Fatal("calculation did not retain foreground ownership")
			}
			advance(v.appearance.FeedbackProgressDelay + time.Second)
			send(pressed(key.NameDownArrow))
			if m := p.feedback.Current(); m == nil || m.Text != "Calculating size…" {
				t.Fatal("progress missing after ignored input")
			}
			close(release)
			select {
			case r := <-v.loader.results:
				v.acceptLoad(r)
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not finish")
			}
			if !v.interaction.idle() || p.feedback.Current() == nil || p.feedback.Current().Text != "1,234 files · 23.0 MB" {
				t.Fatal("calculation result missing")
			}
			if route == "palette" && len(p.marks) != 2 {
				t.Fatal("calculation changed marks")
			}
			send(pressed(key.NameDownArrow))
			if p.feedback.Current() != nil || p.selected == selected {
				t.Fatal("result did not dismiss on resumed browsing")
			}
		})
	}
}

func TestCalculateSizeCancelAndClose(t *testing.T) {
	for _, route := range []string{"escape", "close", "shutdown"} {
		t.Run(route, func(t *testing.T) {
			v := selectionView()
			v.loader = newDirectoryLoader(nil, nil)
			t.Cleanup(v.finishOperations)
			started, release := make(chan struct{}), make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			v.loader.calculateSize = func(ctx context.Context, _ []string) (browser.SizeResult, error) {
				close(started)
				<-ctx.Done()
				<-release
				return browser.SizeResult{Files: 999, Bytes: 999}, nil // Late success must still lose to cancellation.
			}
			p := &v.panels[0]
			p.toggleMark(1)
			p.setFilter("alpha")
			setTestShortcut(t, v, "entry.calculate_size", 0, "Ctrl+F9")
			send, _ := panelInputFrames(v)
			send(modified(key.NameF9, key.ModCtrl))
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("not started")
			}
			s := v.interaction.current.(*sizeSession)
			if route == "escape" {
				send(pressed(key.NameEscape), pressed(key.NameEscape))
				if v.interaction.current != s || len(p.marks) != 1 || p.filter != "alpha" {
					t.Fatal("Escape cleared browsing state or released worker ownership early")
				}
			} else if route == "close" {
				if !v.requestClose() {
					t.Fatal("read-only calculation blocked close")
				}
			} else {
				close(release)
				v.finishOperations()
				if s.ctx.Err() == nil || !v.interaction.idle() {
					t.Fatal("shutdown did not cancel and drain")
				}
				return
			}
			if s.ctx.Err() == nil {
				t.Fatal("worker was not cancelled")
			}
			close(release)
			select {
			case r := <-v.loader.results:
				v.acceptLoad(r)
			case <-time.After(5 * time.Second):
				t.Fatal("cancel did not finish")
			}
			if !v.interaction.idle() || p.feedback.Current() == nil || p.feedback.Current().Text != "Calculation cancelled" {
				t.Fatal("late success replaced cancellation")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			newer := &sizeSession{panel: 0, ctx: ctx, cancel: cancel}
			v.interaction.begin(newer)
			v.acceptCalculateSize(loadResult{request: loadRequest{size: s}, size: browser.SizeResult{Files: 999}})
			if v.interaction.current != newer || p.feedback.Current().Text != "Calculation cancelled" {
				t.Fatal("stale result changed newer session")
			}
			v.interaction.finish(newer)
		})
	}
}

func TestCalculateSizeAvailabilityAndErrors(t *testing.T) {
	v := selectionView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	p := &v.panels[0]
	for _, setup := range []func(){func() { p.selected = 0 }, func() { p.drives = true }, func() { p.archiveFolder = "." }, func() { p.refreshing = true }} {
		*p = selectionView().panels[0]
		setup()
		if commandByID(v, "entry.calculate_size").allowed(v) {
			t.Fatal("command allowed unsupported source")
		}
	}
	for _, tc := range []struct {
		err  error
		want string
	}{{fs.ErrPermission, "Access denied"}, {fs.ErrNotExist, "Entry not found"}, {errors.New("failure"), "Cannot calculate size"}} {
		*p = selectionView().panels[0]
		var ops op.Ops
		v.startCalculateSize(layout.Context{Ops: &ops})
		r := <-v.loader.requests
		v.acceptLoad(loadResult{request: r, err: tc.err, size: browser.SizeResult{Files: 123}})
		if m := p.feedback.Current(); m == nil || m.Text != tc.want || m.Kind != feedback.Error {
			t.Fatal("error displayed a partial total")
		}
	}
	for _, tc := range []struct {
		files int64
		want  string
	}{{0, "0 files · 0 B"}, {1, "1 file · 0 B"}, {1234567, "1,234,567 files · 0 B"}} {
		if got := sizeResultLabel(browser.SizeResult{Files: tc.files}); got != tc.want {
			t.Fatalf("result=%q", got)
		}
	}
}
