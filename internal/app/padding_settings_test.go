package app

import (
	"image"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func TestLowPaddingUsesMeasuredTextWithoutMinimumHeights(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		for _, size := range []unit.Sp{6, 15, 32} {
			for _, padding := range []unit.Dp{0, 1, 2} {
				v := newPreviewView()
				v.appearance.TextSize, v.appearance.SmallTextSize = size, size
				v.appearance.LocationPaddingVertical = padding
				v.appearance.HeaderFooterPaddingVertical = padding
				v.appearance.SearchPaddingVertical = padding
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale},
					Constraints: layout.Exact(image.Pt(1000, 1000))}
				textHeight := v.textHeight(gtx, size)
				bandHeight := textHeight + gtx.Dp(2*padding)
				locationHeight := bandHeight + gtx.Dp(2)
				rowHeight := textHeight + 2*gtx.Dp(v.appearance.RowPaddingVertical)
				p := &v.panels[0]
				p.selected, p.reveal = 0, true
				v.panel(gtx, 0)
				if p.selectionAnchor.Y != locationHeight+bandHeight+rowHeight {
					t.Fatalf("size %g, padding %g, scale %g: first row anchor %v still includes a minimum height", size, padding, scale, p.selectionAnchor)
				}
				v.startPalette(gtx)
				s := v.paletteInteraction()
				s.results = s.results[:1]
				dims := v.layoutPalette(gtx)
				if s.surface.bounds.Min.Y != locationHeight || dims.Size.Y != 2*bandHeight+rowHeight {
					t.Fatalf("size %g, padding %g, scale %g: palette bounds %v retain a search/footer minimum", size, padding, scale, s.surface.bounds)
				}
				v.stopPalette(gtx)
				v.startSettingsWidget("appearance.padding.vertical")
				dims = v.layoutSettingsWidget(gtx)
				if dims.Size.Y != 6*rowHeight+gtx.Dp(2)+bandHeight {
					t.Fatalf("size %g, padding %g, scale %g: settings height %d retains a footer minimum", size, padding, scale, dims.Size.Y)
				}
			}
		}
	}
}

func TestPaddingWidgetsPreviewCancelResetAndSave(t *testing.T) {
	for _, tc := range []struct {
		group string
		keys  []string
	}{
		{"Vertical", []string{settingPaddingRows, settingPaddingHeadersFooters, settingPaddingLocation, settingPaddingSearch}},
		{"Horizontal", []string{settingPaddingContent}},
	} {
		t.Run(tc.group, func(t *testing.T) {
			v := newPreviewView()
			before := v.appearance
			widget := "appearance.padding." + strings.ToLower(tc.group)
			v.startSettingsWidget(widget)
			s := v.settingsInteraction()
			if s == nil || s.title != "Appearance › Padding › "+tc.group || len(s.fields) != len(tc.keys) {
				t.Fatal("incorrect padding widget")
			}
			for i, key := range tc.keys {
				if s.fields[i].Key != key {
					t.Fatal("padding field order changed")
				}
				v.adjustSetting(key, 1)
			}
			if tc.group == "Vertical" {
				if v.appearance.RowPaddingVertical != before.RowPaddingVertical+1 ||
					v.appearance.HeaderFooterPaddingVertical != before.HeaderFooterPaddingVertical+1 ||
					v.appearance.LocationPaddingVertical != before.LocationPaddingVertical+1 ||
					v.appearance.SearchPaddingVertical != before.SearchPaddingVertical+1 ||
					v.appearance.ContentPaddingHorizontal != before.ContentPaddingHorizontal {
					t.Fatal("vertical controls did not preview together or changed horizontal padding")
				}
			} else if v.appearance.ContentPaddingHorizontal != before.ContentPaddingHorizontal+1 ||
				v.contentInset().Left != before.ContentPaddingHorizontal+1 || v.appearance.RowPaddingVertical != before.RowPaddingVertical {
				t.Fatal("horizontal control did not preview shared padding or changed vertical padding")
			}
			v.finishSettingsWidget(layout.Context{}, false)
			if v.appearance.RowPaddingVertical != before.RowPaddingVertical ||
				v.appearance.HeaderFooterPaddingVertical != before.HeaderFooterPaddingVertical ||
				v.appearance.LocationPaddingVertical != before.LocationPaddingVertical ||
				v.appearance.SearchPaddingVertical != before.SearchPaddingVertical ||
				v.appearance.ContentPaddingHorizontal != before.ContentPaddingHorizontal {
				t.Fatal("Cancel did not restore padding")
			}
			v.startSettingsWidget(widget)
			for _, key := range tc.keys {
				v.adjustSetting(key, 2)
			}
			dir := t.TempDir()
			v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
				return func() (string, error) { return "", settings.Save(dir, values) }
			}
			v.finishSettingsWidget(layout.Context{}, true)
			loaded, err := settings.Load(dir, settings.CatalogueDefault)
			if err != nil || !v.interaction.idle() {
				t.Fatal("padding did not save", err)
			}
			restored := newPreviewView()
			restored.restoreSettings(loaded, nil)
			for _, key := range tc.keys {
				if restored.settingText(key) != v.settingText(key) {
					t.Fatal("saved padding did not apply on startup")
				}
			}
			restored.startSettingsWidget(widget)
			restored.activateSettingsAction(layout.Context{}, 2)
			for _, key := range tc.keys {
				if restored.settingsValues.Overridden(key) {
					t.Fatal("Reset retained a padding override")
				}
			}
			restored.finishSettingsWidget(layout.Context{}, false)
			for _, key := range tc.keys {
				if restored.settingText(key) != v.settingText(key) {
					t.Fatal("Cancel did not undo Reset")
				}
			}
		})
	}
}

func TestPaddingKeyboardInputAndMinimum(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[0]
	selected, marks := p.selected, maps.Clone(p.marks)
	originalPadding := v.appearance.RowPaddingVertical
	v.startSettingsWidget("appearance.padding.vertical")
	send, _ := panelInputFrames(v)
	originalLength := p.list.Position.Length
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 1}, Text: "-1"})
	send(pressed(key.NameReturn))
	if !v.settingsInteraction().editing || v.appearance.RowPaddingVertical != originalPadding {
		t.Fatal("negative padding changed the layout")
	}
	send(key.EditEvent{Range: key.Range{Start: 0, End: 2}, Text: "1.5"})
	send(pressed(key.NameReturn))
	if !v.settingsInteraction().editing || v.settingsInteraction().errorText != "Enter a whole number" {
		t.Fatal("fractional padding was accepted")
	}
	send(key.EditEvent{Range: key.Range{Start: 0, End: 3}, Text: "0"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameLeftArrow))
	if v.appearance.RowPaddingVertical != 0 {
		t.Fatal("padding stepped below zero")
	}
	send(pressed(key.NameRightArrow))
	if v.appearance.RowPaddingVertical != unit.Dp(1) {
		t.Fatal("padding did not step by one dp")
	}
	if p.list.Position.Length >= originalLength || p.selected != selected || !maps.Equal(p.marks, marks) {
		t.Fatal("padding did not resize rows or disturbed the selection and marks")
	}
}

func TestPaddingWidgetPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional headless padding preview")
	}
	for _, group := range []string{"vertical", "horizontal"} {
		v := newPreviewView()
		v.startSettingsWidget("appearance.padding." + group)
		if err := renderViewPNG(filepath.Join(dir, "settings-padding-"+group+".png"), v); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{settingPaddingRows, settingPaddingHeadersFooters, settingPaddingLocation, settingPaddingSearch, settingPaddingContent} {
			if err := v.settingsValues.Set(settings.CatalogueDefault, key, settings.Value{Number: 0}); err != nil {
				t.Fatal(err)
			}
		}
		v.applySettings()
		if err := renderViewPNG(filepath.Join(dir, "settings-padding-"+group+"-zero.png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
