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
	"github.com/dannyben/commando/internal/settings"
)

func TestMetadataSettingsFourTogglesAndCreatedDefaultOff(t *testing.T) {
	v := newPreviewView()
	found := false
	for _, cmd := range v.settingsCatalogue() {
		if cmd.label() == "View › Metadata columns" {
			cmd.execute(v)
			found = true
			break
		}
	}
	s := v.settingsInteraction()
	if !found || s == nil || len(s.fields) != 4 {
		t.Fatal("metadata widget did not provide four controls")
	}
	for i, label := range []string{"Extension", "Size", "Modified", "Created"} {
		if s.fields[i].Label != label || s.fields[i].Control != "toggle" {
			t.Fatal("metadata choices are out of order or not toggles")
		}
	}
	if v.metadataColumnEnabled(metadataCreated) || !v.metadataColumnEnabled(metadataNone) {
		t.Fatal("Created must default Off and None must always be available")
	}
	v.adjustSetting(settingMetadataColumns+".created", 0)
	v.finishSettingsWidget(layout.Context{}, true)
	send, _ := panelInputFrames(v)
	for _, want := range []metadataColumn{metadataModified, metadataCreated, metadataNone, metadataExtension, metadataSize} {
		send(modified("M", key.ModCtrl))
		if v.panels[0].metadata != want {
			t.Fatal("forward cycle missed enabled column", want)
		}
	}
	for _, want := range []metadataColumn{metadataExtension, metadataNone, metadataCreated, metadataModified, metadataSize} {
		send(modified("M", key.ModCtrl|key.ModShift))
		if v.panels[0].metadata != want {
			t.Fatal("reverse cycle missed enabled column", want)
		}
	}
}

func TestMetadataSettingsDisableAllAndCancelRestoresPanels(t *testing.T) {
	v := newView([]panel{sortedMetadataPanel(metadataSize, true), sortedMetadataPanel(metadataModified, true), sortedMetadataPanel(metadataExtension, false)})
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingParentEntry, settings.Value{Bool: true})
	columns := []metadataColumn{metadataSize, metadataModified, metadataExtension}
	for i := range v.panels {
		v.panels[i].metadata = columns[i]
		v.panels[i].resort()
	}
	v.closedPanels = []closedPanel{{panel: sortedMetadataPanel(metadataSize, true)}}
	p := &v.panels[0]
	selectName(t, p, "readme")
	p.toggleMark(p.selected)
	p.list.Position.First, p.list.Position.Offset = p.selected, 3
	before, marks := v.settingsValues.Clone(), maps.Clone(p.marks)
	items, position := slices.Clone(p.items), p.list.Position
	v.startSettingsWidget(settingMetadataColumns)
	v.adjustSetting(settingMetadataColumns+".size", 0)
	if p.metadata != metadataModified || v.closedPanels[0].panel.metadata != metadataModified {
		t.Fatal("disabling Size did not advance live and closed panels")
	}
	for _, name := range []string{"modified", "extension"} {
		v.adjustSetting(settingMetadataColumns+"."+name, 0)
	}
	for i := range v.panels {
		if v.panels[i].metadata != metadataNone {
			t.Fatal("all disabled must leave None")
		}
	}
	for _, direction := range []int{-1, 1} {
		if v.nextMetadataColumn(metadataNone, direction) != metadataNone {
			t.Fatal("cycling all-disabled columns did not stay at None")
		}
	}
	if p.items[p.selected].name != "readme" || len(p.marks) != 1 || !p.marks[p.selected] {
		t.Fatal("live column change lost selected or marked file")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	for i, want := range columns {
		if v.panels[i].metadata != want {
			t.Fatal("Cancel did not restore panel column", i)
		}
	}
	if !reflect.DeepEqual(before, v.settingsValues) || !slices.Equal(items, p.items) || !maps.Equal(marks, p.marks) || p.list.Position != position || p.items[p.selected].name != "readme" || v.closedPanels[0].panel.metadata != metadataSize {
		t.Fatal("Cancel did not restore settings, order, marks, viewport and closed pane")
	}
}

func TestMetadataSettingsSaveStartupAndReset(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget(settingMetadataColumns)
	for _, name := range []string{"extension", "size", "modified", "created"} {
		v.adjustSetting(settingMetadataColumns+"."+name, 0)
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.panels[0].metadata != metadataCreated || restored.nextMetadataColumn(metadataCreated, 1) != metadataNone || restored.nextMetadataColumn(metadataNone, 1) != metadataCreated {
		t.Fatal("startup did not apply the saved column choices")
	}
	if metadataFromState(metadataCreated.stateName()) != metadataCreated {
		t.Fatal("Created panel state did not round-trip")
	}
	restored.startSettingsWidget(settingMetadataColumns)
	restored.activateSettingsAction(layout.Context{}, 2)
	if restored.metadataColumnEnabled(metadataCreated) || !restored.metadataColumnEnabled(metadataSize) || restored.panels[0].metadata != metadataNone {
		t.Fatal("Reset did not restore defaults and move away from Created")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.panels[0].metadata != metadataCreated || !restored.metadataColumnEnabled(metadataCreated) {
		t.Fatal("Cancel did not undo Reset")
	}
}

func TestMetadataSettingsPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional metadata settings preview")
	}
	v := newPreviewView()
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingMetadataColumns+".created", settings.Value{Bool: true})
	v.panels[0].metadata = metadataCreated
	v.applySettings()
	v.startSettingsWidget(settingMetadataColumns)
	if err := renderViewPNG(filepath.Join(dir, "settings-metadata-columns.png"), v); err != nil {
		t.Fatal(err)
	}
}
