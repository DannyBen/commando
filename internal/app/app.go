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
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

const windowWidth, windowHeight = 800, 480

// Run opens the native folder browser.
func Run() error {
	preview := flag.String("png", "", "render the workspace to a PNG and exit")
	traceShell := flag.Bool("trace-shell", false, "record shell menu preparation timings")
	traceTransfers := flag.Bool("trace-transfers", false, "record transfer diagnostics in the application data folder")
	flag.Parse()
	if *preview != "" {
		return renderPNG(*preview)
	}
	baseDir, err := defaultDataDir()
	if err != nil {
		return err
	}
	// Keep the instance identity stable while a new data folder awaits restart.
	instance, primary, err := startInstance(baseDir)
	if err != nil || !primary {
		return err
	}
	defer instance.close()
	dataDir, folderErr := resolveDataFolder(baseDir)
	if folderErr != nil {
		fmt.Fprintln(os.Stderr, "cannot load data folder:", folderErr)
	}
	store := &dataFolderStore{base: baseDir, active: dataDir, chosen: dataDir}
	if folderErr != nil {
		store.chosen = "" // Saving a choice can repair an unreadable startup pointer.
	}
	settingsValues, settingsErr := loadStartupSettings(dataDir, systemPrefersDarkTheme)
	if settingsErr != nil {
		fmt.Fprintln(os.Stderr, "settings:", settingsErr)
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

	var shellTrace func(shellCompletion)
	stopShellTrace := func() error { return nil }
	if *traceShell {
		shellTrace, stopShellTrace, err = openShellTrace(dataDir)
		if err != nil {
			return fmt.Errorf("cannot open shell trace: %w", err)
		}
	}
	defer stopShellTrace()

	done := make(chan error, 1)
	go func() {
		w := new(gioapp.Window)
		tracker := windowTracker{value: saved.Window, sizing: workspaceSizing{count: len(locations), baseWidth: saved.PanelWidth}}
		keys := &nativeKeyInput{invalidate: w.Invalidate}
		native := nativeWindow{state: &tracker, keys: keys, instance: instance}
		w.Option(gioapp.Title("Commando"),
			gioapp.Size(unit.Dp(windowWidth), unit.Dp(windowHeight)),
			gioapp.MinSize(unit.Dp(minWindowWidth), unit.Dp(360)))
		w.Option(native.options()...)
		v := newLocationView(locations, saved.ActivePanel, w.Invalidate)
		v.defaultDataFolder = baseDir
		v.activeDataFolder = dataDir
		store.restoreLocation(&settingsValues)
		v.restoreSettings(settingsValues, settingsErr)
		if folderErr != nil {
			v.showFeedback(v.active, feedback.Error, "Cannot read data folder")
		}
		v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
			state := v.sessionState(tracker.snapshot())
			state.PanelWidth = tracker.sizingSnapshot().baseWidth
			return func() (string, error) { return store.save(values, state) }
		}
		v.monitor = newDirectoryMonitor(browser.WatchDirectory, w.Invalidate)
		v.nativeKeys = keys
		v.updateNativeBindings()
		v.shellExecutor = native.shellExecutor(w)
		v.shellTrace = shellTrace
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
				stopShellTrace()
				state := v.sessionState(tracker.snapshot())
				state.PanelWidth = tracker.sizingSnapshot().baseWidth
				result := e.Err
				if err := saveState(dataDir, state); err != nil {
					result = errors.Join(e.Err, fmt.Errorf("cannot save state: %w", err))
				}
				instance.close()
				done <- result
				return
			case gioapp.FrameEvent:
				v.layout(gioapp.NewContext(&ops, e))
				e.Frame(&ops)
			}
			native.configure(w, e)
			native.resizePanels(w, len(v.panels))
			v.ownerWindow = native.owner()
			v.dispatchNative(w.Invalidate)
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
	v := newPreviewView()
	v.applySettings()
	return renderViewPNG(path, v)
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
	home, err := os.UserHomeDir()
	if err != nil && len(args) == 0 {
		return nil, err
	}
	if len(args) > 0 {
		home = args[0]
	}
	paths := make([]string, max(3, len(args)))
	for i := range paths {
		paths[i] = home
	}
	copy(paths, args)
	return paths, nil
}
