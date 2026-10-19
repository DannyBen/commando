package app

import (
	"image"
	"maps"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
)

func TestLocationMarqueePingPongPausesAndResets(t *testing.T) {
	start := time.Unix(1000, 0)
	gtx := layout.Context{Now: start, Metric: unit.Metric{PxPerDp: 1}}
	var scroll locationMarquee
	for _, tc := range []struct {
		elapsed time.Duration
		want    float32
	}{
		{0, 0}, {locationScrollDelay - time.Millisecond, 0},
		{locationScrollDelay + time.Second, 30},
		{locationScrollDelay + 3*time.Second, 90},
		{locationScrollDelay + 7*time.Second, 90},
		{locationScrollDelay + 8*time.Second, 90},
		{locationScrollDelay + 9*time.Second, 60},
		{locationScrollDelay + 11*time.Second, 0},
		{locationScrollDelay + 20*time.Second, 0},
		{locationScrollDelay + 21*time.Second, 0},
		{locationScrollDelay + 22*time.Second, 30},
	} {
		gtx.Now = start.Add(tc.elapsed)
		if got := scroll.offset(gtx, "full Unicode path 日本語/é", 100, 190); math.Abs(float64(got-tc.want)) > 0.001 {
			t.Fatalf("at %s offset=%v, want %v", tc.elapsed, got, tc.want)
		}
	}
	if got := scroll.offset(gtx, "another path", 100, 200); got != 0 {
		t.Fatal("new location did not restart")
	}
	gtx.Now = gtx.Now.Add(time.Minute)
	if got := scroll.offset(gtx, "another path", 300, 200); got != 0 {
		t.Fatal("fitting location scrolled")
	}
	if got := scroll.offset(gtx, "another path", 50, 200); got != 0 {
		t.Fatal("resizing did not restart")
	}
}

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
	travel := time.Duration(float64(scroll.fullWidth-scroll.width) * float64(time.Second) / float64(locationScrollSpeed))
	for _, phase := range []struct {
		name    string
		elapsed time.Duration
	}{
		{"middle", locationScrollDelay + travel/2},
		{"end", locationScrollDelay + travel + locationScrollEndPause/2},
		{"return", locationScrollDelay + travel + locationScrollEndPause + travel/2},
		{"beginning", locationScrollDelay + 2*travel + locationScrollEndPause + locationScrollStartPause/2},
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
