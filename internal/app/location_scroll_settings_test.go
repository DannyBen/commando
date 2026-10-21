package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestLocationScrollWidgetKeyboardPreviewAndCancel(t *testing.T) {
	v := newPreviewView()
	v.panels[0].path = `W:\go\commando\日本語\café\a very long folder\internal\app\themes`
	before := v.settingsValues.Clone()
	send, advance := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	send(key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: "Interface Animations Location bar"})
	palette := v.paletteInteraction()
	if len(palette.results) == 0 || palette.results[0].command.id != "settings."+settingLocationScroll {
		t.Fatal("location animation was not discoverable by its interface path")
	}
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.title != "Interface › Animations › Location bar" || len(s.fields) != 3 || s.fields[0].Label != "Enabled" || s.fields[1].Label != "Animation" || s.fields[2].Label != "Speed" {
		t.Fatal("location widget did not offer the agreed controls")
	}
	p := &v.panels[0]
	if p.locationScroll.start.IsZero() || p.locationScroll.ticker || p.locationScroll.speed != textScrollSpeed {
		t.Fatal("widget did not preview existing Ping Pong behavior at default speed")
	}
	send(pressed(key.NameRightArrow))
	advance(3 * time.Second)
	if p.locationScroll != (textMarquee{}) {
		t.Fatal("Enabled Off did not stop location animation")
	}
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow), pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingLocationScrollModel) != "Ticker" || v.settingText(settingLocationScrollSpeed) != "6" {
		t.Fatal("keyboard did not change animation and speed")
	}
	send(pressed(key.NameUpArrow), pressed(key.NameUpArrow), pressed(key.NameLeftArrow))
	if !p.locationScroll.ticker || p.locationScroll.speed != 36 || p.locationScroll.start.IsZero() {
		t.Fatalf("enabling did not preview Ticker at the chosen speed: enabled=%s model=%s speed=%s selected=%d scroll=%+v", v.settingText(settingLocationScrollEnabled), v.settingText(settingLocationScrollModel), v.settingText(settingLocationScrollSpeed), s.selected, p.locationScroll)
	}
	send(pressed(key.NameEscape))
	if !s.actionsFocused || s.action != 1 {
		t.Fatal("Escape did not offer the existing Cancel action")
	}
	send(pressed(key.NameReturn))
	if !reflect.DeepEqual(v.settingsValues.Clone(), before) || v.paletteInteraction() == nil || p.locationScroll != (textMarquee{}) {
		t.Fatal("Cancel did not restore settings and return to settings search with scrolling paused")
	}
	send(pressed(key.NameEscape))
	if !v.interaction.idle() || p.locationScroll.ticker || p.locationScroll.speed != textScrollSpeed {
		t.Fatal("returning to browsing did not restore normal location scrolling")
	}
}

func TestLocationScrollSaveStartupResetAndBounds(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingNotificationAnimation, settings.Value{Bool: false})
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: 2})
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget(settingLocationScroll)
	v.adjustSetting(settingLocationScrollEnabled, 0)
	v.adjustSetting(settingLocationScrollModel, 1)
	for range 20 {
		v.adjustSetting(settingLocationScrollSpeed, 1)
	}
	if v.settingText(settingLocationScrollSpeed) != "10" {
		t.Fatal("speed did not stop at 10")
	}
	for _, n := range []float64{0, 11, 1.5} {
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingLocationScrollSpeed, settings.Value{Number: n}); err == nil {
			t.Fatalf("accepted invalid speed %v", n)
		}
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.appearance.Feedback.Enabled || values.Get(settings.CatalogueDefault, settingSoundProfile).Number != 2 {
		t.Fatal("Interface regrouping lost existing sound or notification settings")
	}
	if values.Get(settings.CatalogueDefault, settingLocationScrollEnabled).Bool || restored.settingText(settingLocationScrollModel) != "Ticker" || restored.settingText(settingLocationScrollSpeed) != "10" {
		t.Fatal("location preferences did not survive save and startup")
	}
	restored.startSettingsWidget(settingLocationScroll)
	restored.activateSettingsAction(layout.Context{}, 2)
	if !restored.settingsValues.Get(settings.CatalogueDefault, settingLocationScrollEnabled).Bool || restored.settingText(settingLocationScrollModel) != "Ping Pong" || restored.settingText(settingLocationScrollSpeed) != "5" {
		t.Fatal("Reset did not restore On / Ping Pong / 5")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.settingText(settingLocationScrollModel) != "Ticker" {
		t.Fatal("Cancel did not undo Reset")
	}
}

func TestLocationScrollSettingsPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_LOCATION_SCROLL")
	if dir == "" {
		t.Skip("optional location settings and ticker previews")
	}
	v := newPreviewView()
	v.panels[0].path = `C:\Projects\日本語\café\Commando\internal\app\a long folder name`
	v.startSettingsWidget(settingLocationScroll)
	for _, model := range []string{"Ping Pong", "Ticker"} {
		_ = v.setWidgetValue(settingLocationScrollModel, settings.Value{Text: model})
		if err := renderViewPNG(filepath.Join(dir, "settings-location-"+model+".png"), v); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		v.panels[0].locationScroll.start = start.Add(-textScrollDelay - 3*time.Second)
		if err := renderViewPNG(filepath.Join(dir, "settings-location-moving-"+model+".png"), v); err != nil {
			t.Fatal(err)
		}
		if model == "Ticker" {
			loop := time.Duration(float64(v.panels[0].locationScroll.fullWidth) / float64(textScrollSpeed) * float64(time.Second))
			v.panels[0].locationScroll.start = time.Now().Add(-textScrollDelay - loop + 2*time.Second)
			if err := renderViewPNG(filepath.Join(dir, "settings-location-ticker-wrap.png"), v); err != nil {
				t.Fatal(err)
			}
		}
	}
}
