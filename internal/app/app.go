package app

import (
	"errors"
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
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/sound"
)

const windowWidth, windowHeight = 800, 480

// Run opens the native folder browser.
func Run() error {
	preview := flag.String("png", "", "render the workspace to a PNG and exit")
	traceTransfers := flag.Bool("trace-transfers", false, "record transfer diagnostics in the application data folder")
	flag.Parse()
	if *preview != "" {
		return renderPNG(*preview)
	}
	dataDir, err := defaultDataDir()
	if err != nil {
		return err
	}
	saved, err := loadState(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot load state:", err)
	}
	locations, err := startingLocations(flag.Args(), saved.Panels)
	if err != nil {
		return err
	}

	stopTrace := func() error { return nil }
	if *traceTransfers {
		var tracePath string
		tracePath, stopTrace, err = browser.EnableTransferTrace(dataDir)
		if err != nil {
			return fmt.Errorf("cannot open transfer trace: %w", err)
		}
		fmt.Fprintln(os.Stderr, "Transfer trace:", tracePath)
	}
	defer stopTrace()

	done := make(chan error, 1)
	go func() {
		w := new(gioapp.Window)
		tracker := windowTracker{value: saved.Window}
		keys := &nativeKeyInput{invalidate: w.Invalidate}
		native := nativeWindow{state: &tracker, keys: keys}
		w.Option(gioapp.Title("Commando"),
			gioapp.Size(unit.Dp(windowWidth), unit.Dp(windowHeight)),
			gioapp.MinSize(unit.Dp(720), unit.Dp(360)))
		w.Option(native.options()...)
		v := newLocationView(locations, saved.ActivePanel, w.Invalidate)
		v.monitor = newDirectoryMonitor(browser.WatchDirectory, w.Invalidate)
		v.nativeKeys = keys
		sounds := sound.NewPlayer()
		defer sounds.Close()
		v.sounds = sounds
		var ops op.Ops
		for {
			e := w.Event()
			switch e := e.(type) {
			case *gioapp.ClosingEvent:
				if !v.requestClose() {
					e.Abort()
					w.Invalidate()
				}
			case gioapp.DestroyEvent:
				v.finishOperations()
				sounds.Close()
				stopTrace()
				if err := saveState(dataDir, v.sessionState(tracker.snapshot())); err != nil {
					done <- errors.Join(e.Err, fmt.Errorf("cannot save state: %w", err))
				} else {
					done <- e.Err
				}
				return
			case gioapp.FrameEvent:
				v.layout(gioapp.NewContext(&ops, e))
				e.Frame(&ops)
			}
			native.configure(w, e)
			v.ownerWindow = native.owner()
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
	return renderViewPNG(path, newPreviewView())
}

func renderViewPNG(path string, view *view) error {
	w, err := headless.NewWindow(windowWidth, windowHeight)
	if err != nil {
		return err
	}
	defer w.Release()
	var ops op.Ops
	view.layout(layout.Context{
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
