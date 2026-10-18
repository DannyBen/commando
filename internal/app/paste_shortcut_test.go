package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestPasteShortcutKeyboardAndPalettePreserveClipboard(t *testing.T) {
	for _, palette := range []bool{false, true} {
		for _, cut := range []bool{false, true} {
			v, source, _ := clipboardView(t)
			c := attachTestFileClipboard(v)
			// Same-folder paste must create shortcuts rather than duplicate or move.
			original := browser.FileClipboard{Paths: []string{filepath.Join(source, "a.txt"), filepath.Join(source, "folder")}, Move: cut, Sequence: 10}
			c.data = original
			selectName(t, &v.panels[0], "z.txt")
			v.panels[0].toggleMark(v.panels[0].selected)
			selectName(t, &v.panels[2], "z.txt")
			v.loader.transfer = func(browser.TransferRequest) (browser.TransferResult, error) {
				t.Error("Paste shortcut invoked file transfer")
				return browser.TransferResult{}, nil
			}
			v.loader.createShortcuts = func(paths []string, directory string) (browser.TransferResult, error) {
				if !reflect.DeepEqual(paths, original.Paths) || directory != source {
					t.Error("Paste shortcut did not use clipboard sources and open folder")
				}
				item, err := browser.CreateFileData(directory, "shortcut.lnk", []byte("fixture"))
				return browser.TransferResult{Directory: directory, Attempted: true,
					Items: []browser.TransferredItem{{Source: paths[0], Entry: item}}, Summary: browser.TransferSummary{Requested: 2, Completed: 1}}, err
			}
			send, _ := panelInputFrames(v)
			if palette {
				send(pressed("`"), key.EditEvent{Text: "Entry Paste shortcut"})
				send(pressed(key.NameReturn))
			} else {
				send(modified("V", key.ModCtrl|key.ModShift))
			}
			waitLoaded(t, v, 0)
			if v.active != 0 || v.panels[0].items[v.panels[0].selected].file.Name != "shortcut.lnk" || v.panels[2].items[v.panels[2].selected].file.Name != "z.txt" {
				t.Fatal("shortcut selection or alias reconciliation changed panel focus/selection")
			}
			if len(v.panels[0].marks) != 1 {
				t.Fatal("Paste shortcut lost surviving marks")
			}
			for row := range v.panels[0].marks {
				if v.panels[0].items[row].file.Name != "z.txt" {
					t.Fatal("Paste shortcut moved a mark to another entry")
				}
			}
			if got, _ := c.read(); !reflect.DeepEqual(got, original) {
				t.Fatal("Paste shortcut changed the clipboard", got)
			}
			if _, err := os.Stat(original.Paths[0]); err != nil {
				t.Fatal("Paste shortcut removed its source", err)
			}
			if message := v.panels[0].feedback.Current(); message == nil || message.Text != "1 shortcut created" || v.transferPending {
				t.Fatal("shortcut completion did not finish cleanly", message)
			}
		}
	}
}

func TestPasteShortcutRejectsImageClipboardAndBusyDestination(t *testing.T) {
	v, _, _ := clipboardView(t)
	v.loader.readClipboard = func() (browser.FileClipboard, error) {
		return browser.FileClipboard{ImagePNG: []byte("image")}, nil
	}
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl|key.ModShift))
	waitLoaded(t, v, 0)
	if v.pendingNameDraft != nil || v.transferPending || v.panels[0].feedback.Current().Text != "Nothing to paste" {
		t.Fatal("Paste shortcut treated an image as a file selection")
	}
	v.transferPending = true
	send(modified("V", key.ModCtrl|key.ModShift))
	if v.panels[0].loading || v.loader.busy.Load() != 0 {
		t.Fatal("Paste shortcut bypassed the shared busy policy")
	}
}
