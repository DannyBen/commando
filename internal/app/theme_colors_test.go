package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

const colorPrefix = "appearance.theme."

func TestThemeElementNamesAndCycleOrder(t *testing.T) {
	want := []string{"Background", "Background alternate", "Border", "Accent", "Text", "Text secondary", "Text selection", "Selection background", "Selection text", "Error text", "Info text", "Success text", "Warning text"}
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	if !reflect.DeepEqual(v.settingsInteraction().completion.candidates, want) {
		t.Fatal("autocomplete names or order differ from cycling")
	}
	for _, name := range want {
		if got := v.settingText(settingThemeElement); got != name {
			t.Fatalf("element = %q; want %q", got, name)
		}
		v.cycleThemeElement(layout.Context{}, 1)
	}
	if v.settingText(settingThemeElement) != want[0] {
		t.Fatal("cycle did not wrap")
	}
	if _, exists := settings.CatalogueDefault.Definition(colorPrefix + "cut_text.lightness"); exists {
		t.Fatal("separate cut color remains")
	}
}

func TestThemeColorsAreIndependent(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	before, files := v.appearance.Colors, *v.appearance.FilenameColors
	v.startSettingsWidget(settingThemeCustomize)
	if s := v.settingsInteraction(); s == nil || len(s.fields) != 4 {
		t.Fatal("Customize needs Element and three color controls")
	}
	if err := v.setWidgetValue(colorPrefix+"background.lightness", settings.Value{Number: 0.4}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	if v.settingText(colorPrefix+"alt_background.lightness") != "0.9" {
		t.Fatal("editing one color changed another")
	}
	if v.appearance.Colors == before || *v.appearance.FilenameColors != files {
		t.Fatal("color did not preview independently")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.Colors != before {
		t.Fatal("Cancel did not restore color")
	}
}

func TestThemeElementSwitchSavesAndStartsClean(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget(settingThemeCustomize)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow), pressed(key.NameLeftArrow))
	// Leave a real edit on Main, then switch using Element.
	_ = v.setWidgetValue(colorPrefix+"background.lightness", settings.Value{Number: 0.4})
	send(pressed(key.NameUpArrow))
	send(pressed(key.NameRightArrow))
	s := v.settingsInteraction()
	if s == nil || themeElementKey(s) != colorPrefix+"alt_background" || s.dirty || s.selected != 0 {
		t.Fatal("switch did not open a clean next color")
	}
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || loaded.Get(settings.CatalogueDefault, colorPrefix+"background.lightness").Number != 0.4 {
		t.Fatal("switch did not persist previous color", err)
	}
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if !s.dirty {
		t.Fatal("new color edit should be dirty")
	}
	send(pressed(key.NameEscape))
	if v.settingsInteraction() != s {
		t.Fatal("Escape discarded dirty color")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if v.settingText(colorPrefix+"background.lightness") != "0.4" || v.settingText(colorPrefix+"alt_background.lightness") != "0.9" {
		t.Fatal("Cancel discarded saved work or retained draft")
	}
}

func TestThemeSwitchWaitsForSaveAndRetainsFailedDraft(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	s := v.settingsInteraction()
	_ = v.setWidgetValue(colorPrefix+"background.lightness", settings.Value{Number: 0.4})
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	v.prepareSettingsSave = func(settings.Values) func() (string, error) { return func() (string, error) { return "", nil } }
	v.cycleThemeElement(layout.Context{}, 1)
	request := <-v.loader.requests
	if v.settingsInteraction() != s || !s.pending || !s.dirty {
		t.Fatal("editor disappeared while saving")
	}
	v.acceptLoad(loadResult{request: request, err: errors.New("disk full")})
	if v.settingsInteraction() != s || s.pending || !s.dirty || s.errorText == "" {
		t.Fatal("save failure lost editor or draft")
	}
	if s.afterSave != nil {
		t.Fatal("failed navigation should not change a later Save into a switch")
	}
	v.cycleThemeElement(layout.Context{}, 1)
	request = <-v.loader.requests
	v.acceptLoad(loadResult{request: request})
	if next := v.settingsInteraction(); next == nil || next == s || next.dirty || themeElementKey(next) != colorPrefix+"alt_background" {
		t.Fatal("save completion did not replace editor directly")
	}
	v.acceptSettingsSave(s, "", nil)
	if v.settingsInteraction() == nil {
		t.Fatal("stale completion closed new editor")
	}
}

func TestThemeElementCompletionAndCleanEscape(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	send(key.EditEvent{Range: key.Range{Start: 0, End: len([]rune(s.editor.Text()))}, Text: "Err"}, key.SelectionEvent{Start: 3, End: 3})
	if s.dirty {
		t.Fatal("navigation text made the color dirty")
	}
	send(pressed(key.NameTab))
	if s.editor.Text() != "Error text" || s.dirty {
		t.Fatal("element completion failed or dirtied color")
	}
	send(pressed(key.NameReturn))
	if themeElementKey(v.settingsInteraction()) != colorPrefix+"error" || v.settingsInteraction().dirty {
		t.Fatal("accepted name did not switch to clean color")
	}
	send(pressed(key.NameEscape))
	if v.settingsInteraction() != nil {
		t.Fatal("clean Escape did not close editor")
	}
}

func TestThemeClipboardPasteIsAtomicAndIndependent(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	s := v.settingsInteraction()
	text := v.themeClipboardText(s)
	v.cycleThemeElement(layout.Context{}, 1)
	s = v.settingsInteraction()
	v.pasteThemeColor(s, text)
	if !s.dirty || v.themeClipboardText(s) != text {
		t.Fatal("paste did not copy whole triplet")
	}
	before := v.settingsValues.Clone()
	for _, invalid := range []string{"", "0.1 0.2", "0.1 0.2 4 extra", "0.1 0.2 NaN", "0.1 0.2 Inf", "0.1 0.8 42", "0.1 0.2 361"} {
		v.pasteThemeColor(s, invalid)
		if !reflect.DeepEqual(v.settingsValues, before) || s.errorText == "" {
			t.Fatalf("invalid paste changed values: %q", invalid)
		}
	}
	v.pasteThemeColor(s, "0.6 0.1 42")
	if v.settingText(colorPrefix+"background.lightness") != "1" || !strings.HasPrefix(v.themeClipboardText(s), "0.6 ") {
		t.Fatal("paste changed another color or retained error")
	}
}

func TestThemeClipboardKeyboardAndTextEditing(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	router, send := clipboardFrames(v)
	send(modified("C", key.ModCtrl))
	_, content, ok := router.WriteClipboard()
	if !ok {
		t.Fatal("Copy did not write color clipboard")
	}
	data := content
	if string(data) != "1 0 0" {
		t.Fatal("Copy did not write LCH triplet", string(data))
	}
	send(pressed(key.NameRightArrow))
	send(modified("V", key.ModCtrl))
	if !router.ClipboardRequested() {
		t.Fatal("Paste did not request clipboard")
	}
	send(clipboardData(string(data)))
	s := v.settingsInteraction()
	if !s.dirty || v.themeClipboardText(s) != string(data) {
		t.Fatal("keyboard paste did not apply color")
	}
	// While typing a component, clipboard shortcuts retain normal text behavior.
	send(pressed(key.NameDownArrow), pressed(key.NameReturn))
	send(modified("V", key.ModCtrl))
	if !router.ClipboardRequested() {
		t.Fatal("text paste did not request clipboard")
	}
	send(clipboardData("0.25"))
	if s.editor.Text() != "0.25" || v.themeClipboardText(s) != string(data) {
		t.Fatal("text paste was interpreted as a color")
	}
	send(pressed(key.NameReturn))
	if v.settingText(colorPrefix+"alt_background.lightness") != "0.25" {
		t.Fatal("pasted component did not apply")
	}
}

func TestThemeLateClipboardCannotChangeNextElement(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	router, send := clipboardFrames(v)
	send(modified("V", key.ModCtrl))
	if !router.ClipboardRequested() {
		t.Fatal("Paste did not request clipboard")
	}
	send(pressed(key.NameRightArrow))
	s := v.settingsInteraction()
	before := v.themeClipboardText(s)
	send(clipboardData("0.1 0.2 42"))
	if v.themeClipboardText(s) != before || s.dirty {
		t.Fatal("late paste changed next element")
	}
}

func TestThemeColorKeyboardUsesFixedRanges(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(colorPrefix + "alt_background")
	send, _ := panelInputFrames(v)
	for range 3 {
		send(pressed(key.NameDownArrow))
	}
	_ = v.setWidgetValue(colorPrefix+"alt_background.hue", settings.Value{Number: 359})
	send(pressed(key.NameRightArrow))
	if v.settingText(colorPrefix+"alt_background.hue") != "0" {
		t.Fatal("hue did not wrap forward")
	}
	send(pressed(key.NameLeftArrow))
	if v.settingText(colorPrefix+"alt_background.hue") != "359" {
		t.Fatal("hue did not wrap backward")
	}
	send(pressed(key.NameUpArrow), pressed(key.NameUpArrow), pressed(key.NameReturn))
	s := v.settingsInteraction()
	send(key.EditEvent{Range: key.Range{Start: 0, End: len([]rune(s.editor.Text()))}, Text: "0.1"})
	send(pressed(key.NameReturn))
	if s.editing || v.settingText(colorPrefix+"alt_background.lightness") != "0.1" {
		t.Fatal("typed value did not use absolute range")
	}
}

func TestThemeColorPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional color widget previews")
	}
	for _, role := range []string{"background", "alt_background", "error"} {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(colorPrefix + role)
		if err := renderViewPNG(filepath.Join(dir, fmt.Sprintf("theme-color-%s.png", role)), v); err != nil {
			t.Fatal(err)
		}
	}
}
