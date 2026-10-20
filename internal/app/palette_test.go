package app

import (
	"image"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func paletteFrames(v *view, size image.Point, metric unit.Metric) (func(...event.Event), *input.Router) {
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: metric, Constraints: layout.Exact(size)})
		router.Frame(&ops)
	}
	frame()
	frame()
	return func(events ...event.Event) { router.Queue(events...); frame(); frame() }, &router
}

func replacePalette(v *view, text string) key.EditEvent {
	return key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.commandSearch.Text())}, Text: text}
}

func TestPaletteOpensActiveDataFolderInOriginatingPanel(t *testing.T) {
	left, right, data := t.TempDir(), t.TempDir(), t.TempDir()
	v := newBrowserView([]string{left, right}, nil)
	defer v.loader.close()
	waitLoaded(t, v, 0)
	waitLoaded(t, v, 1)
	v.active = 1
	v.activeDataFolder = data
	// A saved folder change must not redirect this command before restart.
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingDataFolder, settings.Value{Text: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"))
	send(key.EditEvent{Text: "Open data folder"})
	s := v.paletteInteraction()
	if len(s.results) == 0 || s.results[0].command.label() != "Application › Open data folder" || s.results[0].command.shortcut() != "" {
		t.Fatal("data-folder command is missing or has a shortcut")
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	if v.panels[1].path != data || v.panels[0].path != left || v.active != 1 || !v.interaction.idle() || !router.Source().Focused(&v.focus) {
		t.Fatal("data-folder command lost its active folder, panel context or browsing focus")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Info || message.Text != "Data folder" {
		t.Fatal("data-folder command did not identify the folder through Info feedback")
	}
}

func TestPaletteGroupedBrowsingAndSearch(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	s := v.paletteInteraction()
	var groups []string
	for _, result := range s.results {
		if len(groups) == 0 || groups[len(groups)-1] != result.command.group {
			groups = append(groups, result.command.group)
		}
	}
	if !slices.Equal(groups, []string{"File", "Entry", "Folder", "Navigation", "View", "Panel", "Selection", "Programs", "Appearance", "Theme", "Feedback", "Application"}) ||
		s.results[0].command.label() != "File › Edit" || s.results[1].command.label() != "Entry › Open" || s.results[2].command.label() != "Entry › Rename…" {
		t.Fatalf("blank search lost reviewed browsing order: %v", groups)
	}
	send(key.EditEvent{Text: "view"})
	if len(s.results) < 4 {
		t.Fatalf("group search found %d commands, want at least 4", len(s.results))
	}
	// Other commands can also fuzzy-match the group word. Its contiguous
	// occurrences should put all four View commands ahead of scattered matches.
	for _, result := range s.results[:4] {
		if result.command.group != "View" || !slices.Equal(result.match.Indices, []int{0, 1, 2, 3}) {
			t.Fatalf("group search or highlights failed: %+v", result)
		}
	}
	send(replacePalette(v, "navnext"))
	if len(s.results) == 0 || s.results[0].command.label() != "Navigation › Next panel" ||
		!slices.Equal(s.results[0].match.Indices, []int{0, 1, 2, 13, 14, 15, 16}) {
		t.Fatalf("combined group/action search failed: %+v", s.results)
	}
	send(pressed(key.NameReturn))
	if !v.interaction.idle() || v.active != 1 {
		t.Fatal("grouped action did not execute existing panel navigation")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Hidden files"}, pressed(key.NameReturn))
	if !v.interaction.idle() || v.panels[1].visibility != visibilityHideHidden {
		t.Fatal("Hidden files label did not retain the existing mode change")
	}
}

func TestPaletteSettingsDiscoveryRetainsSearchAndKeyboardFlow(t *testing.T) {
	for _, query := range []string{"", "settings"} {
		t.Run("query="+query, func(t *testing.T) {
			v := newPreviewView()
			v.active = 1
			send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
			send(pressed("`"))
			s := v.paletteInteraction()
			if query == "" {
				send(pressed(key.NameEnd))
			} else {
				send(key.EditEvent{Text: query})
			}
			result := s.results[s.selected].command
			if result.label() != "Application › Settings" || result.shortcut() != "@" {
				t.Fatal("Settings was not discoverable with its shortcut", result.label(), result.shortcut())
			}
			send(pressed(key.NameReturn))
			start, end := v.commandSearch.Selection()
			if v.paletteInteraction() != s || !s.settingsMode || s.query != "@" || v.commandSearch.Text() != "@" ||
				start != 1 || end != 1 || len(s.results) != len(v.settingsCatalogue()) || !router.Source().Focused(&v.commandSearch) || v.active != 1 {
				t.Fatal("Settings command lost session, prefix, caret, results or search focus")
			}
			send(pressed(key.NameDownArrow))
			if s.selected != 1 {
				t.Fatal("settings results lost keyboard navigation")
			}
			send(key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: "system"})
			if s.query != "@system" || len(s.results) != 1 || s.results[0].command.label() != "View › System files" {
				t.Fatal("typing did not refine settings search")
			}
			send(pressed(key.NameReturn))
			if widget := v.settingsInteraction(); widget == nil || widget.palette != s || widget.panel != 1 {
				t.Fatal("Enter did not open the widget in its original panel context")
			}
			send(pressed(key.NameEscape))
			if v.paletteInteraction() != s || v.commandSearch.Text() != "@system" || !router.Source().Focused(&v.commandSearch) {
				t.Fatal("widget cancellation did not restore settings search")
			}
			send(replacePalette(v, ""))
			if s.settingsMode || len(s.results) != len(v.commandCatalogue()) {
				t.Fatal("clearing the prefix did not restore commands")
			}
			send(pressed(key.NameEscape))
			if !v.interaction.idle() || !router.Source().Focused(&v.focus) {
				t.Fatal("Escape did not restore browsing focus")
			}
			before := v.panels[1].selected
			send(pressed(key.NameDownArrow))
			if v.panels[1].selected != before+1 {
				t.Fatal("browsing keys stopped working after settings discovery")
			}
		})
	}
}

func TestPaletteSearchPreservesPanelsAndRestoresFocus(t *testing.T) {
	for _, opener := range []key.Event{pressed("`"), modified("P", key.ModCtrl|key.ModShift)} {
		t.Run(string(opener.Name), func(t *testing.T) {
			v := newPreviewView()
			send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
			send(pressed(key.NameRightArrow), pressed(key.NameEnd))
			var selections []int
			var positions []layout.Position
			var marks []map[int]bool
			for i := range v.panels {
				selections = append(selections, v.panels[i].selected)
				positions = append(positions, v.panels[i].list.Position)
				marks = append(marks, maps.Clone(v.panels[i].marks))
			}
			send(opener)
			if v.paletteInteraction() == nil || v.paletteInteraction().panel != 1 || !router.Source().Focused(&v.commandSearch) {
				t.Fatal("palette did not own the originating pane and search focus")
			}
			send(key.EditEvent{Text: "cpy"})
			s := v.paletteInteraction()
			if s.query != "cpy" || len(s.results) == 0 || s.results[0].command.label() != "Entry › Copy" || !slices.Equal(s.results[0].match.Indices, []int{8, 10, 11}) {
				t.Fatalf("unexpected fuzzy results: %+v", s.results)
			}
			send(pressed(key.NameDownArrow), pressed(key.NameTab), modified(key.NameTab, key.ModShift), modified(key.NameLeftArrow, key.ModShift), modified("N", key.ModCtrl))
			if v.active != 1 || v.paletteInteraction() != s || !router.Source().Focused(&v.commandSearch) {
				t.Fatal("search navigation leaked to browsing or lost focus")
			}
			for i := range v.panels {
				if v.panels[i].selected != selections[i] || v.panels[i].list.Position != positions[i] || !maps.Equal(v.panels[i].marks, marks[i]) {
					t.Fatalf("palette changed pane %d", i)
				}
			}
			send(replacePalette(v, "no-such-command"), pressed(key.NameReturn))
			if v.paletteInteraction() == nil || len(v.paletteInteraction().results) != 0 {
				t.Fatal("empty search executed a command or retained old results")
			}
			send(pressed(key.NameEscape))
			if !v.interaction.idle() || !router.Source().Focused(&v.focus) || v.active != 1 {
				t.Fatal("Escape did not restore browsing focus")
			}
			send(pressed(key.NameUpArrow))
			if v.panels[1].selected != selections[1]-1 {
				t.Fatal("browsing did not resume after dismissal")
			}
		})
	}
}

func TestPaletteOpeningDoesNotInsertBacktick(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"), key.EditEvent{Text: "`"})
	if v.paletteInteraction() == nil || v.commandSearch.Text() != "" {
		t.Fatalf("opening inserted text: %q", v.commandSearch.Text())
	}
	send(key.EditEvent{Text: "rename"})
	if s := v.paletteInteraction(); s == nil || s.query != "rename" || len(s.results) != 1 {
		t.Fatal("opening swallowed subsequent search input")
	}
}

func TestPaletteOpeningBacktickNeverReachesRenderedFrame(t *testing.T) {
	v := newPreviewView()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))})
		router.Frame(&ops)
	}
	frame()
	frame()
	router.Queue(pressed("`"), key.EditEvent{Text: "`"})
	frame()
	if v.paletteInteraction() == nil || v.commandSearch.Text() != "" {
		t.Fatalf("opening frame displayed %q", v.commandSearch.Text())
	}
	router.Queue(key.EditEvent{Text: "rename"})
	frame()
	if v.commandSearch.Text() != "rename" || v.paletteInteraction().query != "rename" {
		t.Fatal("search text and results were not ready in the rendered frame")
	}
}

func TestPaletteRenameHandsOffToExistingEditor(t *testing.T) {
	v := newPreviewView()
	v.active = 2
	v.panels[2].selected = 1
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(modified("P", key.ModCtrl|key.ModShift))
	send(key.EditEvent{Text: "rename"}, pressed(key.NameReturn))
	if v.paletteInteraction() != nil || v.nameInteraction() == nil || v.nameInteraction().panel != 2 ||
		v.nameEditor.Text() != v.panels[2].items[1].name || !router.Source().Focused(&v.nameEditor) {
		t.Fatalf("command failed to hand off to rename: session=%T editor=%q", v.interaction.current, v.nameEditor.Text())
	}
	owner, text, selection := v.nameInteraction(), v.nameEditor.Text(), v.nameEditor.SelectedText()
	send(modified("P", key.ModCtrl|key.ModShift))
	if v.nameInteraction() != owner || v.nameEditor.Text() != text || v.nameEditor.SelectedText() != selection {
		t.Fatal("palette shortcut interrupted an editor")
	}
	send(pressed(key.NameEscape))
	if !v.interaction.idle() || !router.Source().Focused(&v.focus) {
		t.Fatal("rename cancellation did not restore browsing")
	}
}

func TestPaletteSearchSelectionAndExecutionInOneEventBatch(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "metadata column"}, pressed(key.NameDownArrow), pressed(key.NameReturn))
	if !v.interaction.idle() || v.panels[0].metadata != metadataNone {
		t.Fatalf("query, Down, Enter did not execute the second search result: metadata=%v session=%T", v.panels[0].metadata, v.interaction.current)
	}
}

func TestPaletteDefersAutomaticRefreshAndRejectsCompetingSessions(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 2)}
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "metadata"})
	owner, selected, text := v.paletteInteraction(), v.panels[0].selected, v.commandSearch.Text()
	v.acceptAutoRefresh(loadResult{request: loadRequest{panel: 0, path: v.panels[0].path, autoRefresh: true}})
	var ops op.Ops
	gtx := layout.Context{Ops: &ops}
	v.startLocation(gtx)
	v.startName(gtx, nameRename)
	v.startInlineActions(0, 1, "", "", []string{"Choose"}, nil, nil)
	if v.paletteInteraction() != owner || v.panels[0].selected != selected || !v.panels[0].refreshNeeded || v.commandSearch.Text() != text {
		t.Fatal("automatic refresh or competing session disturbed search or source context")
	}
	send(pressed(key.NameEscape))
	if v.interaction.finish(owner) || !v.interaction.idle() {
		t.Fatal("dismissed palette retained foreground ownership")
	}
}

func TestPaletteTransferHandsOffToPanelTargeting(t *testing.T) {
	v := newPreviewView()
	v.active = 1
	v.loader = &directoryLoader{requests: make(chan loadRequest, 2)}
	paths := v.panels[1].selectedPaths()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"))
	send(key.EditEvent{Text: "Move to panel"}, pressed(key.NameReturn))
	if v.paletteInteraction() != nil || v.inlineInteraction() == nil || v.active != 2 || !router.Source().Focused(&v.focus) {
		t.Fatal("palette did not enter the shared transfer targeting session")
	}
	send(pressed(key.NameReturn))
	select {
	case request := <-v.loader.requests:
		if request.transfer == nil || request.transfer.snapshot == nil || !request.transfer.snapshot.Move ||
			request.panel != 2 || !slices.Equal(request.transfer.snapshot.Paths, paths) {
			t.Fatalf("transfer lost source selection or destination: %+v", request)
		}
	default:
		t.Fatal("target confirmation did not use the existing directory worker")
	}
}

func TestPaletteRechecksBusyStateBeforeExecution(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 2)}
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "New file"})
	owner := v.paletteInteraction()
	v.loader.busy.Store(1)
	send(pressed(key.NameReturn))
	if v.paletteInteraction() != owner || v.nameInteraction() != nil || len(v.loader.requests) != 0 {
		t.Fatal("palette dispatched a competing mutation")
	}
	v.loader.busy.Store(0)
	send(pressed(key.NameReturn))
	if v.paletteInteraction() != nil || v.nameInteraction() == nil || v.nameInteraction().kind != nameCreateFile {
		t.Fatal("available command did not hand off to inline creation")
	}
	send(pressed(key.NameEscape))
	v.panels[0].drives = true
	send(pressed("`"))
	send(key.EditEvent{Text: "New file"}, pressed(key.NameReturn))
	if v.paletteInteraction() == nil || v.nameInteraction() != nil {
		t.Fatal("palette offered creation in a drive list")
	}
}

func TestPalettePointerBarrier(t *testing.T) {
	v := newPreviewView()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"))
	s := v.paletteInteraction()
	positions := []layout.Position{v.panels[0].list.Position, v.panels[1].list.Position, v.panels[2].list.Position}
	selections := []int{v.panels[0].selected, v.panels[1].selected, v.panels[2].selected}
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(20, 300), Scroll: f32.Pt(0, 120)})
	if v.paletteInteraction() != s {
		t.Fatal("scroll dismissed the palette")
	}
	for i := range v.panels {
		if v.panels[i].list.Position != positions[i] {
			t.Fatal("scroll reached a file panel underneath the modal")
		}
	}
	// A result click selects without executing and keeps typing in Search.
	point := f32.Pt(float32(s.surface.bounds.Min.X+30), float32(s.surface.bounds.Min.Y+78))
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	if v.paletteInteraction() != s || s.selected != 1 || !router.Source().Focused(&v.commandSearch) {
		t.Fatalf("result click lost search focus or executed: selected=%d", s.selected)
	}
	// Clicking outside dismisses, without selecting the obscured file row.
	point = f32.Pt(20, 120)
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	if v.paletteInteraction() != nil || !router.Source().Focused(&v.focus) {
		t.Fatal("outside click did not dismiss and restore focus")
	}
	for i := range v.panels {
		if v.panels[i].selected != selections[i] {
			t.Fatal("dismissal clicked through to the file list")
		}
	}
}

func TestPaletteDoubleClickExecutesClickedResult(t *testing.T) {
	v := newPreviewView()
	v.active = 2
	calls, origin := 0, -1
	v.toggleFullScreen = func() { calls++; origin = v.active }
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"))
	s := v.paletteInteraction()
	s.commands = []command{commandByID(v, "entry.rename"), commandByID(v, "application.toggle_full_screen")}
	s.filter("")
	send()
	point := f32.Pt(float32(s.surface.bounds.Min.X+30), float32(s.surface.bounds.Min.Y+78))
	rowClick(send, point, time.Second, 0)
	if s.selected != 1 || calls != 0 || v.paletteInteraction() != s || !router.Source().Focused(&v.commandSearch) {
		t.Fatal("single click did not select without execution and retain Search focus")
	}
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	if calls != 1 || origin != 2 || v.paletteInteraction() != nil || !router.Source().Focused(&v.focus) {
		t.Fatal("double click did not dispatch the clicked command once in its original panel")
	}
}

func TestPaletteDoubleClickCannotExecuteUnavailableCommand(t *testing.T) {
	v := newPreviewView()
	v.panels[0].drives = true
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"), key.EditEvent{Text: "Rename"})
	s := v.paletteInteraction()
	if len(s.results) == 0 || s.results[0].command.allowed(v) {
		t.Fatal("expected unavailable rename")
	}
	point := f32.Pt(float32(s.surface.bounds.Min.X+30), float32(s.surface.bounds.Min.Y+52))
	rowClick(send, point, time.Second, 0)
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	if v.paletteInteraction() != s || v.nameInteraction() != nil || !router.Source().Focused(&v.commandSearch) {
		t.Fatal("unavailable double click executed, closed the palette or lost Search focus")
	}
}

func TestPaletteDoubleClickSettingsRestoresPaletteOnCancel(t *testing.T) {
	v := newPreviewView()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"), key.EditEvent{Text: "@appearance icons"})
	s := v.paletteInteraction()
	point := f32.Pt(float32(s.surface.bounds.Min.X+30), float32(s.surface.bounds.Min.Y+52))
	rowClick(send, point, time.Second, 0)
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	widget := v.settingsInteraction()
	if widget == nil || widget.palette != s || widget.fields[0].Widget != "appearance.icons" {
		t.Fatal("settings double click did not use the existing widget handoff")
	}
	send(pressed(key.NameEscape))
	if v.paletteInteraction() != s || !router.Source().Focused(&v.commandSearch) {
		t.Fatal("settings cancellation did not restore palette and Search focus")
	}
}

func TestPaletteDoubleClickCanReplaceResults(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	s := v.paletteInteraction()
	s.commands = []command{{name: "Settings", paletteQuery: "@appearance"}}
	s.filter("")
	send()
	point := f32.Pt(float32(s.surface.bounds.Min.X+30), float32(s.surface.bounds.Min.Y+52))
	rowClick(send, point, time.Second, 0)
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	if v.paletteInteraction() != s || s.query != "@appearance" || !s.settingsMode || len(s.results) == 0 {
		t.Fatal("double click failed to replace results within the palette")
	}
}

func TestPaletteSizingAndSelectionReveal(t *testing.T) {
	for _, tc := range []struct {
		width, left, overlay int
		scale                float32
		fontSize             unit.Sp
	}{
		{320, 0, 320, 1, 15},
		{480, 0, 480, 1, 15},
		{600, 0, 0, 1, 15}, // Catalogue labels can grow beyond the minimum width.
		{800, 0, 0, 1, 15},
		{960, 0, 0, 1, 15},
		{1600, 0, 0, 1, 15},
		{1200, 0, 0, 1.5, 24}, // Content-sized width depends on font metrics.
		{720, 0, 720, 1, 48},
	} {
		v := newPreviewView()
		v.appearance.TextSize = tc.fontSize
		send, _ := paletteFrames(v, image.Pt(tc.width, 480), unit.Metric{PxPerDp: tc.scale, PxPerSp: tc.scale})
		send(pressed("`"))
		s := v.paletteInteraction()
		bounds := s.surface.bounds
		if tc.overlay != 0 && (bounds.Min.X != tc.left || bounds.Dx() != tc.overlay) ||
			bounds.Min.X != (tc.width-bounds.Dx())/2 || bounds.Min.Y < 0 || bounds.Max.Y > 480 {
			t.Fatalf("width %d scale %g: overlay bounds %v", tc.width, tc.scale, s.surface.bounds)
		}
		if tc.overlay == 0 && (bounds.Dx() < int(float32(v.appearance.OverlayMinWidth)*tc.scale) || bounds.Dx() > int(float32(v.appearance.OverlayMaxWidth)*tc.scale)) {
			t.Fatalf("content-sized palette exceeded theme bounds: %v", bounds)
		}
		var measureOps op.Ops
		measure := layout.Context{Ops: &measureOps, Metric: unit.Metric{PxPerDp: tc.scale, PxPerSp: tc.scale}}
		if s.surface.bounds.Min.Y != v.locationBarHeight(measure, v.textHeight(measure, tc.fontSize)) {
			t.Fatalf("palette did not start below the location bar and its separate edge: %v", s.surface.bounds)
		}
		send(pressed(key.NameEnd))
		pos := s.list.Position
		if s.selected != len(s.results)-1 || s.selected < pos.First || s.selected >= pos.First+pos.Count || pos.OffsetLast < 0 {
			t.Fatalf("selection not revealed at text size %g: selected=%d position=%+v", tc.fontSize, s.selected, pos)
		}
		send(pressed(key.NameHome))
		if s.selected != 0 || s.list.Position.First != 0 || s.list.Position.Offset > 0 {
			t.Fatalf("Home did not reveal the first result: selected=%d position=%+v", s.selected, s.list.Position)
		}
		bounds = s.surface.bounds
		send(replacePalette(v, "focus"))
		if s.surface.bounds.Min.X != bounds.Min.X || s.surface.bounds.Dx() != bounds.Dx() {
			t.Fatal("searching changed the palette width")
		}
	}
}

func TestPaletteHomeEndPreserveSearch(t *testing.T) {
	v := newPreviewView()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"))
	send(key.EditEvent{Text: "panel"})
	s := v.paletteInteraction()
	start, end := v.commandSearch.Selection()
	for _, name := range []key.Name{key.NameEnd, key.NameHome} {
		send(pressed(name))
		gotStart, gotEnd := v.commandSearch.Selection()
		if v.commandSearch.Text() != "panel" || s.query != "panel" || gotStart != start || gotEnd != end || !router.Source().Focused(&v.commandSearch) {
			t.Fatal("result navigation changed search text, caret or focus")
		}
		want := 0
		if name == key.NameEnd {
			want = len(s.results) - 1
		}
		if s.selected != want {
			t.Fatalf("%s selected %d, want %d", name, s.selected, want)
		}
	}
	// Modified Home still belongs to text editing.
	v.commandSearch.SetCaret(len("panel"), len("panel"))
	send()
	send(modified(key.NameHome, key.ModShift))
	if v.commandSearch.SelectedText() != "panel" || s.selected != 0 {
		t.Fatal("Shift+Home did not retain native text selection")
	}
	send(replacePalette(v, "no-such-command"), pressed(key.NameHome), pressed(key.NameEnd))
	if len(s.results) != 0 || v.paletteInteraction() != s {
		t.Fatal("Home/End on empty results changed the session")
	}
}

func TestPalettePreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if directory == "" {
		t.Skip("optional headless palette preview")
	}
	for _, query := range []string{"", "panel", "settings", "terminal", "xyz", "@xyz"} {
		v := newPreviewView()
		var ops op.Ops
		v.startPalette(layout.Context{Ops: &ops})
		v.commandSearch.SetText(query)
		v.paletteInteraction().filter(query)
		name := "palette"
		if query != "" {
			name += "-" + query
		}
		if err := renderViewPNG(filepath.Join(directory, name+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
