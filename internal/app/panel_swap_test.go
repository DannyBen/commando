package app

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
)

func TestSwapPaneMovesFocusAndRetainsEachPaneState(t *testing.T) {
	v := newPreviewView()
	v.panels[0].metadata, v.panels[1].metadata, v.panels[2].metadata = metadataNone, metadataExtension, metadataModified
	router, send := clipboardFrames(v)
	for i := range v.panels {
		v.active = i
		send(pressed(key.NameEnd))
		for range i {
			send(pressed(key.NameUpArrow))
		}
	}
	v.active = 1
	type paneState struct {
		path     string
		items    []entry
		selected int
		marks    map[int]bool
		position layout.Position
		metadata metadataColumn
	}
	before := make([]paneState, len(v.panels))
	for i, p := range v.panels {
		before[i] = paneState{p.path, slices.Clone(p.items), p.selected, maps.Clone(p.marks), p.list.Position, p.metadata}
	}
	check := func(order []int, active int) {
		t.Helper()
		if v.active != active || !router.Source().Focused(&v.focus) {
			t.Fatal("swap did not keep focus on the moved pane")
		}
		for i, original := range order {
			p, want := &v.panels[i], before[original]
			if p.path != want.path || !slices.Equal(p.items, want.items) || p.selected != want.selected ||
				!reflect.DeepEqual(p.marks, want.marks) || p.list.Position != want.position || p.metadata != want.metadata {
				t.Fatalf("pane %d lost location, listing, selection, marks, scroll or metadata", original)
			}
		}
	}
	send(modified(key.NameRightArrow, key.ModShift))
	check([]int{0, 2, 1}, 2)
	send(modified(key.NameRightArrow, key.ModShift))
	check([]int{0, 2, 1}, 2) // The right edge does not wrap.
	send(modified(key.NameLeftArrow, key.ModShift), modified(key.NameLeftArrow, key.ModShift))
	check([]int{1, 0, 2}, 0)
	send(modified(key.NameLeftArrow, key.ModShift))
	check([]int{1, 0, 2}, 0) // The left edge does not wrap.

	// The existing store saves the reordered locations/metadata and active pane.
	dataDir := t.TempDir()
	if err := saveState(dataDir, v.sessionState(windowState{Width: 800, Height: 480})); err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(dataDir)
	if err != nil || saved.ActivePanel != 0 {
		t.Fatalf("reordered state did not round-trip: %+v %v", saved, err)
	}
	for i, original := range []int{1, 0, 2} {
		if saved.Panels[i].Path != before[original].path || saved.Panels[i].Metadata != before[original].metadata.stateName() {
			t.Fatal("saved pane order or metadata was wrong")
		}
	}
	send(pressed(key.NameDownArrow))
	if v.panels[0].selected != before[1].selected+1 || v.panels[1].selected != before[0].selected {
		t.Fatal("list keyboard input did not follow the swapped pane")
	}
}

func TestSwapPaneKeepsCutAppearanceForResolvedAliases(t *testing.T) {
	for _, direction := range []int{-1, 1} {
		v, root, _ := clipboardView(t)
		c := attachTestFileClipboard(v)
		alias := filepath.Join(root, "alias")
		// Keep an alias display name distinct from its worker-resolved backing
		// path, without depending on Windows SUBST in a development-platform test.
		v.panels[1].path = alias
		v.panels[1].items = slices.Clone(v.panels[0].items)
		selectName(t, &v.panels[1], "a.txt")
		v.panels[1].toggleMark(v.panels[1].selected)
		v.active = 1
		before, _ := c.write([]string{filepath.Join(root, "a.txt")}, true)
		v.rememberCutClipboard(before, v.fileOperationLocations(), []string{root, root, root})
		if !v.cutEntry(1, v.panels[1].selected) {
			t.Fatal("alias fixture did not show its cut item")
		}
		router, send := clipboardFrames(v)
		name := key.NameRightArrow
		if direction < 0 {
			name = key.NameLeftArrow
		}
		send(modified(name, key.ModShift))
		p := &v.panels[v.active]
		if v.active != 1+direction || p.path != alias || !p.marks[p.selected] || !v.cutEntry(v.active, p.selected) || !router.Source().Focused(&v.focus) {
			t.Fatal("swap lost alias cut appearance, marks or focus")
		}
		current, _ := c.read()
		if !reflect.DeepEqual(current, before) || !reflect.DeepEqual(v.cutClipboard, before) {
			t.Fatal("swap changed the file clipboard")
		}
	}
}

func TestSwappedPaneTransferUsesItsNewNeighbors(t *testing.T) {
	v, source, target := clipboardView(t)
	selectName(t, &v.panels[0], "a.txt")
	forbidTransferClipboard(t, v)
	requests := make(chan browser.TransferRequest, 1)
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		requests <- request
		return fixtureTransfer(request)
	}
	send, _ := panelInputFrames(v)
	// Move the source to the middle, start F5 toward the right, then choose
	// the left destination. Process the complete sequence in event order.
	send(modified(key.NameRightArrow, key.ModShift), pressed(key.NameF5), pressed(key.NameLeftArrow), pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	request := <-requests
	if v.active != 0 || request.Directory != target || !slices.Equal(request.Paths, []string{filepath.Join(source, "a.txt")}) {
		t.Fatalf("transfer used pre-swap pane positions: active=%d request=%+v", v.active, request)
	}
	assertBatchFile(t, filepath.Join(target, "a.txt"), "alpha")
	assertBatchFile(t, filepath.Join(source, "a.txt"), "alpha")
}

func TestSwapPaneIgnoredDuringPendingWorkAndInlineInteractions(t *testing.T) {
	for _, state := range []string{"source read", "target read", "other read", "mutation", "pending transfer", "inline action", "location editor", "name editor"} {
		t.Run(state, func(t *testing.T) {
			v := newPreviewView()
			v.active = 1
			router, send := clipboardFrames(v)
			switch state {
			case "source read":
				v.panels[1].loading = true
			case "target read":
				v.panels[2].loading = true
			case "other read":
				v.panels[0].loading = true
			case "mutation":
				v.loader = &directoryLoader{}
				v.loader.busy.Store(1)
			case "pending transfer":
				v.transferPending = true
			case "inline action":
				v.startInlineActions(1, v.panels[1].selected+1, "", "", []string{"Copy here"}, nil, nil)
			case "location editor":
				send(modified("D", key.ModAlt))
			case "name editor":
				send(pressed(key.NameF2))
			}
			before := v.sessionState(windowState{})
			editor := v.activeEditor()
			text, first, last := "", 0, 0
			if editor != nil {
				text = editor.Text()
				first, last = editor.Selection()
			}
			send(modified(key.NameRightArrow, key.ModShift))
			if !reflect.DeepEqual(v.sessionState(windowState{}), before) {
				t.Fatal("pending work or inline interaction allowed a pane swap")
			}
			if editor != nil {
				if v.activeEditor() != editor || editor.Text() != text || !router.Source().Focused(editor) {
					t.Fatal("swap shortcut disturbed the editor")
				}
				// Shift+Right retains its ordinary text-selection behavior.
				gotFirst, gotLast := editor.Selection()
				if gotFirst == first && gotLast == last && text != "" {
					t.Fatal("editor did not receive its ordinary Shift+Right selection key")
				}
			}
		})
	}
}

func TestSwapPaneRetainsDriveViewAndFeedback(t *testing.T) {
	v := newPreviewView()
	v.panels[1].drives = true
	v.panels[1].feedback.Progress("Reading folder")
	v.active = 1
	v.swapPanel(-1)
	message := v.panels[0].feedback.Current()
	if v.active != 0 || !v.panels[0].drives || v.panels[1].drives || message == nil || message.Text != "Reading folder" {
		t.Fatal("swapping pane models lost the drive view or feedback channel")
	}
}

func TestSwapPanePreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if directory == "" {
		t.Skip("optional headless swapped-pane preview")
	}
	v := newPreviewView()
	v.active = 1
	v.panels[1].metadata = metadataModified
	v.swapPanel(1)
	if err := renderViewPNG(filepath.Join(directory, "panes-swapped.png"), v); err != nil {
		t.Fatal(err)
	}
}
