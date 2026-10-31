package app

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
)

func forbidTransferClipboard(t *testing.T, v *view) {
	t.Helper()
	v.loader.readClipboard = func() (browser.FileClipboard, error) {
		t.Error("panel transfer read the file clipboard")
		return browser.FileClipboard{}, errors.New("unexpected clipboard read")
	}
	v.loader.writeClipboard = func([]string, bool) (browser.FileClipboard, error) {
		t.Error("panel transfer wrote the file clipboard")
		return browser.FileClipboard{}, errors.New("unexpected clipboard write")
	}
	v.loader.completeClipboard = func(browser.FileClipboard, []string) (browser.FileClipboard, error) {
		t.Error("panel transfer consumed the cut clipboard")
		return browser.FileClipboard{}, errors.New("unexpected clipboard completion")
	}
}

func TestDirectPanelTransferRevealsDestinationWithoutMarking(t *testing.T) {
	for _, direction := range []int{-1, 1} {
		for _, move := range []bool{false, true} {
			t.Run(fmt.Sprintf("direction=%d/move=%t", direction, move), func(t *testing.T) {
				v, source, target := clipboardView(t)
				files := make(map[string]string)
				for i := range 50 {
					files[fmt.Sprintf("old-%02d.txt", i)] = "old"
				}
				writeBatchFiles(t, target, files)
				sourcePanel, targetPanel := 0, 1
				if direction < 0 {
					sourcePanel, targetPanel = 1, 0
				}
				v.load(sourcePanel, source, "")
				waitLoaded(t, v, sourcePanel)
				v.load(targetPanel, target, "")
				waitLoaded(t, v, targetPanel)
				v.active = sourcePanel
				for _, name := range []string{"a.txt", "z.txt"} {
					selectName(t, &v.panels[sourcePanel], name)
					v.panels[sourcePanel].toggleMark(v.panels[sourcePanel].selected)
				}
				p := &v.panels[targetPanel]
				selectName(t, p, "old-00.txt")
				p.toggleMark(p.selected)
				v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
					// Simulate the final names supplied by shell conflict handling.
					request.Names = []string{"zz-2.txt", "zz-1.txt"}
					return fixtureTransfer(request)
				}
				forbidTransferClipboard(t, v)
				send, _ := panelInputFrames(v)
				shortcut, next := key.Name("]"), key.NameRightArrow
				if direction < 0 {
					shortcut, next = "[", key.NameLeftArrow
				}
				if move {
					brace := "}"
					if direction < 0 {
						brace = "{"
					}
					send(modified(shortcut, key.ModShift), key.EditEvent{Text: brace})
				} else {
					send(pressed(shortcut), key.EditEvent{Text: string(shortcut)})
				}
				waitLoaded(t, v, sourcePanel)
				send()
				if v.active != sourcePanel || p.items[p.selected].name != "zz-1.txt" || len(p.marks) != 0 {
					t.Fatal("transfer did not clear destination marks/select the first sorted result while retaining focus")
				}
				if p.list.Position.First == 0 || p.selected < p.list.Position.First || p.selected >= p.list.Position.First+p.list.Position.Count {
					t.Fatal("offscreen result was not brought into view", p.selected, p.list.Position)
				}
				send(pressed(next))
				if v.active != targetPanel || p.items[p.selected].name != "zz-1.txt" {
					t.Fatal("switching panels lost the transferred selection")
				}
				selectName(t, p, "old-00.txt")
				send(pressed(insertKey))
				if names, _ := p.removalSelection(); !slices.Equal(names, []string{"old-00.txt"}) {
					t.Fatal("later deletion included an unmarked transfer result", names)
				}
			})
		}
	}
}

func TestPanelTransferSelectionHonorsFilterAndIgnoresStaleOrFailedResults(t *testing.T) {
	for _, state := range []string{"filtered", "empty", "navigated", "refresh failed"} {
		t.Run(state, func(t *testing.T) {
			v := newPreviewView()
			p := &v.panels[1]
			target := &panelTransferTarget{panel: 1, location: p.location()}
			p.filter = "keep"
			p.reconcileDirectoryListing(browser.Directory{Path: p.path, Entries: []browser.Entry{{Name: "keep.txt"}, {Name: "hidden.txt"}}})
			selectName(t, p, "keep.txt")
			p.toggleMark(p.selected)
			result := loadResult{panelUpdates: make([]transferPanelUpdate, len(v.panels)), transferred: browser.TransferResult{Items: []browser.TransferredItem{{Entry: browser.Entry{Name: "hidden.txt"}}}}}
			switch state {
			case "empty":
				result.transferred.Items = nil
			case "navigated":
				target.location.Path += "elsewhere"
			case "refresh failed":
				result.panelUpdates[1].err = fs.ErrPermission
			}
			v.revealPanelTransfer(target, result)
			wantMarks := 1
			if state == "filtered" {
				wantMarks = 0
			}
			if p.filter != "keep" || p.items[p.selected].name != "keep.txt" || len(p.marks) != wantMarks {
				t.Fatal("completion changed the filter or applied an invalid batch selection")
			}
		})
	}
}

func TestPanelTransferDefaultDestinationNavigationAndCancellation(t *testing.T) {
	for _, shortcut := range []key.Name{key.NameF5, key.NameF6} {
		for source := range 3 {
			t.Run(string(shortcut)+string(rune('0'+source)), func(t *testing.T) {
				v, root, _ := clipboardView(t)
				v.load(1, root, "")
				waitLoaded(t, v, 1)
				v.active = source
				selectName(t, &v.panels[source], "a.txt")
				v.panels[source].toggleMark(v.panels[source].selected)
				send, _ := panelInputFrames(v)
				positions := make([]layout.Position, len(v.panels))
				selected := make([]int, len(v.panels))
				marks := make([]map[int]bool, len(v.panels))
				for pi := range v.panels {
					positions[pi], selected[pi] = v.panels[pi].list.Position, v.panels[pi].selected
					marks[pi] = maps.Clone(v.panels[pi].marks)
				}
				v.loader.transfer = func(browser.TransferRequest) (browser.TransferResult, error) {
					t.Error("targeting or cancellation performed a transfer")
					return browser.TransferResult{}, nil
				}
				forbidTransferClipboard(t, v)
				send(pressed(shortcut))
				if id, _ := defaultHelp.resolve(v.helpContextIDs()...); id != "actions.target_panel" {
					t.Fatal("panel transfer did not explain target-panel navigation", id)
				}
				initial := source + 1
				if source == 2 {
					initial = 1
				}
				verb := "Copy"
				if shortcut == key.NameF6 {
					verb = "Move"
				}
				if v.active != initial || v.inlineInteraction() == nil || len(v.inlineInteraction().headers) != 0 ||
					!slices.Equal(v.inlineInteraction().options, []string{verb + " here"}) || v.transferPending {
					t.Fatal("shortcut did not immediately offer the action in the default destination")
				}
				left, right := 0, 2
				if source == 0 {
					left = 1
				}
				if source == 2 {
					right = 1
				}
				send(pressed(key.NameRightArrow), pressed(key.NameRightArrow), pressed(key.NameRightArrow))
				if v.active != right || !slices.Equal(v.inlineInteraction().options, []string{verb + " here"}) {
					t.Fatal("right navigation did not skip source or stop at outer edge")
				}
				send(pressed(key.NameLeftArrow), pressed(key.NameLeftArrow), pressed(key.NameLeftArrow))
				if v.active != left || v.inlineInteraction().panel != left {
					t.Fatal("left navigation did not carry the action or skip source")
				}
				send(pressed(key.NameEscape))
				if v.inlineInteraction() != nil || v.active != source || v.transferPending {
					t.Fatal("Escape did not restore source focus")
				}
				for pi := range v.panels {
					p := &v.panels[pi]
					if p.selected != selected[pi] || p.list.Position != positions[pi] || !reflect.DeepEqual(p.marks, marks[pi]) {
						t.Fatalf("targeting changed pane %d selection, scroll or marks", pi)
					}
				}
			})
		}
	}
}

func TestPanelTransferCapturedSelectionAndOpenFolderDestination(t *testing.T) {
	for _, move := range []bool{false, true} {
		for _, marked := range []bool{false, true} {
			v, source, target := clipboardView(t)
			c := attachTestFileClipboard(v)
			before, _ := c.write([]string{filepath.Join(source, "z.txt")}, true)
			v.rememberCutClipboard(before, v.fileOperationLocations(), v.fileOperationLocations())
			forbidTransferClipboard(t, v)
			selectName(t, &v.panels[0], "a.txt")
			want := []string{filepath.Join(source, "a.txt")}
			if marked {
				markClipboardItems(t, &v.panels[0])
				want = []string{filepath.Join(source, "folder"), filepath.Join(source, "a.txt")}
			}
			if err := os.Mkdir(filepath.Join(target, "highlighted"), 0700); err != nil {
				t.Fatal(err)
			}
			v.load(1, target, "")
			waitLoaded(t, v, 1)
			selectName(t, &v.panels[1], "highlighted")
			v.panels[1].toggleMark(v.panels[1].selected)
			selectName(t, &v.panels[2], "z.txt")
			v.panels[2].toggleMark(v.panels[2].selected)
			requests := make(chan browser.TransferRequest, 1)
			v.ownerWindow = 123
			v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
				requests <- request
				return fixtureTransfer(request)
			}
			send, _ := panelInputFrames(v)
			shortcut := key.NameF5
			if move {
				shortcut = key.NameF6
			}
			send(pressed(shortcut))
			// The operation retains the selection captured at F5/F6.
			selectName(t, &v.panels[0], "z.txt")
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 1)
			send()
			request := <-requests
			if !slices.Equal(request.Paths, want) || request.Directory != target || request.Move != move || request.Duplicate || request.Owner != 123 {
				t.Fatalf("wrong native dispatch: %+v", request)
			}
			assertBatchFile(t, filepath.Join(target, "a.txt"), "alpha")
			if _, err := os.Stat(filepath.Join(target, "highlighted", "a.txt")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("transfer used the highlighted child folder")
			}
			if marked {
				assertBatchFile(t, filepath.Join(target, "folder", "nested", "file"), "nested")
			}
			current, _ := c.read()
			if !reflect.DeepEqual(current, before) || !reflect.DeepEqual(v.cutClipboard, before) {
				t.Fatal("transfer changed the clipboard or cut appearance")
			}
			if v.active != 1 || v.inlineInteraction() != nil || v.transferPending || v.panels[0].items[v.panels[0].selected].name != "z.txt" {
				t.Fatal("completion lost destination focus/source current item or retained targeting")
			}
			names := []string{"a.txt"}
			if marked {
				names = []string{"folder", "a.txt"}
			}
			if p := &v.panels[1]; p.items[p.selected].name != names[0] || len(p.marks) != 0 {
				t.Fatal("destination did not select the first result without marks")
			}
			selectName(t, &v.panels[1], "highlighted")
			send(pressed(insertKey))
			if names, _ := v.panels[1].removalSelection(); !slices.Equal(names, []string{"highlighted"}) {
				t.Fatal("later deletion included an unmarked transfer result", names)
			}
			if p := &v.panels[2]; p.items[p.selected].name != "z.txt" || !p.marks[p.selected] {
				t.Fatal("completion changed the other panel's current item/mark")
			}
			if !move {
				assertBatchFile(t, filepath.Join(source, "a.txt"), "alpha")
			} else if _, err := os.Stat(filepath.Join(source, "a.txt")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("move left its source behind")
			}
		}
	}
}

func TestPanelTransferCancelledOrFailedBatchKeepsPartialResults(t *testing.T) {
	for _, fail := range []bool{false, true} {
		v, source, target := clipboardView(t)
		forbidTransferClipboard(t, v)
		markClipboardItems(t, &v.panels[0])
		v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
			request.Paths = request.Paths[:1]
			result, err := fixtureTransfer(request)
			if err != nil {
				return result, err
			}
			if fail {
				return result, fs.ErrPermission
			}
			result.Aborted = true
			return result, nil
		}
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF6), pressed(key.NameReturn))
		waitLoaded(t, v, 1)
		send()
		assertBatchFile(t, filepath.Join(target, "folder", "nested", "file"), "nested")
		if p := &v.panels[1]; p.items[p.selected].name != "folder" || len(p.marks) != 0 {
			t.Fatal("partial transfer did not select only its successful item")
		}
		assertBatchFile(t, filepath.Join(source, "a.txt"), "alpha")
		selectName(t, &v.panels[0], "a.txt")
		if !v.panels[0].marks[v.panels[0].selected] || len(v.panels[0].marks) != 1 || v.active != 1 {
			t.Fatal("partial move lost surviving marks or destination focus")
		}
		want := "1 transferred; operation stopped"
		if fail {
			want = "1 transferred; Access denied"
		}
		if message := v.panels[1].feedback.Current(); message == nil || message.Text != want {
			t.Fatalf("partial result feedback: %v", message)
		}
	}
}

func TestPanelTransferIgnoresInvalidSourcesAndEditorKeys(t *testing.T) {
	for _, state := range []string{"parent", "drives", "loading", "busy", "modifier", "location", "name"} {
		t.Run(state, func(t *testing.T) {
			v, _, _ := clipboardView(t)
			selectName(t, &v.panels[0], "a.txt")
			router, send := clipboardFrames(v)
			shortcut := pressed(key.NameF5)
			second := pressed(key.NameF6)
			switch state {
			case "parent":
				v.panels[0].selected = 0
			case "drives":
				v.panels[0].drives = true
			case "loading":
				v.panels[0].loading = true
			case "busy":
				v.loader.busy.Store(1)
			case "modifier":
				shortcut.Modifiers = key.ModShift
				second.Modifiers = key.ModShift
			case "location":
				send(modified("D", key.ModAlt))
			case "name":
				send(pressed(key.NameF2))
			}
			editor := v.activeEditor()
			text, first, last := "", 0, 0
			if editor != nil {
				text = editor.Text()
				first, last = editor.Selection()
			}
			send(shortcut, second)
			if v.inlineInteraction() != nil || v.transferPending || v.active != 0 {
				t.Fatal("invalid source/editor accepted targeting")
			}
			if editor != nil {
				gotFirst, gotLast := editor.Selection()
				if v.activeEditor() != editor || editor.Text() != text || gotFirst != first || gotLast != last || !router.Source().Focused(editor) {
					t.Fatal("F5/F6 disturbed editor text, selection or focus")
				}
			}
		})
	}
}

func TestPanelTransferRejectsInvalidDestinationWithoutLosingMode(t *testing.T) {
	for _, state := range []string{"drives", "loading", "busy"} {
		t.Run(state, func(t *testing.T) {
			v, _, _ := clipboardView(t)
			selectName(t, &v.panels[0], "a.txt")
			send, _ := panelInputFrames(v)
			send(pressed(key.NameF5))
			switch state {
			case "drives":
				v.panels[1].drives = true
			case "loading":
				v.panels[1].loading = true
			case "busy":
				v.loader.busy.Store(1)
			}
			send(pressed(key.NameReturn))
			if v.inlineInteraction() == nil || v.active != 1 || v.transferPending || len(v.loader.requests) != 0 {
				t.Fatal("invalid destination submitted work or lost targeting")
			}
			send(pressed(key.NameEscape))
			if v.active != 0 || v.inlineInteraction() != nil {
				t.Fatal("invalid destination prevented cancellation")
			}
		})
	}
}

func TestPanelTransferSameFolderAliasesReuseDuplicateAndMoveNoOp(t *testing.T) {
	for _, move := range []bool{false, true} {
		v, root, _ := clipboardView(t)
		forbidTransferClipboard(t, v)
		selectName(t, &v.panels[0], "a.txt")
		send, _ := panelInputFrames(v)
		shortcut := key.NameF5
		if move {
			shortcut = key.NameF6
		}
		send(pressed(shortcut), pressed(key.NameRightArrow), pressed(key.NameReturn))
		waitLoaded(t, v, 2)
		send()
		if v.active != 2 || v.inlineInteraction() != nil {
			t.Fatal("alias transfer lost destination focus or retained targeting")
		}
		if move {
			if v.nameInteraction() != nil || v.transferPending || v.panels[2].feedback.Current().Text != "Already in this folder" {
				t.Fatal("same-folder move did not retain the existing no-op behavior")
			}
		} else {
			if v.nameInteraction() == nil || v.nameEditor.Text() != "a - Copy.txt" {
				t.Fatal("same-folder copy did not reuse the duplicate name editor")
			}
			send(pressed(key.NameReturn))
			waitCreated(t, v)
			send()
			assertBatchFile(t, filepath.Join(root, "a - Copy.txt"), "alpha")
			if p := &v.panels[2]; !slices.Equal(p.selectedNames(), []string{"a - Copy.txt"}) || len(p.marks) != 0 {
				t.Fatal("same-folder copy did not select its result without marks")
			}
		}
		assertBatchFile(t, filepath.Join(root, "a.txt"), "alpha")
	}
}

func TestPanelTransferTabCarriesActionAndSkipsSource(t *testing.T) {
	for _, shortcut := range []key.Name{key.NameF5, key.NameF6} {
		for _, backwards := range []bool{false, true} {
			v, source, _ := clipboardView(t)
			v.load(1, source, "")
			waitLoaded(t, v, 1)
			v.active = 1
			selectName(t, &v.panels[1], "a.txt")
			router, send := clipboardFrames(v)
			send(pressed(shortcut))
			if v.active != 2 || v.inlineInteraction() == nil {
				t.Fatal("shortcut did not start in the pane on the right")
			}
			if backwards {
				send(modified(key.NameTab, key.ModShift))
				if v.active != 0 || v.inlineInteraction() == nil || v.inlineInteraction().panel != 0 || !router.Source().Focused(&v.focus) {
					t.Fatal("Shift+Tab did not carry targeting to the previous destination")
				}
				send(modified(key.NameTab, key.ModShift))
				if v.active != 2 || v.inlineInteraction() == nil || v.inlineInteraction().panel != 2 {
					t.Fatal("Shift+Tab did not cycle from the first destination to the last")
				}
			} else {
				send(pressed(key.NameTab))
				if v.active != 0 || v.inlineInteraction() == nil || v.inlineInteraction().panel != 0 {
					t.Fatal("Tab did not cycle from the last destination to the first")
				}
				send(pressed(key.NameTab))
				if v.active != 2 || v.inlineInteraction() == nil || v.inlineInteraction().panel != 2 || !router.Source().Focused(&v.focus) {
					t.Fatal("Tab did not carry targeting to the next destination")
				}
			}
			send(pressed(key.NameEscape))
			if v.active != 1 || v.inlineInteraction() != nil {
				t.Fatal("Tab navigation prevented cancellation back to the source")
			}
		}
	}
}

func TestPanelTransferRowClickRetainsNormalInteraction(t *testing.T) {
	v, source, _ := clipboardView(t)
	v.load(1, source, "")
	waitLoaded(t, v, 1)
	v.active = 1
	selectName(t, &v.panels[1], "a.txt")
	router, send := clipboardFrames(v)
	send(pressed(key.NameF5))
	if v.active != 2 || v.inlineInteraction() == nil || !router.Source().Focused(&v.focus) {
		t.Fatal("shortcut did not offer the action in the pane on the right")
	}
	v.panels[0].rows[1].Click()
	send()
	if v.inlineInteraction() != nil || v.active != 0 || v.panels[0].selected != 1 || !router.Source().Focused(&v.focus) {
		t.Fatal("row click did not cancel targeting and perform its normal selection")
	}
}

func TestPanelTransferCyclesPastSourceAtEveryPosition(t *testing.T) {
	for _, shortcut := range []key.Name{key.NameF5, key.NameF6} {
		for _, tc := range []struct {
			source int
			panels []int
		}{
			{0, []int{2, 1, 2, 1}},
			{1, []int{0, 2, 0, 2}},
			{2, []int{0, 1, 0, 1}},
		} {
			v, root, _ := clipboardView(t)
			v.load(1, root, "")
			waitLoaded(t, v, 1)
			v.active = tc.source
			selectName(t, &v.panels[tc.source], "a.txt")
			forbidTransferClipboard(t, v)
			send, _ := panelInputFrames(v)
			send(pressed(shortcut))
			for index, panel := range tc.panels {
				event := pressed(key.NameTab)
				if index >= 2 {
					event.Modifiers = key.ModShift
				}
				send(event)
				if v.active != panel || v.inlineInteraction() == nil || v.inlineInteraction().panel != panel || v.transferPending {
					t.Fatalf("cycling from source %d lost target/action: active=%d want=%d", tc.source, v.active, panel)
				}
			}
			send(pressed(key.NameEscape))
			if v.active != tc.source || v.inlineInteraction() != nil {
				t.Fatal("cycling prevented Escape from restoring source focus")
			}
		}
	}
}

func TestPanelTransferCyclingWithOneDestinationKeepsItsAction(t *testing.T) {
	v, _, _ := clipboardView(t)
	v.panels = v.panels[:2]
	selectName(t, &v.panels[0], "a.txt")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF5))
	for _, event := range []key.Event{pressed(key.NameTab), modified(key.NameTab, key.ModShift)} {
		send(event)
		if v.active != 1 || v.inlineInteraction() == nil || v.inlineInteraction().panel != 1 {
			t.Fatal("cycling with one destination selected the source or lost its action")
		}
	}
	send(pressed(key.NameEscape))
	if v.active != 0 || v.inlineInteraction() != nil {
		t.Fatal("single destination did not retain normal cancellation")
	}
}

func TestPanelTransferIntoEmptyFolderAndLaterFocusChange(t *testing.T) {
	v, _, target := clipboardView(t)
	selectName(t, &v.panels[0], "a.txt")
	// An empty listing still has a usable destination action.
	v.panels[1].items, v.panels[1].selected = nil, -1
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		return fixtureTransfer(request)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF5))
	if v.inlineInteraction() == nil || v.inlineInteraction().row != 0 {
		t.Fatal("empty destination did not offer an inline action")
	}
	send(pressed(key.NameReturn), pressed(key.NameRightArrow), pressed(key.NameRightArrow))
	waitLoaded(t, v, 1)
	send()
	if v.active != 2 {
		t.Fatal("completion stole focus after subsequent panel navigation")
	}
	assertBatchFile(t, filepath.Join(target, "a.txt"), "alpha")
}

func TestPanelTransferCompletionPreservesLaterFocusForEveryOutcome(t *testing.T) {
	for _, outcome := range []string{"success", "cancel", "error"} {
		t.Run(outcome, func(t *testing.T) {
			v, _, _ := clipboardView(t)
			selectName(t, &v.panels[0], "a.txt")
			entered, release := make(chan struct{}), make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
				close(entered)
				<-release
				result, err := fixtureTransfer(request)
				if outcome == "cancel" {
					result.Aborted = true
				} else if outcome == "error" {
					err = fs.ErrPermission
				}
				return result, err
			}
			send, _ := panelInputFrames(v)
			send(pressed(key.NameF6), pressed(key.NameReturn))
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("transfer did not reach the worker")
			}
			if v.active != 1 {
				t.Fatal("confirmation did not keep destination focus")
			}
			send(pressed(key.NameRightArrow))
			if v.active != 2 {
				t.Fatal("browsing did not move focus during the transfer")
			}
			close(release)
			waitLoaded(t, v, 1)
			send()
			if v.active != 2 {
				t.Fatal("transfer outcome stole later panel focus")
			}
		})
	}
}

func TestPanelTransferPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if directory == "" {
		t.Skip("optional headless panel-targeting preview")
	}
	for _, move := range []bool{false, true} {
		v := newPreviewView()
		// The preview uses fixed sample entries; no worker or persistence is involved.
		v.loader = &directoryLoader{}
		v.active = 1
		v.startPanelTransfer(move)
		name := "copy"
		if move {
			name = "move"
		}
		if err := renderViewPNG(filepath.Join(directory, name+"-target-default.png"), v); err != nil {
			t.Fatal(err)
		}
		v.navigateInlineActions(-1, false)
		if err := renderViewPNG(filepath.Join(directory, name+"-target-other.png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
