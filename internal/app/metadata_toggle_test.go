package app

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestMetadataToggleRestoresColumnAndPreservesBrowsing(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[0]
	p.metadata = metadataModified
	send, _ := panelInputFrames(v)
	items, marks := slices.Clone(p.items), maps.Clone(p.marks)
	selected, position := p.selected, p.list.Position
	send(pressed("`"))
	send(key.EditEvent{Text: "View Toggle metadata column"})
	s := v.paletteInteraction()
	if len(s.results) == 0 || s.results[0].command.id != "view.toggle_metadata_column" || s.results[0].command.shortcut() != "" {
		t.Fatal("metadata toggle must be discoverable and initially unbound")
	}
	send(pressed(key.NameReturn))
	if p.metadata != metadataNone || p.lastMetadata != metadataModified {
		t.Fatal("toggle did not remember the visible column")
	}
	setTestShortcut(t, v, "view.toggle_metadata_column", 0, "Ctrl+T")
	send(modified("T", key.ModCtrl))
	if p.metadata != metadataModified || v.panels[1].metadata != metadataSize || !slices.Equal(items, p.items) || !maps.Equal(marks, p.marks) || p.selected != selected || p.list.Position != position {
		t.Fatal("toggle lost its column or changed panel browsing state")
	}
	// Cycling to None remembers the column too; cycling remains separate.
	send(modified("M", key.ModCtrl))
	if p.metadata != metadataNone {
		t.Fatal("cycle no longer reaches None")
	}
	send(modified("T", key.ModCtrl))
	if p.metadata != metadataModified {
		t.Fatal("toggle did not restore the column hidden by cycling")
	}
	send(modified("D", key.ModAlt), modified("T", key.ModCtrl))
	if p.metadata != metadataModified || !v.locationEditing() {
		t.Fatal("toggle interrupted a native editor")
	}
}

func TestMetadataToggleFallbackAndSettingsCancel(t *testing.T) {
	for _, tc := range []struct {
		disabled []string
		want     metadataColumn
	}{
		{nil, metadataModified},
		{[]string{"modified"}, metadataSize},
		{[]string{"modified", "size"}, metadataExtension},
		{[]string{"modified", "size", "extension"}, metadataNone},
	} {
		v := newPreviewView()
		p := &v.panels[0]
		p.metadata, p.lastMetadata = metadataNone, metadataModified
		for _, name := range tc.disabled {
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingMetadataColumns+"."+name, settings.Value{Bool: false})
		}
		v.applySettings()
		v.toggleMetadataColumn()
		if p.metadata != tc.want {
			t.Fatal("wrong fallback column", p.metadata, tc.want)
		}
	}
	v := newPreviewView()
	p := &v.panels[0]
	p.metadata, p.lastMetadata = metadataModified, metadataExtension
	v.startSettingsWidget(settingMetadataColumns)
	v.adjustSetting(settingMetadataColumns+".modified", 0)
	v.finishSettingsWidget(layout.Context{}, false)
	if p.metadata != metadataModified || p.lastMetadata != metadataExtension {
		t.Fatal("Cancel did not restore visible and remembered columns")
	}
}

func TestMetadataToggleFollowsPanelsAndRestarts(t *testing.T) {
	v := newPreviewView()
	v.panels[0].metadata = metadataModified
	v.toggleMetadataColumn()
	v.swapPanel(1)
	v.closePanel()
	v.reopenPanel()
	if v.panels[v.active].metadata != metadataNone || v.panels[v.active].lastMetadata != metadataModified {
		t.Fatal("moving or restoring a panel lost the remembered column")
	}
	v.addPanel(1)
	v.toggleMetadataColumn()
	if v.panels[v.active].metadata != metadataModified || v.panels[v.active-1].metadata != metadataNone {
		t.Fatal("cloned panel did not inherit independent metadata preferences")
	}
	v.active--
	dir := t.TempDir()
	for i := range v.panels {
		v.panels[i].path = dir
	}
	if err := saveState(dir, v.sessionState(windowState{})); err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	locations, err := startingLocations([]string{filepath.Join(dir, "other")}, saved.Panels)
	if err != nil || locations[v.active].LastMetadata != "modified" {
		t.Fatal("startup paths lost remembered column", err)
	}
	restored := newLocationView(saved.Panels, v.active, nil)
	defer restored.finishOperations()
	waitLoaded(t, restored, v.active)
	restored.toggleMetadataColumn()
	if restored.panels[v.active].metadata != metadataModified {
		t.Fatal("restart did not restore the remembered visible column")
	}
}

func TestMetadataToggleResumesSortAndRetainsSelection(t *testing.T) {
	v := newView([]panel{sortedMetadataPanel(metadataSize, true)})
	p := &v.panels[0]
	selectName(t, p, "readme")
	p.toggleMark(p.selected)
	p.list.Position.First, p.list.Position.Offset = p.selected, 3
	before := visibleNames(p)
	v.toggleMetadataColumn()
	if !p.sortByMetadata || p.metadata != metadataNone || slices.Equal(before, visibleNames(p)) {
		t.Fatal("hiding metadata did not suspend its sort while retaining the preference")
	}
	v.toggleMetadataColumn()
	if p.metadata != metadataSize || !slices.Equal(before, visibleNames(p)) || p.items[p.selected].name != "readme" || !p.marks[p.selected] || p.items[p.list.Position.First].name != "readme" || p.list.Position.Offset != 3 {
		t.Fatal("restoring metadata did not resume its sort and preserve browsing")
	}
}
