package app

import (
	"maps"
	"strings"
	"testing"

	"gioui.org/io/key"
)

func TestCopyFilenamesCurrentMarksAndPalette(t *testing.T) {
	for _, marked := range []bool{false, true} {
		for _, palette := range []bool{false, true} {
			v := newPreviewView()
			p := &v.panels[0]
			p.clearMarks()
			p.selected = 2
			if marked {
				p.toggleMark(1)
				p.toggleMark(4)
			}
			want := strings.Join(p.selectedNames(), "\n")
			marks, selected := maps.Clone(p.marks), p.selected
			router, send := clipboardFrames(v)
			position := p.list.Position
			if palette {
				send(pressed("`"))
				send(replacePalette(v, "Entry Copy filenames"))
				if result := v.paletteInteraction().results[0].command; result.id != "entry.copy_filenames" || !result.allowed(v) {
					t.Fatal("Copy filenames missing or unavailable in palette")
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
				t.Fatal("copying filenames disturbed browsing state")
			}
			message := p.feedback.Current()
			if message == nil || message.Text != map[bool]string{false: "Filename copied", true: "2 filenames copied"}[marked] {
				t.Fatal("incorrect copy filenames feedback", message)
			}
		}
	}
}

func TestCopyFilenamesUnavailableWithoutEntryOrDuringInteraction(t *testing.T) {
	for _, mode := range []string{"parent", "loading", "busy", "editor"} {
		v := newPreviewView()
		p := &v.panels[0]
		p.clearMarks()
		p.selected = 2
		router, send := clipboardFrames(v)
		switch mode {
		case "parent":
			p.selected = 0
		case "loading":
			p.loading = true
		case "busy":
			v.transferPending = true
		case "editor":
			send(modified("D", key.ModAlt))
		}
		send(modified("C", key.ModCtrl|key.ModShift))
		if _, _, ok := router.WriteClipboard(); ok {
			t.Fatalf("%s copied filenames", mode)
		}
	}
}
