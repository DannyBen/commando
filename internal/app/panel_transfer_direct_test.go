package app

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestDirectPanelTransfersUseNeighborAndRetainSource(t *testing.T) {
	for _, direction := range []int{-1, 1} {
		for _, move := range []bool{false, true} {
			for _, marked := range []bool{false, true} {
				v, root, target := clipboardView(t)
				targetIndex := 1 + direction
				v.load(1, root, "")
				v.load(targetIndex, target, "")
				waitLoaded(t, v, 1)
				waitLoaded(t, v, targetIndex)
				v.active = 1
				selectName(t, &v.panels[1], "a.txt")
				want := []string{filepath.Join(root, "a.txt")}
				if marked {
					markClipboardItems(t, &v.panels[1])
					want = []string{filepath.Join(root, "folder"), filepath.Join(root, "a.txt")}
					selectName(t, &v.panels[1], "z.txt")
				}
				if err := os.Mkdir(filepath.Join(target, "highlighted"), 0700); err != nil {
					t.Fatal(err)
				}
				v.load(targetIndex, target, "")
				waitLoaded(t, v, targetIndex)
				selectName(t, &v.panels[targetIndex], "highlighted")
				c := attachTestFileClipboard(v)
				before, _ := c.write([]string{filepath.Join(root, "z.txt")}, true)
				v.rememberCutClipboard(before, v.fileOperationLocations(), v.fileOperationLocations())
				forbidTransferClipboard(t, v)
				requests := make(chan browser.TransferRequest, 3)
				v.ownerWindow = 123
				v.loader.transfer = func(r browser.TransferRequest) (browser.TransferResult, error) {
					requests <- r
					return fixtureTransfer(r)
				}
				send, _ := panelInputFrames(v)
				symbol := key.Name("[")
				if direction > 0 {
					symbol = "]"
				}
				if move {
					brace := "{"
					if symbol == "]" {
						brace = "}"
					}
					send(modified(symbol, key.ModShift), key.EditEvent{Text: brace})
				} else {
					send(pressed(symbol), key.EditEvent{Text: string(symbol)})
				}
				send(key.Event{Name: symbol, State: key.Release})
				if v.active != 1 || v.inlineInteraction() != nil || v.panels[1].prefix != "" {
					t.Fatal("direct shortcut changed focus, started targeting or became filename input")
				}
				waitLoaded(t, v, 1)
				send()
				if len(requests) != 1 {
					t.Fatalf("shortcut dispatched %d transfers", len(requests))
				}
				request := <-requests
				if !slices.Equal(request.Paths, want) || request.Directory != target || request.Move != move || request.Owner != 123 {
					t.Fatalf("wrong transfer: %+v", request)
				}
				assertBatchFile(t, filepath.Join(target, "a.txt"), "alpha")
				if marked {
					assertBatchFile(t, filepath.Join(target, "folder", "nested", "file"), "nested")
				}
				current, _ := c.read()
				if !reflect.DeepEqual(current, before) || !reflect.DeepEqual(v.cutClipboard, before) || v.active != 1 || v.transferPending {
					t.Fatal("completion changed clipboard, focus, or retained pending state")
				}
				if v.panels[targetIndex].items[v.panels[targetIndex].selected].file.Name != "highlighted" {
					t.Fatal("destination selection changed")
				}
				if !move {
					assertBatchFile(t, filepath.Join(root, "a.txt"), "alpha")
				} else if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
					t.Fatal("move left the source behind")
				}
			}
		}
	}
}

func TestDirectPanelTransfersIgnoreUnavailableTargetsAndEditors(t *testing.T) {
	for _, state := range []string{"left edge", "right edge", "destination loading", "destination drives", "source loading", "source drives", "parent", "busy", "name", "location", "filter"} {
		t.Run(state, func(t *testing.T) {
			v := newPreviewView()
			v.loader = &directoryLoader{requests: make(chan loadRequest, 4), results: make(chan loadResult, 4)}
			v.active = 1
			v.panels[1].clearMarks()
			v.panels[1].selected = 1
			send, _ := panelInputFrames(v)
			symbol := key.Name("[")
			switch state {
			case "left edge":
				v.active = 0
			case "right edge":
				v.active = 2
				symbol = "]"
			case "destination loading":
				v.panels[0].loading = true
			case "destination drives":
				v.panels[0].drives = true
			case "source loading":
				v.panels[1].loading = true
			case "source drives":
				v.panels[1].drives = true
			case "parent":
				v.panels[1].selected = 0
			case "busy":
				v.loader.busy.Store(1)
			case "name":
				send(pressed(key.NameF2))
			case "location":
				send(modified("D", key.ModAlt))
			case "filter":
				send(key.EditEvent{Text: "/"})
			}
			before, count := v.active, len(v.panels)
			editor := v.activeEditor()
			if editor != nil {
				editor.SetText("")
				editor.SetCaret(0, 0)
			}
			send(pressed(symbol), key.EditEvent{Text: string(symbol)})
			brace := "{"
			if symbol == "]" {
				brace = "}"
			}
			send(modified(symbol, key.ModShift), key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: brace})
			for len(v.loader.requests) > 0 {
				if request := <-v.loader.requests; request.transfer != nil {
					t.Fatal("unavailable shortcut dispatched transfer")
				}
			}
			if v.active != before || len(v.panels) != count || v.inlineInteraction() != nil || v.transferPending {
				t.Fatal("unavailable shortcut changed panel state")
			}
			if editor != nil && (v.activeEditor() != editor || editor.Text() != string(symbol)+brace) {
				t.Fatal("bracket stopped being ordinary editor text")
			}
			if editor == nil && v.panels[v.active].prefix != "" {
				t.Fatal("unavailable bracket became filename input")
			}
		})
	}
}

func TestDirectTransferPaletteAndConfigurableBindings(t *testing.T) {
	v, root, target := clipboardView(t)
	selectName(t, &v.panels[0], "a.txt")
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "Entry Copy to right"})
	s := v.paletteInteraction()
	if len(s.results) == 0 || s.results[0].command.id != "entry.copy_right" {
		t.Fatal("direct transfer missing from palette")
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	assertBatchFile(t, filepath.Join(target, "a.txt"), "alpha")
	assertBatchFile(t, filepath.Join(root, "a.txt"), "alpha")
	if v.active != 0 || !v.interaction.idle() {
		t.Fatal("palette transfer did not retain source context")
	}
	catalogue := bindingSettingsCatalogue()
	for _, tc := range []struct{ id, binding string }{{"entry.copy_left", "Ctrl+Y"}, {"entry.copy_right", "["}, {"entry.move_left", "none"}} {
		if err := v.settingsValues.Set(catalogue, bindingSettingKey(tc.id, 0), settings.Value{Text: tc.binding}); err != nil {
			t.Fatalf("could not configure %s: %v", tc.id, err)
		}
	}
	if err := validateBindingOverrides(&v.settingsValues); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := settings.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	values, err := settings.Load(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(values, nil)
	v.load(1, root, "")
	waitLoaded(t, v, 1)
	send(key.EditEvent{Text: "["})
	waitLoaded(t, v, 0)
	send()
	if v.nameInteraction() == nil {
		t.Fatal("reassigned bare bracket did not execute the right-hand copy's same-folder duplicate flow")
	}
	send(pressed(key.NameEscape))
	send(modified("]", key.ModShift), key.EditEvent{Text: "}"})
	waitLoaded(t, v, 0)
	send()
	if v.active != 0 || v.transferPending || v.panels[0].feedback.Current() == nil || v.panels[0].feedback.Current().Text != "Already in this folder" {
		t.Fatal("same-folder direct move lost its no-op behavior or source context")
	}
}
