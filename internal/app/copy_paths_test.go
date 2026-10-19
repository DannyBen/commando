package app

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestArchiveCopyPathsAndLocationRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		folder, current string
		marks           []string
		palette         bool
	}{
		{".", "a2.txt", nil, false},
		{".", "a10.txt", []string{"docs", "a2.txt"}, true},
		{"docs", "sub", nil, false},
		{"docs/sub", "report.txt", nil, true},
	} {
		t.Run(tc.folder+"/"+tc.current, func(t *testing.T) {
			v, archive := archiveView(t)
			v.loadLocation(0, browser.Location{Path: archive, Folder: tc.folder}, tc.current)
			waitLoaded(t, v, 0)
			p := &v.panels[0]
			for _, name := range tc.marks {
				selectName(t, p, name)
				p.toggleMark(p.selected)
			}
			selectName(t, p, tc.current)
			marks, selected := maps.Clone(p.marks), p.selected
			names := tc.marks
			if len(names) == 0 {
				names = []string{tc.current}
			}
			want := make([]string, len(names))
			for i, name := range names {
				want[i] = filepath.Join(archive, filepath.FromSlash(tc.folder), name)
			}
			router, send := clipboardFrames(v)
			if tc.palette {
				send(pressed("`"))
				send(replacePalette(v, "Entry Copy paths"))
				send(pressed(key.NameReturn))
			} else {
				send(modified("C", key.ModCtrl|key.ModShift))
			}
			mime, data, ok := router.WriteClipboard()
			if !ok || mime != "application/text" || string(data) != strings.Join(want, "\n") {
				t.Fatalf("clipboard=%q %q, want %q", mime, data, want)
			}
			if p.selected != selected || !maps.Equal(p.marks, marks) || !v.interaction.idle() {
				t.Fatal("Copy paths changed archive selection")
			}
			// Feed the actual copied text back through the location editor.
			for _, copied := range strings.Split(string(data), "\n") {
				send(modified("D", key.ModAlt))
				send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: copied})
				send(pressed(key.NameReturn))
				waitLoaded(t, v, 0)
				send()
				if p.path != archive || p.err != nil || v.locationEditing() {
					t.Fatal("copied archive path did not navigate", p.err)
				}
				if filepath.Base(copied) == "docs" || filepath.Base(copied) == "sub" {
					if p.location().String() != copied {
						t.Fatal("copied folder path opened the wrong location")
					}
				} else if p.archiveFolder != tc.folder || p.items[p.selected].file.Name != filepath.Base(copied) {
					t.Fatal("copied file path did not select the file")
				}
			}
		})
	}
}

func TestCopyPathsCurrentMarksAndPalette(t *testing.T) {
	for _, marked := range []bool{false, true} {
		for _, palette := range []bool{false, true} {
			v := newPreviewView()
			p := &v.panels[0]
			p.path = filepath.Join(t.TempDir(), "folder with spaces")
			p.clearMarks()
			p.selected = 2
			if marked {
				p.toggleMark(1)
				p.toggleMark(4)
			}
			want := strings.Join(p.selectedPaths(), "\n")
			marks := maps.Clone(p.marks)
			selected, position := p.selected, p.list.Position
			router, send := clipboardFrames(v)
			position = p.list.Position
			if palette {
				send(pressed("`"))
				send(replacePalette(v, "Entry Copy paths"))
				if result := v.paletteInteraction().results[0].command; result.name != "Copy paths" || !result.allowed(v) {
					t.Fatal("Copy paths missing or unavailable in palette")
				}
				send(pressed(key.NameReturn))
			} else {
				send(modified("C", key.ModCtrl|key.ModShift))
			}
			mime, data, ok := router.WriteClipboard()
			if !ok || mime != "application/text" || string(data) != want {
				t.Fatalf("marked=%v palette=%v: clipboard=%q %q, want %q", marked, palette, mime, data, want)
			}
			if p.selected != selected || !maps.Equal(p.marks, marks) || p.list.Position != position || v.active != 0 || !v.interaction.idle() {
				t.Fatal("copying paths disturbed browsing state")
			}
			message := p.feedback.Current()
			if message == nil || message.Text != map[bool]string{false: "Path copied", true: "2 paths copied"}[marked] {
				t.Fatal("incorrect copy paths feedback", message)
			}
		}
	}
}

func TestCopyPathsExcludedRowsBusyEditorsAndKeyRelease(t *testing.T) {
	for _, mode := range []string{"parent", "drives", "loading", "busy", "native session", "editor", "release"} {
		v := newPreviewView()
		p := &v.panels[0]
		p.clearMarks()
		p.selected = 2
		router, send := clipboardFrames(v)
		switch mode {
		case "parent":
			p.selected = 0
		case "drives":
			p.drives = true
		case "loading":
			p.loading = true
		case "busy":
			v.transferPending = true
		case "native session":
			v.interaction.begin(&nativeSession{panel: 0})
		case "editor":
			send(modified("D", key.ModAlt))
		case "release":
			send(key.Event{Name: "C", State: key.Release, Modifiers: key.ModCtrl | key.ModShift})
			if _, _, ok := router.WriteClipboard(); ok {
				t.Fatal("release copied paths")
			}
			continue
		}
		send(modified("C", key.ModCtrl|key.ModShift))
		if _, _, ok := router.WriteClipboard(); ok {
			t.Fatalf("%s copied paths", mode)
		}
	}
}

func TestCopyPathsReplacesCutAppearanceThroughClipboardSequence(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[0]
	p.clearMarks()
	p.selected = 2
	v.cutClipboard = browser.FileClipboard{Move: true, Sequence: 10, Paths: p.selectedPaths()}
	v.clipboardSequence = func() uint32 { return 10 }
	router, send := clipboardFrames(v)
	send(modified("C", key.ModCtrl|key.ModShift))
	if _, _, ok := router.WriteClipboard(); !ok {
		t.Fatal("paths did not replace the clipboard")
	}
	v.clipboardSequence = func() uint32 { return 11 }
	send()
	if len(v.cutClipboard.Paths) != 0 {
		t.Fatal("replaced clipboard retained stale cut appearance")
	}
}
