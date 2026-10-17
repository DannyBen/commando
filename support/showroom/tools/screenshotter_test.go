//go:build showroom

package app

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/gpu/headless"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
)

// Compiled into internal/app through an overlay so private UI models remain
// private. The showroom tag excludes this tool from the regular app test suite.
// Only the supplied virtual listings are used; no host directories are read.
type showroomRequest struct {
	Capture struct {
		Width     int       `json:"width"`
		Height    int       `json:"height"`
		Scale     int       `json:"scale"`
		Themes    []string  `json:"themes"`
		StartTime time.Time `json:"start_time"`
	} `json:"capture"`
	Panels []struct {
		Folder   string   `json:"folder"`
		Selected string   `json:"selected"`
		Marked   []string `json:"marked"`
	} `json:"panels"`
	ActivePanel int `json:"active_panel"`
	Folders     map[string][]struct {
		Name      string    `json:"name"`
		Directory bool      `json:"directory"`
		Bytes     int64     `json:"bytes"`
		Modified  time.Time `json:"modified"`
		Created   time.Time `json:"created"`
	} `json:"folders"`
	Theme  string `json:"theme"`
	Output string `json:"output"`
	Steps  []struct {
		File   string `json:"file"`
		Key    string `json:"key"`
		Text   string `json:"text"`
		WaitMS int    `json:"wait_ms"`
	} `json:"steps"`
}

func (r showroomRequest) view(t *testing.T) *view {
	t.Helper()
	if len(r.Panels) == 0 || r.ActivePanel < 1 || r.ActivePanel > len(r.Panels) {
		t.Fatal("invalid initial panels")
	}
	v := newView(make([]panel, len(r.Panels)))
	for i, initial := range r.Panels {
		files, ok := r.Folders[initial.Folder]
		if !ok {
			t.Fatalf("unknown folder: %s", initial.Folder)
		}
		entries := []browser.Entry{{Name: "..", Directory: true, Parent: true}}
		for _, f := range files {
			entries = append(entries, browser.Entry{Name: f.Name, Directory: f.Directory,
				Bytes: f.Bytes, SizeKnown: !f.Directory, Modified: f.Modified, Created: f.Created})
		}
		slices.SortFunc(entries, browser.CompareEntries)
		p := &v.panels[i]
		p.reconcileDirectoryListing(browser.Directory{Path: initial.Folder, Entries: entries})
		p.selected = -1
		for row, item := range p.items {
			if p.selected == -1 && !item.file.Parent || item.name == initial.Selected {
				p.selected = row
			}
			if slices.Contains(initial.Marked, item.name) {
				p.toggleMark(row)
			}
		}
		p.reveal = true
	}
	themeIndex := slices.Index([]string{"light", "dark"}, r.Theme)
	if themeIndex < 0 {
		t.Fatalf("unknown theme: %s", r.Theme)
	}
	v.restoreSettings(builtInThemes[themeIndex].values.Clone(), nil)
	v.active = r.ActivePanel - 1
	return v
}

func TestShowroomScreenshots(t *testing.T) {
	requestPath := os.Getenv("COMMANDO_SHOWROOM_REQUEST")
	if requestPath == "" {
		t.Skip("run support/showroom/showroom")
	}
	f, err := os.Open(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var request showroomRequest
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		t.Fatal(err)
	}
	size := image.Pt(request.Capture.Width, request.Capture.Height)
	if size.X < 320 || size.Y < 320 || size.X > 4096 || size.Y > 4096 {
		t.Fatal("capture dimensions must be between 320 and 4096")
	}
	scale := request.Capture.Scale
	if scale == 0 {
		scale = 1
	}
	if scale < 1 || scale > 3 {
		t.Fatal("capture scale must be between 1 and 3")
	}
	// Increase the framebuffer and Gio metrics together: more pixels, same layout.
	size = size.Mul(scale)
	t.Log("Preparing workspace and theme…")
	v := request.view(t)
	t.Log("Initializing headless graphics…")
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Fatal(err)
	}
	defer window.Release()
	var router input.Router
	var ops op.Ops
	now := request.Capture.StartTime
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}, Constraints: layout.Exact(size)})
		router.Frame(&ops)
		if err := window.Frame(&ops); err != nil {
			t.Fatal(err)
		}
	}
	send := func(events ...event.Event) { router.Queue(events...); frame(); frame() }
	t.Log("Rendering first frame (fonts and shaders)…")
	frame()
	frame()
	t.Log("Running scene…")
	seen := make(map[string]bool)
	for _, scene := range request.Steps {
		if name := scene.Key; name != "" {
			binding, err := parseBindingOverride(name)
			if err != nil || binding.Mode != bindingAssigned {
				t.Fatalf("invalid key %q: %v", name, err)
			}
			s := binding.Shortcut
			if text := s.textTrigger(); text != "" {
				send(key.EditEvent{Text: text})
			} else {
				send(key.Event{Name: s.Name, Modifiers: s.Modifiers, State: key.Press})
				send(key.Event{Name: s.Name, Modifiers: s.Modifiers, State: key.Release})
			}
		}
		if scene.Text != "" {
			send(showroomTextEvents(v, scene.Text)...)
		}
		if scene.WaitMS < 0 {
			t.Fatal("wait_ms cannot be negative")
		}
		now = now.Add(time.Duration(scene.WaitMS) * time.Millisecond)
		frame()
		frame()
		if scene.File == "" {
			continue
		}
		if filepath.Base(scene.File) != scene.File || !strings.HasSuffix(scene.File, ".png") || seen[scene.File] {
			t.Fatalf("screenshot needs a unique .png filename: %q", scene.File)
		}
		seen[scene.File] = true
		img := image.NewRGBA(image.Rectangle{Max: size})
		if err := window.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(request.Output, scene.File)
		output, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		encodeErr := png.Encode(output, img)
		closeErr := output.Close()
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		t.Logf("%s (%dx%d, %s, %d panels)", path, size.X, size.Y, request.Theme, len(v.panels))
	}
}

// Gio expects the input method to advance the selection after inserting text.
// Sending only EditEvent leaves the caret behind and reverses later type calls.
func showroomTextEvents(v *view, text string) []event.Event {
	selection := key.Range{}
	editor := v.activeEditor()
	if editor != nil {
		start, end := editor.Selection()
		selection = key.Range{Start: min(start, end), End: max(start, end)}
	}
	events := []event.Event{key.EditEvent{Range: selection, Text: text}}
	if editor != nil {
		caret := selection.Start + utf8.RuneCountInString(text)
		events = append(events, key.SelectionEvent{Start: caret, End: caret})
	}
	return events
}

func TestShowroomTextInputAppendsAndReplacesSelection(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("/"), key.EditEvent{Text: "/"})
	for _, text := range []string{"m", "p"} {
		send(showroomTextEvents(v, text)...)
	}
	if v.filterEditor.Text() != "mp" || v.panels[v.active].filter != "mp" {
		t.Fatalf("consecutive input reversed the filter: %q", v.filterEditor.Text())
	}
	v.filterEditor.SetCaret(0, 2)
	for _, text := range []string{"é", "té"} {
		send(showroomTextEvents(v, text)...)
	}
	start, end := v.filterEditor.Selection()
	if v.filterEditor.Text() != "été" || start != 3 || end != 3 {
		t.Fatalf("replacement/caret used incorrect rune offsets: %q, %d:%d", v.filterEditor.Text(), start, end)
	}
}
