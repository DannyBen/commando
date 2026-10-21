package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
)

func TestUnpackZIPFloaterChoicesNavigationAndCompletion(t *testing.T) {
	for _, mode := range []string{"here", "folder", "source", "single", "palette"} {
		t.Run(mode, func(t *testing.T) {
			source, target := t.TempDir(), t.TempDir()
			archive := filepath.Join(source, "bundle.zip")
			writePanelArchive(t, archive, "docs/report.txt", "loose.txt", "empty/")
			paths := []string{source, target, source}
			if mode == "single" {
				paths = []string{source}
			}
			v := newBrowserView(paths, nil)
			t.Cleanup(v.finishOperations)
			for pi := range paths {
				waitLoaded(t, v, pi)
			}
			p := &v.panels[0]
			selectName(t, p, "bundle.zip")
			p.toggleMark(p.selected)
			forbidTransferClipboard(t, v)
			send, _ := panelInputFrames(v)
			if mode == "palette" {
				send(pressed("`"))
				send(replacePalette(v, "Entry Unpack archive"))
				send(pressed(key.NameReturn))
			} else {
				send(modified(key.NameF6, key.ModAlt))
			}
			if a := v.inlineInteraction(); a == nil || !slices.Equal(a.options, []string{"Extract here", "Extract to folder"}) || a.selected != 0 {
				t.Fatal("Unpack did not offer both choices, defaulting to Extract here")
			}
			named := mode == "folder" || mode == "source"
			if named {
				send(pressed(key.NameDownArrow))
			}
			if mode == "folder" {
				// The chosen action follows the floater across panels and back.
				send(pressed(key.NameTab))
				send(modified(key.NameTab, key.ModShift))
				if v.active != 1 || v.inlineInteraction().selected != 1 {
					t.Fatal("panel navigation lost extraction choice")
				}
			}
			if mode == "source" {
				send(pressed(key.NameLeftArrow))
			}
			panel := 1
			if mode == "source" || mode == "single" {
				panel, target = 0, source
			}
			if directory := os.Getenv("COMMANDO_UNPACK_PREVIEW_DIR"); directory != "" && mode == "folder" {
				if err := renderViewPNG(filepath.Join(directory, "unpack-choices.png"), v); err != nil {
					t.Fatal(err)
				}
			}
			send(pressed(key.NameReturn))
			send()
			folder := ""
			if named {
				r := v.nameInteraction()
				if r == nil || r.kind != nameUnpack || v.nameEditor.Text() != "bundle" || v.nameEditor.SelectedText() != "bundle" {
					t.Fatal("Extract to folder did not become a selected folder name")
				}
				send(replaceCreate(v, "custom.folder"))
				send(pressed(key.NameReturn))
				waitCreated(t, v)
				send()
				folder = "custom.folder"
			} else {
				waitLoaded(t, v, panel)
				send()
			}
			for _, name := range []string{"docs/report.txt", "loose.txt"} {
				data, err := os.ReadFile(filepath.Join(target, folder, filepath.FromSlash(name)))
				if err != nil || string(data) != name {
					t.Fatal("wrong extraction destination or contents", name, err)
				}
			}
			if v.active != panel || !v.interaction.idle() || v.transferPending || v.mutationsBusy() || !slices.Equal(p.selectedNames(), []string{"bundle.zip"}) {
				t.Fatal("extraction changed marks/focus or retained operation state")
			}
			if named && v.panels[panel].items[v.panels[panel].selected].name != folder {
				t.Fatal("extracted folder was not selected")
			}
			if message := v.panels[panel].feedback.Current(); message == nil || message.Text != "Extracted" {
				t.Fatal("wrong extraction feedback", message)
			}
		})
	}
}

func TestUnpackZIPFolderCollisionRetryAndCancellation(t *testing.T) {
	v, archive := archiveView(t)
	v.parentFolder()
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], filepath.Base(archive))
	forbidTransferClipboard(t, v)
	send, _ := panelInputFrames(v)
	send(modified(key.NameF6, key.ModAlt), pressed(key.NameEscape))
	if !v.interaction.idle() || v.active != 0 {
		t.Fatal("unpack cancellation did not restore source")
	}
	send(modified(key.NameF6, key.ModAlt), pressed(key.NameDownArrow), pressed(key.NameReturn))
	send()
	if err := os.Mkdir(filepath.Join(v.panels[1].path, "sample"), 0700); err != nil {
		t.Fatal(err)
	}
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	if n := v.nameInteraction(); n == nil || n.err == nil || n.pending || v.nameEditor.ReadOnly {
		t.Fatal("folder collision did not retain editable name")
	}
	send(replaceCreate(v, "retry"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if _, err := os.Stat(filepath.Join(v.panels[1].path, "retry", "docs", "sub", "report.txt")); err != nil {
		t.Fatal("folder retry failed", err)
	}
	v.active = 0
	send(modified(key.NameF6, key.ModAlt), pressed(key.NameDownArrow), pressed(key.NameReturn))
	send()
	send(pressed(key.NameEscape))
	if !v.interaction.idle() || v.transferPending || v.mutationsBusy() {
		t.Fatal("folder draft cancellation retained operation state")
	}
}
