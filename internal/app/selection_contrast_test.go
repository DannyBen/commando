package app

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

func TestSelectionContrastChangesBackgroundMinimally(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		text                 uint32
		surfaceL, selectionL float64
		wantLighten          bool
	}{
		{"black text", 0, 0.95, 0.3, true},
		{"white text", 0xffffff, 0.12, 0.85, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := rgb(tc.text)
			surface := (oklchColor{l: tc.surfaceL}).rgb()
			selection := (oklchColor{tc.selectionL, 0.06, 260}).rgb()
			rule := themeContrastRules[contrastTextSelection]
			got := contrastColorOn(text, selection, surface, rule)
			if math.Abs(perceptualContrast(text, got)) < rule.targetLc {
				t.Fatal("selection missed a reachable contrast target")
			}
			before, after := oklchFromRGB(selection), oklchFromRGB(got)
			if (after.l > before.l) != tc.wantLighten || got == rgb(0) || got == rgb(0xffffff) {
				t.Fatal("selection did not preserve its color in the correct direction")
			}
			if hue := min(wrapHue(before.h-after.h), wrapHue(after.h-before.h)); hue > 2 {
				t.Fatalf("selection hue changed by %.3f degrees", hue)
			}
			closer := before
			closer.l += (after.l - before.l) * 0.9
			if math.Abs(perceptualContrast(text, closer.rgb())) >= rule.targetLc {
				t.Fatal("selection correction overshot a closer passing color")
			}
			if contrastColorOn(text, got, surface, rule) != got {
				t.Fatal("a passing selection was adjusted again")
			}
		})
	}
}

func TestSelectionContrastStopsAtVisibilityGap(t *testing.T) {
	text := rgb(0)
	surface := (oklchColor{l: 0.70}).rgb()
	selection := (oklchColor{0.30, 0.06, 260}).rgb()
	rule := themeContrastRules[contrastTextSelection]
	got := contrastColorOn(text, selection, surface, rule)
	gap := (oklchFromRGB(surface).l - oklchFromRGB(got).l) * 100
	if gap < rule.minLightnessGap || gap > rule.minLightnessGap+0.4 {
		t.Fatalf("correction did not stop at the visibility limit: %.4f points", gap)
	}
	if score := math.Abs(perceptualContrast(text, got)); score >= rule.targetLc || score <= math.Abs(perceptualContrast(text, selection)) {
		t.Fatal("fixture should improve contrast without reaching the target")
	}
	if got == rgb(0) || got == rgb(0xffffff) {
		t.Fatal("unreachable selection fell back to black/white")
	}
}

func TestSelectionContrastKeepsReadableColorsAndRepairsInvisibleSelections(t *testing.T) {
	rule := themeContrastRules[contrastTextSelection]
	for _, tc := range []struct {
		name                       string
		text, surface, selectColor uint32
		unchanged                  bool
	}{
		{"readable light", 0, 0xffffff, 0xccccff, true},
		{"readable dark", 0xffffff, 0, 0x223344, true},
		{"identical light", 0, 0xffffff, 0xffffff, false},
		{"identical dark", 0xffffff, 0, 0, false},
		{"identical midtone", 0, 0x777777, 0x777777, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, selection, surface := rgb(tc.text), rgb(tc.selectColor), rgb(tc.surface)
			got := contrastColorOn(text, selection, surface, rule)
			if (got == selection) != tc.unchanged {
				t.Fatalf("unexpected selection correction: %v", got)
			}
			if gap := math.Abs(oklchFromRGB(got).l-oklchFromRGB(surface).l) * 100; gap < rule.minLightnessGap {
				t.Fatalf("selection became indistinguishable from surface: %.4f", gap)
			}
			if !tc.unchanged && (got == rgb(0) || got == rgb(0xffffff)) {
				t.Fatal("visibility repair discarded selection color for an extreme")
			}
		})
	}
}

func TestSelectionCacheIncludesEditorSurfaceAndRule(t *testing.T) {
	var cache textContrastCache
	text, selection := rgb(0), (oklchColor{0.3, 0.06, 260}).rgb()
	light, mid := (oklchColor{l: 0.95}).rgb(), (oklchColor{l: 0.70}).rgb()
	a := cache.resolveOn(text, selection, light, contrastTextSelection)
	b := cache.resolveOn(text, selection, mid, contrastTextSelection)
	if a == b || len(cache) != 2 {
		t.Fatal("selection resolution ignored the ordinary surface")
	}
	if cache.resolveOn(text, selection, light, contrastTextSelection) != a || len(cache) != 2 {
		t.Fatal("identical selection was not reused")
	}
	rule := themeContrastRules[contrastTextSelection]
	defer func() { themeContrastRules[contrastTextSelection] = rule }()
	changed := rule
	changed.targetLc = 50
	themeContrastRules[contrastTextSelection] = changed
	if cache.resolveOn(text, selection, light, contrastTextSelection) == a || len(cache) != 3 {
		t.Fatal("selection cache ignored the changed target")
	}
}

func TestSelectionPreviewMatchesEditorWithoutChangingItsState(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	c := &v.appearance.Colors
	c.EditorText, c.TextSelection = rgb(0xffff00), rgb(0xffff00)
	c.Surface = rgb(0)
	var editor widget.Editor
	editor.SetText("Example selection")
	editor.SetCaret(3, 12)
	start, end := editor.Selection()
	style := v.textEditor(&editor, "")
	sample := v.settingsPreviewItems("text_selection")[0]
	if style.Color != c.EditorText || style.SelectionColor == c.TextSelection || sample.foreground != style.Color || sample.background != style.SelectionColor {
		t.Fatal("preview and editor do not share selection correction")
	}
	a, b := editor.Selection()
	if a != start || b != end || editor.Text() != "Example selection" || c.TextSelection != rgb(0xffff00) {
		t.Fatal("selection styling mutated editor or palette")
	}
	if dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS"); dir != "" {
		v.startSettingsWidget(settingThemeGenerator)
		if err := renderViewPNG(filepath.Join(dir, "text-selection-contrast.png"), v); err != nil {
			t.Fatal(err)
		}
	}
	// The passive panel sample must resolve the same actual text/background pair.
	var ops op.Ops
	v.themePreviewSelection(layout.Context{Ops: &ops, Constraints: layout.Exact(image.Pt(300, 40))})
	if len(v.textContrast) == 0 {
		t.Fatal("selection sample did not resolve its display colors")
	}
}

func TestSelectionContrastAcrossGeneratedPalettes(t *testing.T) {
	rule := themeContrastRules[contrastTextSelection]
	for _, mode := range []string{"Dark", "Midtone", "Light"} {
		for seed := uint64(1); seed <= 40; seed++ {
			colors := defaultThemeRecipe.generate(themeGeneration{mode, seed, 100, 100, 96}, nil)
			var cache textContrastCache
			for _, name := range []string{"background", "alt_background", "selection"} {
				surface := colors[name].rgb()
				text := cache.resolve(colors["text"].rgb(), surface, contrastPrimary)
				selection := colors["text_selection"].rgb()
				got := cache.resolveOn(text, selection, surface, contrastTextSelection)
				gap := math.Abs(oklchFromRGB(got).l-oklchFromRGB(surface).l) * 100
				if gap < rule.minLightnessGap {
					t.Fatalf("%s seed %d on %s lost its visibility gap: %.4f", mode, seed, name, gap)
				}
				originalGap := math.Abs(oklchFromRGB(selection).l-oklchFromRGB(surface).l) * 100
				originalScore := math.Abs(perceptualContrast(text, selection))
				if originalGap >= rule.minLightnessGap {
					if originalScore >= rule.targetLc && got != selection {
						t.Fatal("already readable and visible selection was changed")
					}
					if math.Abs(perceptualContrast(text, got)) < originalScore {
						t.Fatal("selection correction reduced contrast")
					}
				}
			}
		}
	}
}
