package app

import (
	"image"
	"io"
	"strings"
	"testing"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func clipboardFrames(v *view) (*input.Router, func(...event.Event)) {
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))})
		router.Frame(&ops)
	}
	frame()
	frame()
	return router, func(events ...event.Event) { router.Queue(events...); frame(); frame() }
}

func clipboardData(text string) transfer.DataEvent {
	return transfer.DataEvent{Type: "application/text", Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader(text)) }}
}

func TestEditorCopyAndPaste(t *testing.T) {
	for _, editorName := range []string{"rename", "location", "create"} {
		for _, shortcut := range []string{"native Shift+Insert", "routed Shift+Insert", "Ctrl+V"} {
			t.Run(editorName+"/"+shortcut, func(t *testing.T) {
				v := newPreviewView()
				v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
				v.panels[0].selected = 12 // AGENTS.md
				router, send := clipboardFrames(v)
				start := func() {
					if editorName == "rename" {
						send(pressed(key.NameF2))
					} else if editorName == "location" {
						send(modified("D", key.ModAlt))
					} else {
						send(newFolderKey())
					}
				}
				start()
				editor := v.activeEditor()
				if editorName == "create" {
					send(replaceCreate(v, "copied folder"))
					editor.SetCaret(0, len("copied folder"))
				}
				selected := editor.SelectedText()
				send(modified("C", key.ModCtrl))
				mime, content, ok := router.WriteClipboard()
				if !ok || mime != "application/text" || string(content) != selected {
					t.Fatalf("copy: %q %q %v", mime, content, ok)
				}
				send(pressed(key.NameEscape))
				if editorName == "rename" {
					v.panels[0].selected = 14
				} // README.md
				start()
				editor = v.activeEditor()
				before := editor.Text()
				// Copy from one editor session, then paste into another.
				switch shortcut {
				case "native Shift+Insert":
					v.nativeKeys.push(modified(insertKey, key.ModShift))
					send()
				case "routed Shift+Insert":
					send(modified(insertKey, key.ModShift))
				case "Ctrl+V":
					send(modified("V", key.ModCtrl))
				}
				if !router.ClipboardRequested() {
					t.Fatal("shortcut did not request clipboard")
				}
				send(clipboardData(string(content)))
				want := string(content)
				if editorName == "rename" {
					want += ".md"
				}
				if editor.Text() != want || v.active != 0 || !router.Source().Focused(editor) {
					t.Fatalf("paste: got %q want %q (before %q)", editor.Text(), want, before)
				}
				if len(v.panels[0].marks) != 3 {
					t.Fatal("editor Insert changed file marks")
				}
				// Ctrl+Z must undo the paste through Gio's ordinary editor history.
				send(modified("Z", key.ModCtrl))
				if editor.Text() != before {
					t.Fatalf("paste undo: %q want %q", editor.Text(), before)
				}
			})
		}
	}
}

func TestNativeInsertPasteRequiresShiftAndWritableEditor(t *testing.T) {
	for _, name := range []string{"location", "rename", "pending rename"} {
		t.Run(name, func(t *testing.T) {
			v := newPreviewView()
			v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
			router, send := clipboardFrames(v)
			if name == "location" {
				send(modified("D", key.ModAlt))
			} else {
				send(pressed(key.NameF2))
			}
			if name == "pending rename" {
				v.nameInteraction().pending, v.nameEditor.ReadOnly = true, true
			}
			before := v.activeEditor().Text()
			keys := []key.Event{pressed(insertKey), modified(insertKey, key.ModCtrl), modified(insertKey, key.ModAlt|key.ModShift),
				{Name: insertKey, State: key.Release, Modifiers: key.ModShift}}
			if name == "pending rename" {
				keys = append(keys, modified(insertKey, key.ModShift))
			}
			for _, ke := range keys {
				v.nativeKeys.push(ke)
				send()
				if router.ClipboardRequested() || v.activeEditor().Text() != before {
					t.Fatal("ignored Insert requested paste or changed editor")
				}
			}
		})
	}
}
