package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

const longScrollingFilename = "2026-10-31 -- annual budget review -- 日本語 café -- final report.txt"

func filenameScrollView() *view {
	v := newPreviewView()
	for i := range v.panels {
		p := &v.panels[i]
		row := p.selected
		oldName := p.items[row].file.Name
		p.items[row].name, p.items[row].file.Name = longScrollingFilename, longScrollingFilename
		p.items[row].file.Directory = false
		for j := range p.entries {
			if p.entries[j].Name == oldName {
				p.entries[j] = p.items[row].file
			}
		}
	}
	return v
}

func TestFilenameScrollTracksCurrentRowAndPanel(t *testing.T) {
	v := filenameScrollView()
	send, advance := panelInputFrames(v)
	p := &v.panels[0]
	if p.filenameScroll.content != longScrollingFilename || p.filenameScroll.fullWidth <= p.filenameScroll.width || !v.panels[1].filenameScroll.start.IsZero() {
		t.Fatal("only the active current filename should scroll")
	}
	start := p.filenameScroll.start
	advance(3 * time.Second)
	p.toggleMark(p.selected + 1)
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(100, 180)})
	if p.filenameScroll.start != start || p.filenameScroll.content != longScrollingFilename {
		t.Fatal("hover or marks changed the scrolling filename")
	}
	send(pressed(key.NameDownArrow))
	if !p.filenameScroll.start.After(start) || p.filenameScroll.content == longScrollingFilename {
		t.Fatal("changing the current row did not reset its scroll")
	}
	send(pressed(key.NameTab))
	if p.filenameScroll != (textMarquee{}) || v.panels[1].filenameScroll.content != longScrollingFilename {
		t.Fatal("panel switching did not transfer and reset filename animation")
	}
	p = &v.panels[1]
	start = p.filenameScroll.start
	advance(time.Second)
	p.path += string(filepath.Separator) + "other"
	send()
	if !p.filenameScroll.start.After(start) {
		t.Fatal("a different folder with the same filename retained its animation")
	}
	if clone := p.clone(); clone.filenameScroll != (textMarquee{}) || clone.filenameScrollID != "" {
		t.Fatal("cloning copied a running filename animation")
	}
	start = p.filenameScroll.start
	advance(time.Second)
	send(modified("W", key.ModCtrl))
	send(modified("W", key.ModCtrl|key.ModShift))
	if !v.panels[v.active].filenameScroll.start.After(start) {
		t.Fatal("reopening a closed panel resumed its old filename animation")
	}
}

func TestFilenameScrollResumesWithAppliedFilter(t *testing.T) {
	v := filenameScrollView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "/"})
	send(key.EditEvent{Text: "report"})
	if v.panels[0].filenameScroll != (textMarquee{}) {
		t.Fatal("filter editing left a filename scrolling")
	}
	send(pressed(key.NameReturn))
	if v.panels[0].filter != "report" || !v.interaction.idle() || v.panels[0].filenameScroll.content != longScrollingFilename {
		t.Fatal("an applied filter prevented focused filename scrolling")
	}
}

func TestFilenameScrollStopsBehindInteractionsAndRestartsDelay(t *testing.T) {
	for _, owner := range []string{"palette", "settings", "filename settings", "filter", "location", "rename"} {
		t.Run(owner, func(t *testing.T) {
			v := filenameScrollView()
			send, advance := panelInputFrames(v)
			p := &v.panels[0]
			advance(3 * time.Second)
			start := p.filenameScroll.start
			switch owner {
			case "palette":
				send(pressed("`"))
			case "settings":
				v.startSettingsWidget("appearance.autocolor")
			case "filename settings":
				v.startSettingsWidget(settingFilenameScroll)
			case "filter":
				send(key.EditEvent{Text: "/"})
			case "location":
				send(modified("D", key.ModAlt))
			case "rename":
				send(pressed(key.NameF2))
			}
			send()
			if v.interaction.idle() || p.filenameScroll != (textMarquee{}) {
				t.Fatal("interaction left the filename scrolling")
			}
			advance(10 * time.Second)
			if p.filenameScroll != (textMarquee{}) {
				t.Fatal("filename scrolling restarted behind the interaction")
			}
			send(pressed(key.NameEscape))
			if !v.interaction.idle() || !p.filenameScroll.start.After(start) {
				t.Fatal("returning to browsing did not start a fresh scrolling delay")
			}
			start = p.filenameScroll.start
			advance(textScrollDelay - time.Millisecond)
			if p.filenameScroll.start != start {
				t.Fatal("resumed scrolling restarted on each frame")
			}
		})
	}
}

func TestFilenameScrollFocusSchedulingAndClipping(t *testing.T) {
	v := filenameScrollView()
	// Keep the test limited to the filename and its metadata: no location or
	// help animation can account for a scheduled wake-up or changed pixels.
	p := &v.panels[0]
	item := p.items[p.selected]
	item.size = "1 KB"
	var router input.Router
	var ops op.Ops
	now := time.Unix(1000, 0)
	var window *headless.Window
	if os.Getenv("COMMANDO_PREVIEW_FILENAMES") != "" {
		var err error
		window, err = headless.NewWindow(240, 34)
		if err != nil {
			t.Fatal(err)
		}
		defer window.Release()
	}
	frame := func() bool {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(240, 34))}
		event.Op(&ops, &v.focus)
		for {
			if _, ok := gtx.Event(key.FocusFilter{Target: &v.focus}); !ok {
				break
			}
		}
		var scroll *textMarquee
		if v.prepareFilenameScroll(gtx, 0) {
			scroll = &p.filenameScroll
		}
		bg, fg, metadata := v.appearance.entryColors(item.file, true, false)
		v.fileRowSurface(gtx, image.Pt(240, 34), bg, v.appearance.Colors.Edge, func(gtx layout.Context) layout.Dimensions {
			return v.rowContents(gtx, item, fg, metadata, metadataSize, fileTextRoles{contrastPrimary, contrastPrimary}, false, scroll)
		})
		router.Frame(&ops)
		_, wake := router.WakeupTime()
		return wake
	}
	capture := func() *image.RGBA {
		if err := window.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 240, 34))
		if err := window.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		return img
	}
	frame()
	for _, tc := range []struct {
		name   string
		stop   func()
		ticker bool
	}{
		{"keyboard focus lost", func() { router.Source().Execute(key.FocusCmd{}) }, false},
		{"inactive panel", func() { v.active = 1 }, false},
		{"loading", func() { p.loading = true }, false},
		{"disabled", func() {
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingFilenameScrollEnabled, settings.Value{Bool: false})
		}, false},
		{"native dialog", func() { v.interaction.current = &nativeSession{} }, false},
		{"ticker", func() { router.Source().Execute(key.FocusCmd{}) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v.active, v.interaction.current, p.loading = 0, nil, false
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingFilenameScrollEnabled, settings.Value{Bool: true})
			if tc.ticker {
				_ = v.settingsValues.Set(settings.CatalogueDefault, settingFilenameScrollModel, settings.Value{Text: "Ticker"})
			}
			router.Source().Execute(key.FocusCmd{Tag: &v.focus})
			frame()
			if !frame() || p.filenameScroll.start.IsZero() {
				t.Fatal("focused browsing did not schedule filename scrolling")
			}
			var initial *image.RGBA
			if window != nil {
				initial = capture()
			}
			now = now.Add(3 * time.Second)
			frame()
			if window != nil {
				moved := capture()
				if bytes.Equal(initial.Pix, moved.Pix) {
					t.Fatal("filename did not visibly scroll")
				}
				for y := range 34 {
					for _, span := range [][2]int{{0, 14}, {200, 240}} {
						from, to := initial.PixOffset(span[0], y), initial.PixOffset(span[1], y)
						if !bytes.Equal(initial.Pix[from:to], moved.Pix[from:to]) {
							t.Fatal("scrolling painted over the mark, padding or metadata")
						}
					}
				}
			}
			tc.stop()
			frame()
			for range 3 {
				now = now.Add(time.Second)
				if frame() || p.filenameScroll != (textMarquee{}) {
					t.Fatal("stopped filename requested animation frames or retained its scroll")
				}
			}
		})
	}
}

func TestFilenameScrollPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_FILENAMES")
	if dir == "" {
		t.Skip("optional focused filename animation previews")
	}
	for _, model := range []string{"Ping Pong", "Ticker"} {
		v := filenameScrollView()
		_ = v.settingsValues.Set(settings.CatalogueDefault, settingFilenameScrollModel, settings.Value{Text: model})
		window, err := headless.NewWindow(800, 480)
		if err != nil {
			t.Fatal(err)
		}
		var router input.Router
		var ops op.Ops
		now := time.Now()
		frame := func() {
			ops.Reset()
			v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now,
				Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))})
			router.Frame(&ops)
			if err := window.Frame(&ops); err != nil {
				t.Fatal(err)
			}
		}
		save := func(phase string) {
			img := image.NewRGBA(image.Rect(0, 0, 800, 480))
			if err := window.Screenshot(img); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(dir, "filenames-"+model+"-"+phase+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, img)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				t.Fatal("cannot save preview", err, closeErr)
			}
		}
		frame()
		frame()
		save("start")
		now = now.Add(3 * time.Second)
		frame()
		save("moving")
		v.startSettingsWidget(settingFilenameScroll)
		frame()
		frame()
		if v.panels[0].filenameScroll != (textMarquee{}) {
			t.Fatal("settings preview did not stop the filename")
		}
		save("settings")
		window.Release()
	}
}
