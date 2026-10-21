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

func TestNotificationFooterPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_MESSAGES")
	if directory == "" {
		t.Skip("optional notification footer previews")
	}
	for _, mode := range []string{"light", "dark"} {
		v := newPreviewView()
		if mode == "dark" {
			v.settingsValues = builtInThemes[1].values.Clone()
			v.applySettings()
		}
		v.followMe = true
		v.panels[1].items[v.panels[1].selected] = v.panels[0].items[v.panels[0].selected]
		for i, kind := range []feedback.Kind{feedback.Info, feedback.Success, feedback.Error} {
			message := []string{"Ready", "Folder created", "Access denied"}[i]
			v.panels[i].feedback.ShowWithStyle(kind, message, feedback.Immediate)
		}
		if err := renderViewPNG(filepath.Join(directory, "notifications-"+mode+".png"), v); err != nil {
			t.Fatal(err)
		}
		v.panels[0].feedback.Dismiss()
		v.panels[1].feedback.ShowWithStyle(feedback.Warning, "Check destination", feedback.Immediate)
		v.panels[2].feedback.QuietProgressAt("Reading folder", time.Unix(1, 0))
		if err := renderViewPNG(filepath.Join(directory, "notifications-progress-"+mode+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}

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
	background, _ := v.notificationColors(feedback.Success)
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
		for _, y := range []int{452, 462, 474} {
			point := aboveHelpFooter(v, f32.Pt(10, float32(y)))
			if color.NRGBAModel.Convert(img.At(10, int(point.Y))) != background {
				t.Fatalf("notification background at %v: got %v, want %v", point, color.NRGBAModel.Convert(img.At(10, int(point.Y))), background)
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
		for _, y := range []int{452, 462, 474} {
			point := aboveHelpFooter(v, f32.Pt(10, float32(y)))
			if color.NRGBAModel.Convert(img.At(10, int(point.Y))) != want {
				t.Fatalf("%s: progress background at %v: got %v, want %v", tc.name, point, color.NRGBAModel.Convert(img.At(10, int(point.Y))), want)
			}
		}
		if (v.panels[0].feedback.Current() != nil) != tc.visible {
			t.Fatalf("%s: delayed progress visibility changed", tc.name)
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
