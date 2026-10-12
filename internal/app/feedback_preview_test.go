package app

import (
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
			if color.NRGBAModel.Convert(img.At(10, y)) != v.feedbackColor(feedback.Success) {
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
