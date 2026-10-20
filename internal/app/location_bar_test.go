package app

import (
	"bytes"
	"fmt"
	"image"
	"maps"
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
)

func TestLocationMarqueeOnlyActiveAndEditorKeepsFullPath(t *testing.T) {
	v := newPreviewView()
	path := `W:\go\commando\日本語\café\a very long folder\internal\app\themes`
	for pi := range v.panels {
		v.panels[pi].path = path
	}
	send, advance := panelInputFrames(v)
	if v.panels[0].locationScroll.fullWidth <= v.panels[0].locationScroll.width {
		t.Fatal("test path does not overflow")
	}
	start := v.panels[0].locationScroll.start
	advance(3 * time.Second)
	if v.panels[0].locationScroll.start != start || !v.panels[1].locationScroll.start.IsZero() {
		t.Fatal("animation restarted or inactive panel animated")
	}
	send(pressed(key.NameTab))
	if !v.panels[0].locationScroll.start.IsZero() || !v.panels[1].locationScroll.start.After(start) {
		t.Fatal("focus did not move and restart animation")
	}
	send(modified("D", key.ModAlt))
	if v.location.Text() != path || !v.panels[1].locationScroll.start.IsZero() {
		t.Fatal("scrolling altered the editable path or continued in editor")
	}
}

func TestLocationMarqueeStopsForPaletteAndSettings(t *testing.T) {
	for _, settings := range []bool{false, true} {
		t.Run(fmt.Sprintf("settings=%v", settings), func(t *testing.T) {
			v := newPreviewView()
			v.panels[0].path = `W:\go\commando\日本語\a very long folder\internal\app\themes`
			send, advance := panelInputFrames(v)
			p := &v.panels[0]
			if p.locationScroll.fullWidth <= p.locationScroll.width {
				t.Fatal("test path does not overflow")
			}
			advance(3 * time.Second)
			before := p.locationScroll.start
			if settings {
				openSettingsGroup(t, v, "appearance.autocolor")
				send()
			} else {
				send(pressed("`"))
			}
			if settings && v.settingsWidget() == nil || !settings && v.paletteInteraction() == nil {
				t.Fatal("test did not open the requested interaction")
			}
			if p.locationScroll != (textMarquee{}) {
				t.Fatal("opening the interaction retained location animation")
			}
			advance(10 * time.Second)
			if p.locationScroll != (textMarquee{}) {
				t.Fatal("location animation restarted behind the interaction")
			}
			send(pressed(key.NameEscape))
			if !v.interaction.idle() || !p.locationScroll.start.After(before) {
				t.Fatal("closing the interaction did not restart normal location scrolling")
			}
			start := p.locationScroll.start
			advance(time.Second)
			if p.locationScroll.start != start {
				t.Fatal("resumed animation restarted on subsequent frames")
			}
		})
	}
}

func TestLocationMarqueeRequiresFocusedBrowsingPanel(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[0]
	p.path = `W:\go\commando\a very long folder\internal\app\themes`
	var router input.Router
	var ops op.Ops
	var capture func() []byte
	if os.Getenv("COMMANDO_PREVIEW_LOCATION_SCROLL") != "" {
		w, err := headless.NewWindow(240, 40)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Release()
		capture = func() []byte {
			if err := w.Frame(&ops); err != nil {
				t.Fatal(err)
			}
			img := image.NewRGBA(image.Rect(0, 0, 240, 40))
			if err := w.Screenshot(img); err != nil {
				t.Fatal(err)
			}
			return img.Pix
		}
	}
	now := time.Unix(1000, 0)
	frame := func() bool {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(240, 40))}
		event.Op(&ops, &v.focus)
		for {
			if _, ok := gtx.Event(key.FocusFilter{Target: &v.focus}); !ok {
				break
			}
		}
		v.layoutLocationBar(gtx, 0, 40)
		router.Frame(&ops)
		_, wake := router.WakeupTime()
		return wake
	}
	frame()
	for _, tc := range []struct {
		name    string
		focused bool
		active  int
		owner   interactionSession
	}{
		{name: "no keyboard focus", active: 0},
		{name: "inactive panel", focused: true, active: 1},
		{name: "palette", focused: true, owner: &paletteSession{}},
		{name: "settings", focused: true, owner: &settingsSession{}},
		{name: "name editor", focused: true, owner: &nameSession{}},
		{name: "filter editor", focused: true, owner: &filterSession{}},
		{name: "native menu", focused: true, owner: &nativeSession{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v.active, v.interaction.current = 0, nil
			router.Source().Execute(key.FocusCmd{Tag: &v.focus})
			frame() // Drain focus-related invalidation before checking animation.
			var initial []byte
			if capture != nil {
				initial = capture()
			}
			now = now.Add(3 * time.Second)
			if !frame() || p.locationScroll.start.IsZero() {
				t.Fatal("focused browsing did not animate")
			}
			if capture != nil && bytes.Equal(initial, capture()) {
				t.Fatal("focused header did not visibly scroll")
			}
			v.active, v.interaction.current = tc.active, tc.owner
			if !tc.focused {
				router.Source().Execute(key.FocusCmd{})
			}
			frame()
			var stopped []byte
			if capture != nil {
				stopped = capture()
			}
			for range 3 {
				now = now.Add(time.Second)
				if frame() || p.locationScroll != (textMarquee{}) {
					t.Fatal("unfocused panel retained animation or requested another frame")
				}
				if capture != nil && !bytes.Equal(stopped, capture()) {
					t.Fatal("stopped header changed pixels across frames")
				}
			}
		})
	}
}

func TestLocationMarqueePreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_LOCATION_SCROLL")
	if directory == "" {
		t.Skip("optional location marquee preview")
	}
	v := newPreviewView()
	for pi := range v.panels {
		v.panels[pi].path = `W:\go\commando\日本語\café\internal\app\themes`
	}
	if err := renderViewPNG(filepath.Join(directory, "location-scroll-start.png"), v); err != nil {
		t.Fatal(err)
	}
	scroll := v.panels[0].locationScroll
	travel := time.Duration(float64(scroll.fullWidth-scroll.width) * float64(time.Second) / float64(textScrollSpeed))
	for _, phase := range []struct {
		name    string
		elapsed time.Duration
	}{
		{"middle", textScrollDelay + travel/2},
		{"end", textScrollDelay + travel + textScrollEndPause/2},
		{"return", textScrollDelay + travel + textScrollEndPause + travel/2},
		{"beginning", textScrollDelay + 2*travel + textScrollEndPause + textScrollStartPause/2},
	} {
		v.panels[0].locationScroll.start = time.Now().Add(-phase.elapsed)
		if err := renderViewPNG(filepath.Join(directory, "location-scroll-"+phase.name+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocationBarClicksFocusThenEdit(t *testing.T) {
	v := selectionView()
	v.panels[1].marks = map[int]bool{2: true, 4: true}
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	p := &v.panels[1]
	selected, position, marks := p.selected, p.list.Position, maps.Clone(p.marks)
	point := f32.Pt(450, 18)
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: point})
	if !p.locationBar.clickable.Hovered() {
		t.Fatal("location bar did not hover")
	}
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	if !p.locationBar.clickable.Pressed() || v.active != 0 {
		t.Fatal("press did not show feedback or activated before release")
	}
	send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point, Time: time.Second + time.Millisecond})
	if v.active != 1 || v.locationEditing() || !router.Source().Focused(&v.focus) {
		t.Fatal("single click did not focus panel")
	}
	if p.selected != selected || !maps.Equal(p.marks, marks) || p.list.Position.First != position.First || p.list.Position.Offset != position.Offset {
		t.Fatal("location click changed selection, marks or viewport")
	}
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	if !v.locationEditing() || v.location.Text() != p.locationPath() || v.location.SelectedText() != p.locationPath() || !router.Source().Focused(&v.location) {
		t.Fatal("double click did not open the existing path editor")
	}
	rowClick(send, point, 2*time.Second, 0)
	if !v.locationEditing() || v.location.SelectedText() != "" {
		t.Fatal("click in open editor did not place native caret")
	}
	send(pressed(key.NameEscape))
	if v.locationEditing() || v.active != 1 || !router.Source().Focused(&v.focus) || p.selected != selected || !maps.Equal(p.marks, marks) {
		t.Fatal("path cancellation did not restore browsing state and focus")
	}
}

func TestLocationBarHonorsEditorCancellationAndPendingMutation(t *testing.T) {
	for _, pending := range []bool{false, true} {
		v := selectionView()
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF2))
		s := v.nameInteraction()
		if s == nil {
			t.Fatal("missing name editor")
		}
		s.pending = pending
		rowClick(send, f32.Pt(450, 18), time.Second, 0)
		if pending {
			if v.active != 0 || v.nameInteraction() != s {
				t.Fatal("header click interrupted submitted name mutation")
			}
		} else if v.active != 1 || v.nameInteraction() != nil || v.panels[0].selected != 1 {
			t.Fatal("header click did not use normal editor cancellation")
		}
	}
}

func TestLocationBarCannotClickThroughPalette(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	rowClick(send, f32.Pt(450, 18), time.Second, 0)
	send()
	if v.paletteInteraction() != nil || v.active != 0 || v.locationEditing() {
		t.Fatal("palette dismissal clicked through to location bar")
	}
	rowClick(send, f32.Pt(450, 18), 2*time.Second, 0)
	if v.active != 1 || v.locationEditing() {
		t.Fatal("fresh header click failed after palette dismissal")
	}
}
