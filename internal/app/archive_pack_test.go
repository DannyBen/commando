package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"gioui.org/io/key"
)

func TestPackZIPKeyboardPaletteDestinationsAndCompletion(t *testing.T) {
	for _, mode := range []string{"neighbor", "source", "single", "palette"} {
		t.Run(mode, func(t *testing.T) {
			v, source, destination := clipboardView(t)
			if mode == "single" {
				for len(v.panels) > 1 {
					v.active = len(v.panels) - 1
					v.closePanel()
				}
			}
			v.active = 0
			p := &v.panels[0]
			selectName(t, p, "a.txt")
			p.toggleMark(p.selected)
			selectName(t, p, "z.txt")
			p.toggleMark(p.selected)
			marks := slices.Clone(p.selectedNames())
			forbidTransferClipboard(t, v)
			send, _ := panelInputFrames(v)
			if mode == "palette" {
				send(pressed("`"))
				send(replacePalette(v, "Entry Pack to ZIP"))
				send(pressed(key.NameReturn))
			} else {
				send(modified(key.NameF5, key.ModAlt))
			}
			if v.inlineInteraction() == nil || !slices.Equal(v.inlineInteraction().options, []string{"Pack here"}) {
				t.Fatal("Pack did not use the destination floater")
			}
			target := 1
			if mode == "source" {
				send(pressed(key.NameLeftArrow))
			}
			if mode == "source" || mode == "single" {
				target, destination = 0, source
			}
			if v.active != target {
				t.Fatal("wrong Pack destination", v.active)
			}
			send(pressed(key.NameReturn))
			send()
			if r := v.nameInteraction(); r == nil || r.kind != namePack || v.nameEditor.Text() != filepath.Base(source)+".zip" {
				t.Fatal("Pack did not offer a suggested ZIP name")
			}
			// The editor selects only the basename, retaining the extension.
			start, end := v.nameEditor.Selection()
			if min(start, end) != 0 || max(start, end) != utf8.RuneCountInString(filepath.Base(source)) {
				t.Fatal("ZIP extension was selected")
			}
			send(replaceCreate(v, "bundle.zip"))
			send(pressed(key.NameReturn))
			waitCreated(t, v)
			send()
			if archiveMemberContents(t, filepath.Join(destination, "bundle.zip"), "a.txt") == "" {
				t.Fatal("ZIP lacks selected file")
			}
			if !slices.Equal(p.selectedNames(), marks) || v.active != target || v.transferPending || v.mutationsBusy() {
				t.Fatal("Pack disturbed marks, focus or operation state")
			}
			if v.panels[target].items[v.panels[target].selected].file.Name != "bundle.zip" {
				t.Fatal("new ZIP was not selected")
			}
			if message := v.panels[target].feedback.Current(); message == nil || message.Text != "ZIP created" {
				t.Fatal("wrong Pack completion feedback", message)
			}
			for _, name := range []string{"a.txt", "z.txt"} {
				if _, err := os.Stat(filepath.Join(source, name)); err != nil {
					t.Fatal("Pack removed a source", err)
				}
			}
		})
	}
}

func TestPackZIPNameCollisionRetryAndCancellation(t *testing.T) {
	v, source, destination := clipboardView(t)
	selectName(t, &v.panels[0], "a.txt")
	forbidTransferClipboard(t, v)
	send, _ := panelInputFrames(v)
	send(modified(key.NameF5, key.ModAlt))
	send(pressed(key.NameTab))
	send(pressed(key.NameEscape))
	if v.active != 0 || !v.interaction.idle() {
		t.Fatal("Pack targeting cancellation did not restore source focus")
	}
	send(modified(key.NameF5, key.ModAlt), pressed(key.NameReturn))
	send()
	if v.nameInteraction() == nil || v.nameEditor.Text() != "a.zip" {
		t.Fatal("single-file suggestion did not use basename")
	}
	if err := os.WriteFile(filepath.Join(destination, "a.zip"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	if r := v.nameInteraction(); r == nil || r.err == nil || r.pending || v.nameEditor.ReadOnly {
		t.Fatal("collision did not retain editable name")
	}
	if data, _ := os.ReadFile(filepath.Join(destination, "a.zip")); string(data) != "existing" {
		t.Fatal("Pack overwrote existing archive")
	}
	send(replaceCreate(v, "retry.zip"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if archiveMemberContents(t, filepath.Join(destination, "retry.zip"), "a.txt") == "" {
		t.Fatal("Pack name retry failed")
	}
	selectName(t, &v.panels[0], "a.txt")
	v.active = 0
	send(modified(key.NameF5, key.ModAlt), pressed(key.NameReturn))
	send()
	send(pressed(key.NameEscape))
	if !v.interaction.idle() || v.transferPending || v.mutationsBusy() {
		t.Fatal("Pack name cancellation retained session")
	}
	if _, err := os.Stat(filepath.Join(source, "a.txt")); err != nil {
		t.Fatal(err)
	}
}
