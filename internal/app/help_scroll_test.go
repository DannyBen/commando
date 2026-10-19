package app

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func TestHelpMarqueeResetsOnResizeFontBindingAndVisibility(t *testing.T) {
	v := newPreviewView()
	var ops op.Ops
	var router input.Router
	start := time.Unix(1000, 0)
	frame := func(elapsed time.Duration, width int) layout.Context {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Now: start.Add(elapsed), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(width, 200)}}
		v.layoutHelpFooter(gtx)
		router.Frame(&ops)
		return gtx
	}
	frame(0, 240)
	for _, line := range v.helpScroll {
		if line.fullWidth <= line.width || line.width <= 0 {
			t.Fatal("help was truncated before measuring the complete line", line)
		}
	}
	if !strings.Contains(v.helpScroll[0].content, "[F1]") || !strings.Contains(v.helpScroll[1].content, "[`]") {
		t.Fatal("marquee did not resolve styled keys")
	}
	gtx := frame(3*time.Second, 240)
	for i := range v.helpScroll {
		line := &v.helpScroll[i]
		if line.start != start || math.Abs(float64(line.offset(gtx, line.content, line.width, line.fullWidth)-60)) > 0.01 {
			t.Fatal("steady frames restarted scrolling or used different timing", i, line)
		}
	}
	frame(4*time.Second, 800)
	if _, wake := router.WakeupTime(); wake {
		t.Fatal("fitting help kept requesting animation frames")
	}
	frame(7*time.Second, 240)
	if v.helpScroll[0].start != start.Add(7*time.Second) {
		t.Fatal("narrowing the window did not restart the reading pause")
	}
	v.appearance.TextSize += 3
	frame(8*time.Second, 240)
	if v.helpScroll[0].start != start.Add(8*time.Second) {
		t.Fatal("font size change did not restart scrolling")
	}
	if err := v.settingsValues.Set(bindingSettingsCatalogue(), bindingSettingKey(helpCommandID, 0), settings.Value{Text: "F9"}); err != nil {
		t.Fatal(err)
	}
	frame(9*time.Second, 240)
	if v.helpScroll[0].start != start.Add(9*time.Second) || !strings.Contains(v.helpScroll[0].content, "[F9]") || v.helpScroll[1].start != start.Add(8*time.Second) {
		t.Fatal("binding change did not restart just the affected line")
	}
	v.helpHidden = true
	frame(10*time.Second, 240)
	if v.helpScrollID != "" || v.helpScroll != [2]textMarquee{} {
		t.Fatal("hidden help retained scrolling state")
	}
	if _, wake := router.WakeupTime(); wake {
		t.Fatal("hidden help kept requesting animation frames")
	}
	v.helpHidden = false
	frame(20*time.Second, 240)
	if v.helpScroll[0].start != start.Add(20*time.Second) {
		t.Fatal("reopening help skipped the initial reading pause")
	}
}

func TestHelpMarqueeOnlyOverflowingLinesMoveAndContextResets(t *testing.T) {
	v := newPreviewView()
	v.openPaletteQuery(layout.Context{}, "no such command")
	var ops op.Ops
	start := time.Unix(1000, 0)
	gtx := layout.Context{Ops: &ops, Now: start, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(280, 200)}}
	v.layoutHelpFooter(gtx)
	if v.helpScroll[0].fullWidth > v.helpScroll[0].width || v.helpScroll[1].fullWidth <= v.helpScroll[1].width {
		t.Fatal("test requires one fitting and one overflowing line")
	}
	gtx.Now = start.Add(3 * time.Second)
	ops.Reset()
	v.layoutHelpFooter(gtx)
	for i := range v.helpScroll {
		line := &v.helpScroll[i]
		offset := line.offset(gtx, line.content, line.width, line.fullWidth)
		if i == 0 && offset != 0 || i == 1 && offset <= 0 {
			t.Fatal("fitting and overflowing lines did not behave independently", i, offset)
		}
	}
	v.openPaletteQuery(layout.Context{}, "@generator")
	gtx.Now = start.Add(4 * time.Second)
	ops.Reset()
	v.layoutHelpFooter(gtx)
	if v.helpScrollID != "palette.settings."+settingThemeGenerator || v.helpScroll[0].start != gtx.Now || !v.helpScroll[1].start.IsZero() {
		t.Fatal("changing to a one-line tip retained the previous scroll")
	}
}

func TestHelpMarqueePreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_HELP_SCROLL")
	if dir == "" {
		t.Skip("optional help marquee previews")
	}
	v := newPreviewView()
	v.startSettingsWidget(settingThemeGenerator)
	v.settingsInteraction().selected = 3
	size := image.Pt(360, 480)
	if err := renderSizedViewPNG(filepath.Join(dir, "help-scroll-start.png"), v, size); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []struct {
		name string
		end  bool
	}{{"middle", false}, {"end", true}} {
		for i := range v.helpScroll {
			line := &v.helpScroll[i]
			travel := time.Duration(float64(line.fullWidth-line.width) * float64(time.Second) / float64(textScrollSpeed))
			elapsed := textScrollDelay + travel/2
			if phase.end {
				elapsed = textScrollDelay + travel + textScrollEndPause/2
			}
			line.start = time.Now().Add(-elapsed)
		}
		if err := renderSizedViewPNG(filepath.Join(dir, "help-scroll-"+phase.name+".png"), v, size); err != nil {
			t.Fatal(err)
		}
	}
}
