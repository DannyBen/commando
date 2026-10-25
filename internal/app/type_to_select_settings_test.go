package app

import (
	"os"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestTypeToSelectDelayCoversTypingBackspaceArrowsAndExpiration(t *testing.T) {
	v := selectionView()
	if v.prefixResetDelay() != time.Second {
		t.Fatal("default delay changed")
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingTypeToSelectDelay, settings.Value{Number: 3}); err != nil {
		t.Fatal(err)
	}
	// Input can arrive after inactivity even without an intervening expiration frame.
	start := time.Now()
	v.typeToSelect("a", start)
	v.typeToSelect("b", start.Add(2*time.Second))
	p := &v.panels[0]
	if p.prefix != "ab" {
		t.Fatal("continued typing ignored the custom delay", p.prefix)
	}
	p.prefix = ""
	send, advance := panelInputFrames(v)
	send(key.EditEvent{Text: "ab"})
	advance(2 * time.Second)
	send(pressed(key.NameDeleteBackward))
	if p.prefix != "a" {
		t.Fatal("Backspace ignored the custom delay", p.prefix)
	}
	advance(2 * time.Second)
	before := p.selected
	send(pressed(key.NameDownArrow))
	if p.prefix != "a" || p.selected == before {
		t.Fatal("prefix cycling ignored the custom delay")
	}
	advance(2 * time.Second)
	if p.prefix != "a" {
		t.Fatal("cycling did not extend the custom delay")
	}
	advance(time.Second)
	if p.prefix != "" {
		t.Fatal("prefix did not expire at the configured delay")
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingTypeToSelectDelay, settings.Value{Number: .5}); err != nil {
		t.Fatal(err)
	}
	send(key.EditEvent{Text: "a"})
	advance(500 * time.Millisecond)
	if p.prefix != "" {
		t.Fatal("short delay did not expire the prefix")
	}
}

func TestTypeToSelectWidgetPreviewAndCancel(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@Interface Type to select"})
	palette := v.paletteInteraction()
	if palette == nil || len(palette.results) == 0 || palette.results[0].command.id != "settings.feedback.type_to_select" {
		t.Fatal("setting is not discoverable under Interface")
	}
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.title != "Interface › Type to select" || len(s.fields) != 1 || s.fields[0].Label != "Reset delay (seconds)" {
		t.Fatal("unexpected type-to-select widget")
	}
	send(pressed(key.NameRightArrow))
	if v.prefixResetDelay() != 1500*time.Millisecond {
		t.Fatal("half-second step was not previewed")
	}
	if output := os.Getenv("COMMANDO_PREVIEW_TYPE_TO_SELECT"); output != "" {
		if err := renderViewPNG(output, v); err != nil {
			t.Fatal(err)
		}
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	if v.prefixResetDelay() != time.Second || v.settingsInteraction() != nil {
		t.Fatal("Cancel did not restore the reset delay")
	}
}

func TestTypeToSelectDelayBoundsSaveAndReset(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget("feedback.type_to_select")
	for range 20 {
		v.adjustSetting(settingTypeToSelectDelay, 1)
	}
	if v.prefixResetDelay() != 5*time.Second {
		t.Fatal("delay did not stop at 5 seconds")
	}
	for range 20 {
		v.adjustSetting(settingTypeToSelectDelay, -1)
	}
	if v.prefixResetDelay() != 500*time.Millisecond {
		t.Fatal("delay did not stop at half a second")
	}
	for _, value := range []float64{0, .4, 5.5} {
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingTypeToSelectDelay, settings.Value{Number: value}); err == nil {
			t.Fatalf("accepted invalid delay %v", value)
		}
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.prefixResetDelay() != 500*time.Millisecond {
		t.Fatal("saved delay did not survive startup")
	}
	restored.startSettingsWidget("feedback.type_to_select")
	restored.activateSettingsAction(layout.Context{}, 2)
	if restored.prefixResetDelay() != time.Second {
		t.Fatal("Reset did not restore one second")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.prefixResetDelay() != 500*time.Millisecond {
		t.Fatal("Cancel did not undo Reset")
	}
}
