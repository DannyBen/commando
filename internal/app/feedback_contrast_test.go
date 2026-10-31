package app

import (
	"image"
	"image/color"
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func TestNotificationFooterUsesIndependentOffsetAndReadableText(t *testing.T) {
	for _, theme := range builtInThemes {
		for _, guarded := range []bool{false, true} {
			v := newPreviewView()
			v.settingsValues = theme.values.Clone()
			v.applySettings()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingContrastGuard, settings.Value{Bool: guarded})
			palette := v.appearance.Colors
			for kind, raw := range map[feedback.Kind]color.NRGBA{
				feedback.Info: palette.Info, feedback.Success: palette.Success,
				feedback.Warning: palette.Warning, feedback.Error: palette.Error,
			} {
				background, foreground := v.notificationColors(kind)
				value := oklchFromRGB(raw)
				value.l = clampLightness(value.l + v.settingsValues.Get(settings.CatalogueDefault, settingFooterOffset).Number/100)
				wantBackground, wantText := value.rgb(), v.appearance.Colors.Surface
				if background != wantBackground || foreground != wantText {
					t.Fatal("notification ignored its semantic color or footer offset")
				}
				displayed := v.resolvedTextFor(contrastPrimary, foreground, background)
				if !guarded && displayed != foreground {
					t.Fatal("disabled guard changed notification text")
				}
				// Midtone backgrounds may not reach the target even with black or
				// white text; the shared guard then keeps the strongest contrast.
				available := math.Max(math.Abs(perceptualContrast(rgb(0), background)), math.Abs(perceptualContrast(rgb(0xffffff), background)))
				target := math.Min(themeContrastRules[contrastPrimary].targetLc, available)
				if guarded && math.Abs(perceptualContrast(displayed, background))+0.01 < target {
					t.Fatalf("%s kind %v: notification text did not receive the available contrast", theme.name, kind)
				}
				v.panels[0].feedback.ShowWithStyle(kind, "Notification", feedback.Immediate)
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(300, 30))}
				v.panelFooter(gtx, 0, 30)
				(panelPreview{messages: []feedback.Kind{kind}}).footer(v, gtx, 30)
				if v.appearance.Colors != palette {
					t.Fatal("display correction modified the saved palette")
				}
			}
		}
	}
}
