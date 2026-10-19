package app

import (
	"errors"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/feedback"
)

func TestFeedbackPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_MESSAGES")
	if directory == "" {
		t.Skip("optional headless message animation preview")
	}
	w, err := headless.NewWindow(windowWidth, windowHeight)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Release()
	v := newPreviewView()
	v.showFeedback(0, feedback.Success, "Folder created")
	start := time.Unix(1000, 0)
	var ops op.Ops
	animation := &gif.GIF{LoopCount: 0}
	for ms := 0; ms <= 2200; ms += 50 {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Now: start.Add(time.Duration(ms) * time.Millisecond),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(windowWidth, windowHeight))})
		if err := w.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
		if err := w.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		for _, y := range []int{450, 451} {
			point := aboveHelpFooter(v, f32.Pt(10, float32(y)))
			if color.NRGBAModel.Convert(img.At(10, int(point.Y))) != v.feedbackColor(feedback.Success) {
				t.Fatal("message edge was not two pixels high across animation states")
			}
		}
		if ms == 200 {
			f, err := os.Create(filepath.Join(directory, "feedback.png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
		frame := image.NewPaletted(img.Bounds(), palette.Plan9)
		draw.FloydSteinberg.Draw(frame, frame.Bounds(), img, image.Point{})
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 5)
	}
	f, err := os.Create(filepath.Join(directory, "feedback.gif"))
	if err != nil {
		t.Fatal(err)
	}
	err = gif.EncodeAll(f, animation)
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func TestDelayedFeedbackPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_MESSAGES")
	if directory == "" {
		t.Skip("optional headless delayed feedback preview")
	}
	w, err := headless.NewWindow(windowWidth, windowHeight)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Release()
	v := newPreviewView()
	start := time.Unix(1000, 0)
	due := start.Add(v.appearance.FeedbackProgressDelay)
	v.panels[0].feedback.ProgressAt("Reading folder", due)
	for _, tc := range []struct {
		name    string
		now     time.Time
		visible bool
	}{
		{"reading-pending", due.Add(-time.Nanosecond), false},
		{"reading-started", due, true},
		{"reading-typed", due.Add(15 * time.Millisecond), true},
		{"reading-completed", due.Add(time.Second), false},
	} {
		if tc.name == "reading-completed" {
			v.panels[0].feedback.Complete()
		}
		var ops op.Ops
		v.layout(layout.Context{Ops: &ops, Now: tc.now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(windowWidth, windowHeight))})
		if err := w.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
		if err := w.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		want := v.appearance.Colors.Footer
		if tc.visible {
			want = v.feedbackColor(feedback.Info)
		}
		for _, y := range []int{450, 451} {
			point := aboveHelpFooter(v, f32.Pt(10, float32(y)))
			if color.NRGBAModel.Convert(img.At(10, int(point.Y))) != want {
				t.Fatalf("%s: footer edge appeared early or did not follow visible progress", tc.name)
			}
		}
		f, err := os.Create(filepath.Join(directory, tc.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		encodeErr := png.Encode(f, img)
		closeErr := f.Close()
		if err := errors.Join(encodeErr, closeErr); err != nil {
			t.Fatal(err)
		}
	}
}
