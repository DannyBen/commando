package app

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

const windowWidth, windowHeight = 800, 480

// Run opens the native folder browser.
func Run() error {
	preview := flag.String("png", "", "render the workspace to a PNG and exit")
	flag.Parse()
	paths, err := startingPaths(flag.Args())
	if err != nil {
		return err
	}
	if *preview != "" {
		return renderPNG(*preview)
	}

	done := make(chan error, 1)
	go func() {
		w := new(gioapp.Window)
		w.Option(gioapp.Title("CONTROL"),
			gioapp.Size(unit.Dp(windowWidth), unit.Dp(windowHeight)),
			gioapp.MinSize(unit.Dp(720), unit.Dp(360)))
		v := newBrowserView(paths, w.Invalidate)
		var ops op.Ops
		for {
			e := w.Event()
			configureNativeWindow(w, e)
			switch e := e.(type) {
			case gioapp.DestroyEvent:
				v.loader.close()
				done <- e.Err
				return
			case gioapp.FrameEvent:
				v.layout(gioapp.NewContext(&ops, e))
				e.Frame(&ops)
			}
		}
	}()
	go func() {
		if err := <-done; err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	gioapp.Main()
	return nil
}

func renderPNG(path string) error {
	w, err := headless.NewWindow(windowWidth, windowHeight)
	if err != nil {
		return err
	}
	defer w.Release()
	var ops op.Ops
	newPreviewView().layout(layout.Context{
		Ops: &ops, Now: time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(windowWidth, windowHeight)),
	})
	if err := w.Frame(&ops); err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
	if err := w.Screenshot(img); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func startingPaths(args []string) ([]string, error) {
	if len(args) > 3 {
		return nil, fmt.Errorf("provide at most three startup folders")
	}
	home, err := os.UserHomeDir()
	if err != nil && len(args) == 0 {
		return nil, err
	}
	if len(args) > 0 {
		home = args[0]
	}
	paths := []string{home, home, home}
	copy(paths, args)
	return paths, nil
}
