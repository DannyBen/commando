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

func TestNotificationContrastPreservesMoreColorThanPrimaryText(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.appearance.Colors.Footer = rgb(0)
	v.appearance.Colors.Error = rgb(0xff0000)
	got := v.feedbackColor(feedback.Error)
	primary := v.resolvedTextFor(contrastPrimary, v.appearance.Colors.Error, v.appearance.Colors.Footer)
	if math.Abs(perceptualContrast(got, v.appearance.Colors.Footer)) < themeContrastRules[contrastNotification].targetLc {
		t.Fatal("notification lost its minimum contrast")
	}
	if oklchFromRGB(got).c <= oklchFromRGB(primary).c {
		t.Fatal("notification correction did not retain more red chroma than ordinary text")
	}
}

func TestNotificationTextAndEdgeShareDisplayColor(t *testing.T) {
	for _, background := range []uint32{0, 0x888888, 0xffffff} {
		for _, guarded := range []bool{false, true} {
			v := newPreviewView()
			v.applySettings()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingContrastGuard, settings.Value{Bool: guarded})
			v.appearance.Colors.Footer = rgb(background)
			v.appearance.Colors.Info, v.appearance.Colors.Success = rgb(0x00aaff), rgb(0x00ff00)
			v.appearance.Colors.Warning, v.appearance.Colors.Error = rgb(0xffff00), rgb(0xff0000)
			palette := v.appearance.Colors
			for kind, raw := range map[feedback.Kind]color.NRGBA{
				feedback.Info: palette.Info, feedback.Success: palette.Success,
				feedback.Warning: palette.Warning, feedback.Error: palette.Error,
			} {
				v.textContrast = nil
				displayed := v.feedbackColor(kind) // The line uses this color directly.
				if !guarded && displayed != raw {
					t.Fatal("disabled guard changed notification color")
				}
				if guarded && math.Abs(perceptualContrast(displayed, palette.Footer)) < themeContrastRules[contrastNotification].targetLc {
					t.Fatal("notification was not readable against its footer")
				}
				v.panels[0].feedback.ShowWithStyle(kind, "Notification", feedback.Immediate)
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(300, 30))}
				v.footerContents(gtx, 0)
				(panelPreview{messages: []feedback.Kind{kind}}).footer(v, gtx, 30)
				// A second pass through ordinary label contrast would recolor the
				// text after resolving the edge, recreating the reported mismatch.
				// The persistent counter also resolves its secondary text color.
				if guarded && len(v.textContrast) != 2 {
					t.Fatal("real or preview notification applied a second contrast policy")
				}
				if v.appearance.Colors != palette {
					t.Fatal("display correction modified the theme palette")
				}
			}
		}
	}
}
