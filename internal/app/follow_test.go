package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func followView(t *testing.T, count int) (*view, []string) {
	t.Helper()
	paths := make([]string, count)
	for i := range paths {
		paths[i] = filepath.Join(t.TempDir(), fmt.Sprintf("panel-%d", i))
		for _, name := range []string{"common", "other"} {
			if err := os.MkdirAll(filepath.Join(paths[i], name), 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	v := newBrowserView(paths, nil)
	t.Cleanup(v.finishOperations)
	for i := range paths {
		waitLoaded(t, v, i)
	}
	return v, paths
}

func TestFollowToggleSelectionAndPalette(t *testing.T) {
	v := selectionView()
	v.panels[1].selected = 4
	v.panels[1].marks = map[int]bool{4: true}
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModAlt))
	if !v.followMe {
		t.Fatal("Alt+X did not enable Follow me")
	}
	send(pressed(key.NameDownArrow))
	if v.panels[1].selected != 2 || v.active != 0 {
		t.Fatal("arrow selection did not follow without stealing focus")
	}
	assertMarks(t, &v.panels[1], 4)
	send(key.EditEvent{Text: "char"})
	if v.panels[1].selected != 3 {
		t.Fatal("type-to-select did not broadcast")
	}
	// Clicking another panel makes it the source for this action.
	height := v.panels[1].rows[4].size.Y
	rowClick(send, f32.Pt(420, float32(68+4*height+height/2)), time.Second, 0)
	if v.active != 1 || v.panels[0].selected != 4 {
		t.Fatal("click selection did not broadcast from the new active panel")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Follow me"})
	s := v.paletteInteraction()
	if s == nil || len(s.results) == 0 || s.results[0].command.label() != "Panel › Follow me" {
		t.Fatal("Follow me is missing from the Panel catalogue")
	}
	send(pressed(key.NameReturn))
	if v.followMe || !v.interaction.idle() {
		t.Fatal("palette did not toggle Follow me off")
	}
	send(pressed(key.NameUpArrow))
	if v.panels[0].selected != 4 {
		t.Fatal("selection followed while disabled")
	}
}

func TestFollowSelectionSkipsUnavailableMatches(t *testing.T) {
	for _, state := range []string{"missing", "type", "loading", "refreshing", "home", "archive", "parent"} {
		t.Run(state, func(t *testing.T) {
			v := selectionView()
			v.followMe = true
			p := &v.panels[1]
			p.selected = 4
			// Give the target an independent projection before changing it.
			p.items = append([]entry(nil), p.items...)
			switch state {
			case "missing":
				p.items = p.items[2:]
				p.selected = 2
			case "type":
				p.items[1].file.Directory = false
			case "loading":
				p.loading = true
			case "refreshing":
				p.refreshing = true
			case "home":
				p.drives = true
			case "archive":
				p.archiveFolder = "."
			case "parent":
				v.panels[0].selected = 0
			}
			before := p.selected
			v.followSelection()
			if p.selected != before {
				t.Fatal("unavailable match changed selection")
			}
		})
	}
}

func TestFollowNavigationAcrossNinePanels(t *testing.T) {
	v, paths := followView(t, 9)
	v.followMe = true
	v.active = 4
	selectName(t, &v.panels[4], "common")
	_, press := navigationFrames(v)
	press(key.NameReturn)
	for i := range paths {
		waitLoaded(t, v, i)
		if v.panels[i].path != filepath.Join(paths[i], "common") {
			t.Fatalf("panel %d did not enter its own matching child", i)
		}
	}
	// Any participant can lead the next action; no leader or roots are retained.
	v.switchPanelTo(7)
	press(key.NameDeleteBackward)
	for i := range paths {
		waitLoaded(t, v, i)
		if v.panels[i].path != paths[i] {
			t.Fatalf("panel %d did not return to its own parent", i)
		}
	}
	if v.active != 7 || !v.followMe {
		t.Fatal("following changed focus or disabled the mode")
	}
}

func TestFollowMissingChildAndNestedDetourKeepOtherPanelsInPlace(t *testing.T) {
	v, paths := followView(t, 3)
	v.followMe = true
	// Remove after caching to exercise a real failed follower read, rather
	// than only the cached no-match case. It must not recover to an ancestor.
	if err := os.Remove(filepath.Join(paths[1], "common")); err != nil {
		t.Fatal(err)
	}
	selectName(t, &v.panels[1], "other")
	v.panels[1].toggleMark(v.panels[1].selected)
	before := v.panels[1].selected
	selectName(t, &v.panels[0], "common")
	v.activateSelected()
	for i := range paths {
		waitLoaded(t, v, i)
	}
	p := &v.panels[1]
	if p.path != paths[1] || p.selected != before || p.err != nil || p.feedback.Current() != nil || !v.followMe {
		t.Fatal("missing follower changed location/state or produced an error")
	}
	assertMarks(t, p, before)
	// A deeper detour exists only in the source panel. Neither the panel at
	// its initial folder nor the panel in common should leave on the way back.
	private := filepath.Join(paths[0], "common", "private", "nested")
	if err := os.MkdirAll(private, 0700); err != nil {
		t.Fatal(err)
	}
	v.refreshPanel()
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], "private")
	v.activateSelected()
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], "nested")
	v.activateSelected()
	waitLoaded(t, v, 0)
	for range 2 {
		v.parentFolder()
		waitLoaded(t, v, 0)
		if v.panels[1].loading || v.panels[2].loading || p.path != paths[1] || v.panels[2].path != filepath.Join(paths[2], "common") {
			t.Fatal("leaving the private detour moved an unrelated panel")
		}
	}
	v.parentFolder()
	for i := range paths {
		waitLoaded(t, v, i)
		if v.panels[i].path != paths[i] {
			t.Fatalf("panel %d: leaving common moved a differently named folder", i)
		}
	}
	assertMarks(t, p, before)
	// Leaving differently named starting folders affects only the source.
	v.parentFolder()
	waitLoaded(t, v, 0)
	if p.loading || v.panels[2].loading {
		t.Fatal("leaving the initial folder moved other panels")
	}
	selectName(t, &v.panels[0], filepath.Base(paths[0]))
	v.activateSelected()
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], "other")
	v.activateSelected()
	for i := range paths {
		waitLoaded(t, v, i)
		if v.panels[i].path != filepath.Join(paths[i], "other") {
			t.Fatalf("panel %d: matching navigation did not follow after returning", i)
		}
	}
}

func TestFollowSkipsBusyPanelsWithoutReplay(t *testing.T) {
	v, paths := followView(t, 3)
	v.followMe = true
	// A submitted ordinary read owns panel 1 until its result is reconciled.
	v.load(1, paths[1], "other")
	selectName(t, &v.panels[0], "common")
	v.activateSelected()
	for i := range paths {
		waitLoaded(t, v, i)
	}
	if v.panels[1].path != paths[1] || v.panels[2].path != filepath.Join(paths[2], "common") {
		t.Fatal("busy panel followed, or available panel did not follow")
	}
	v.pollLoads()
	if v.panels[1].loading || v.panels[1].path != paths[1] {
		t.Fatal("skipped action was replayed")
	}
}

func TestFollowReadFailureKeepsLocationAndReportsReason(t *testing.T) {
	v := selectionView()
	v.followMe = true
	v.panels[1].loading = true
	v.acceptLoad(loadResult{request: loadRequest{panel: 1, path: "/unreadable", follow: true}, err: os.ErrPermission})
	message := v.panels[1].feedback.Current()
	if v.panels[1].path != "/other" || message == nil || message.Kind != feedback.Error || !v.followMe {
		t.Fatal("read failure did not retain location and report its error")
	}
}

func TestFollowParentAtRootStaysPut(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	v := selectionView()
	v.followMe = true
	v.panels[0].path = root
	v.panels[1].path = filepath.Join(root, "other")
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3)}
	v.parentFolder()
	if v.panels[0].loading || v.panels[0].drives || len(v.loader.requests) != 0 {
		t.Fatal("parent at root navigated or broadcast a folder departure")
	}
}

func TestFollowNavigationUsesCompleteCacheAndSkipsMissingNames(t *testing.T) {
	v, paths := followView(t, 3)
	v.followMe = true
	v.panels[1].setFilter("other")
	v.panels[2].entries = nil // No known matching child: do not probe or guess.
	selectName(t, &v.panels[0], "common")
	v.activateSelected()
	for i := range paths {
		waitLoaded(t, v, i)
	}
	if v.panels[1].path != filepath.Join(paths[1], "common") || v.panels[1].filter != "" || v.panels[2].path != paths[2] {
		t.Fatal("navigation depended on a visible match or guessed a missing child")
	}
}

func TestFollowRenameAndExplicitNavigationStayLocal(t *testing.T) {
	v, paths := followView(t, 2)
	v.followMe = true
	selectName(t, &v.panels[0], "common")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "renamed"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	if _, err := os.Stat(filepath.Join(paths[1], "common")); err != nil {
		t.Fatal("rename affected the follower", err)
	}
	if _, err := os.Stat(filepath.Join(paths[1], "renamed")); !os.IsNotExist(err) {
		t.Fatal("rename was broadcast")
	}
	send(modified("D", key.ModAlt))
	v.location.SetText(filepath.Join(paths[0], "other"))
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if v.panels[1].loading || v.panels[1].path != paths[1] || !v.followMe {
		t.Fatal("explicit navigation followed or ended Follow me")
	}
}

func TestFollowPreview(t *testing.T) {
	path := os.Getenv("COMMANDO_PREVIEW_FOLLOW")
	if path == "" {
		t.Skip("optional Follow me preview")
	}
	for _, variant := range []string{"plain", "filter", "filter-edit", "dark"} {
		v := newPreviewView()
		v.followMe = true
		if variant == "dark" {
			v.settingsValues = builtInThemes[1].values.Clone()
			v.applySettings()
		}
		// Show all three indicator states in the optional review artifact.
		v.panels[1].items[v.panels[1].selected] = v.panels[0].items[v.panels[0].selected]
		if variant == "filter" {
			v.panels[2].setFilter("zip")
		}
		if variant == "filter-edit" {
			send, _ := panelInputFrames(v)
			send(key.EditEvent{Text: "/"})
		}
		filename := path
		if variant != "plain" {
			filename = path[:len(path)-len(filepath.Ext(path))] + "-" + variant + filepath.Ext(path)
		}
		if err := renderViewPNG(filename, v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFollowHeaderUsesFileSelectionColorsAndRestoresOrdinaryHeader(t *testing.T) {
	for _, theme := range builtInThemes {
		v := selectionView()
		v.settingsValues = theme.values.Clone()
		v.applySettings()
		for _, offset := range []float64{-0.12, 0.12} {
			colors := *v.appearance.FilenameColors
			colors.activeBackgroundLightnessOffset = offset
			colors.enabled = false // Follow colors remain semantic with auto colors off.
			v.appearance.FilenameColors = &colors
			v.followMe = true
			for i := range v.panels {
				background, filename, metadata := v.metadataHeaderColors(i)
				want := colors.activeBackground(oklchFromRGB(v.followColor(i))).rgb()
				if background != want || filename != v.appearance.Colors.Surface || metadata != filename {
					t.Fatalf("%s: header ignored the file-selection offset or selected text treatment", theme.name)
				}
			}
		}
		v.followMe = false
		background, filename, metadata := v.metadataHeaderColors(0)
		if background != v.appearance.Colors.Surface || filename != v.appearance.Colors.Filename || metadata != v.appearance.Colors.Metadata {
			t.Fatal("disabling Follow did not restore ordinary header colors")
		}
	}
}

func TestFollowColorTracksCurrentRowsAndActivePanel(t *testing.T) {
	v := selectionView()
	v.followMe = true
	colors := v.appearance.Colors
	v.panels[1].marks = map[int]bool{4: true}
	if v.followColor(0) != colors.Info || v.followColor(1) != colors.Success {
		t.Fatal("lead and matching folder colors are incorrect")
	}
	v.panels[0].selected = 2
	if v.followColor(1) != colors.Error {
		t.Fatal("a different current row did not turn red")
	}
	v.followSelection()
	if v.followColor(1) != colors.Success {
		t.Fatal("successfully following a file did not turn green")
	}
	assertMarks(t, &v.panels[1], 4)
	v.switchPanelTo(1)
	if v.followColor(1) != colors.Info || v.followColor(0) != colors.Success {
		t.Fatal("switching panels did not reassign lead and matching indicators")
	}
	// An identical name is insufficient when the entry type differs.
	v.panels[0].items = append([]entry(nil), v.panels[0].items...)
	v.panels[0].items[2].file.Directory = true
	if v.followColor(0) != colors.Error {
		t.Fatal("same-name file and folder were treated as a match")
	}
}

func TestFollowColorUnavailableSelectionsAreRed(t *testing.T) {
	for _, state := range []string{"empty", "parent", "home", "archive", "navigating", "lead empty"} {
		t.Run(state, func(t *testing.T) {
			v := selectionView()
			p := &v.panels[1]
			switch state {
			case "empty":
				p.selected = -1
			case "parent":
				p.selected, v.panels[0].selected = 0, 0
			case "home":
				p.drives = true
			case "archive":
				p.archiveFolder = "."
			case "navigating":
				p.navigating = true
			case "lead empty":
				v.panels[0].selected = -1
			}
			if v.followColor(1) != v.appearance.Colors.Error {
				t.Fatal("unavailable current selection did not show red")
			}
		})
	}
}

func TestFollowTogglePlaysDistinctCuesAndRespectsMute(t *testing.T) {
	v := selectionView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: 4}); err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModAlt))
	send(key.Event{Name: "X", Modifiers: key.ModAlt, State: key.Release})
	send(modified("X", key.ModAlt))
	if len(recorder.played) != 2 || recorder.played[0] != sound.Enabled || recorder.played[1] != sound.Disabled {
		t.Fatal("toggle did not play exactly one distinct cue per change", recorder.played)
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingSoundEnabled, settings.Value{Bool: false}); err != nil {
		t.Fatal(err)
	}
	send(modified("X", key.ModAlt))
	send(modified("X", key.ModAlt))
	if len(recorder.played) != 2 {
		t.Fatal("Follow me ignored sound mute")
	}
}
