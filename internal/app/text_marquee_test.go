package app

import (
	"math"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
)

func TestTextMarqueePingPongPausesAndResets(t *testing.T) {
	start := time.Unix(1000, 0)
	gtx := layout.Context{Now: start, Metric: unit.Metric{PxPerDp: 1}}
	var scroll textMarquee
	for _, tc := range []struct {
		elapsed time.Duration
		want    float32
	}{
		{0, 0}, {textScrollDelay - time.Millisecond, 0},
		{textScrollDelay + time.Second, 30},
		{textScrollDelay + 3*time.Second, 90},
		{textScrollDelay + 7*time.Second, 90},
		{textScrollDelay + 8*time.Second, 90},
		{textScrollDelay + 9*time.Second, 60},
		{textScrollDelay + 11*time.Second, 0},
		{textScrollDelay + 20*time.Second, 0},
		{textScrollDelay + 21*time.Second, 0},
		{textScrollDelay + 22*time.Second, 30},
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

func TestTextMarqueeTickerWrapsWithoutEndPauses(t *testing.T) {
	start := time.Unix(1000, 0)
	gtx := layout.Context{Now: start, Metric: unit.Metric{PxPerDp: 1}}
	var scroll textMarquee
	for _, tc := range []struct {
		elapsed time.Duration
		want    float32
	}{
		{0, 0}, {textScrollDelay - time.Millisecond, 0},
		{textScrollDelay + time.Second, 30},
		{textScrollDelay + 6*time.Second, 180},
		{textScrollDelay + 7*time.Second, 0},
		{textScrollDelay + 8*time.Second, 30},
	} {
		gtx.Now = start.Add(tc.elapsed)
		if got := scroll.offsetWith(gtx, "path • ", 100, 210, 30, true); math.Abs(float64(got-tc.want)) > 0.001 {
			t.Fatalf("at %s ticker offset=%v; want %v", tc.elapsed, got, tc.want)
		}
	}
}

func TestTextMarqueeSpeedAndModelRestart(t *testing.T) {
	start := time.Unix(1000, 0)
	for _, scale := range []float32{1, 2} {
		for _, speed := range []unit.Dp{6, 30, 60} {
			gtx := layout.Context{Now: start, Metric: unit.Metric{PxPerDp: scale}}
			var scroll textMarquee
			scroll.offsetWith(gtx, "long path", 100, 1000, speed, false)
			gtx.Now = start.Add(textScrollDelay + 2*time.Second)
			if got, want := scroll.offsetWith(gtx, "long path", 100, 1000, speed, false), 2*float32(gtx.Dp(speed)); math.Abs(float64(got-want)) > 0.001 {
				t.Fatalf("speed %v scale %v: offset=%v; want %v", speed, scale, got, want)
			}
			if got := scroll.offsetWith(gtx, "long path", 100, 1000, speed, true); got != 0 {
				t.Fatal("changing animation model did not restart")
			}
			gtx.Now = gtx.Now.Add(2 * time.Second)
			if got := scroll.offsetWith(gtx, "long path", 100, 1000, speed/2, true); got != 0 {
				t.Fatal("changing speed did not restart")
			}
		}
	}
}
