package app

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"slices"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
	"github.com/dannyben/commando/internal/tempfiles"
)

const windowWidth, windowHeight = 800, 480

// Run opens the native folder browser.
func Run() error {
	preview := flag.String("png", "", "render the workspace to a PNG and exit")
	traceMask := flag.Int("trace", 0, "diagnostic trace mask (0–127)")
	flag.Parse()
	if *traceMask < 0 || *traceMask > int(diagnostics.All) {
		return fmt.Errorf("trace mask must be 0 to %d", diagnostics.All)
	}
	var traceOverride *diagnostics.Mask
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "trace" {
			m := diagnostics.Mask(*traceMask)
			traceOverride = &m
		}
	})
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
	tempfiles.Clean()
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
	saved, stateErr := loadState(dataDir)
	if stateErr != nil {
		fmt.Fprintln(os.Stderr, "cannot load state:", stateErr)
	}
	visited, visitedErr := loadVisitedFolders(dataDir, settingsValues.Get(settings.CatalogueDefault, settingSaveHistory).Bool)
	if visitedErr != nil {
		fmt.Fprintln(os.Stderr, "cannot load visited folders:", visitedErr)
	}
	locations, err := startingLocations(flag.Args(), saved.Panels)
	if err != nil {
		return err
	}

	mask := diagnostics.Mask(settingsValues.Get(settings.CatalogueDefault, settingTraceMask).Number)
	if traceOverride != nil {
		mask = *traceOverride
	}
	if err := diagnostics.Configure(dataDir, mask); err != nil {
		fmt.Fprintln(os.Stderr, "cannot open trace log:", err)
	}
	defer diagnostics.Close()
	diagnostics.Record(diagnostics.Storage, 0, "StartupStorage", map[string]any{"settings_error": diagnostics.Error(settingsErr), "state_error": diagnostics.Error(stateErr)})
	diagnostics.Record(diagnostics.Lifecycle, 0, "Startup", map[string]any{"paths": locations, "data_folder": dataDir})

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
		v := newLocationView(locations, saved.ActivePanel, w.Invalidate, dataDir)
		v.visited = visited
		v.helpHidden = saved.HelpHidden
		v.defaultDataFolder = baseDir
		v.activeDataFolder = dataDir
		store.restoreLocation(&settingsValues)
		v.traceOverride = traceOverride
		v.restoreSettings(settingsValues, settingsErr)
		if folderErr != nil {
			v.showFeedback(v.active, feedback.Error, "Cannot read data folder")
		}
		v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
			state := v.sessionState(tracker.snapshot())
			state.PanelWidth = tracker.sizingSnapshot().baseWidth
			visited := slices.Clone(v.visited)
			return func() (string, error) {
				if err := saveVisitedFolders(dataDir, visited, values.Get(settings.CatalogueDefault, settingSaveHistory).Bool); err != nil {
					return "", err
				}
				return store.save(values, state)
			}
		}
		v.monitor = newDirectoryMonitor(browser.WatchDirectory, w.Invalidate)
		fullScreenRequested := false
		v.toggleFullScreen = func() { fullScreenRequested = !fullScreenRequested }
		var resizeRequests []windowResizeRequest
		v.resizeWindow = func(axis windowAxis, direction int) {
			resizeRequests = append(resizeRequests, windowResizeRequest{axis, direction})
		}
		v.nativeKeys = keys
		v.updateNativeBindings()
		v.shellExecutor = native.shellExecutor(w)
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
				tempfiles.Clean()
				sounds.Close()
				state := v.sessionState(tracker.snapshot())
				state.PanelWidth = tracker.sizingSnapshot().baseWidth
				result := e.Err
				if err := saveState(dataDir, state); err != nil {
					result = errors.Join(e.Err, fmt.Errorf("cannot save state: %w", err))
				}
				historySettings := v.settingsValues
				if s := v.settingsWidget(); s != nil {
					historySettings = s.before // Closing without Save discards the widget preview.
				}
				if err := saveVisitedFolders(dataDir, v.visited, historySettings.Get(settings.CatalogueDefault, settingSaveHistory).Bool); err != nil {
					result = errors.Join(result, fmt.Errorf("cannot save visited folders: %w", err))
				}
				diagnostics.Record(diagnostics.Lifecycle, 0, "Shutdown", map[string]any{"error": diagnostics.Error(result)})
				diagnostics.Close()
				instance.close()
				done <- result
				return
			case gioapp.FrameEvent:
				v.layout(gioapp.NewContext(&ops, e))
				e.Frame(&ops)
			}
			native.configure(w, e)
			if fullScreenRequested {
				fullScreenRequested = false
				if enabled, changed := native.toggleFullScreen(w); changed {
					v.announceBoolean(enabled)
				}
			}
			for _, request := range resizeRequests {
				if native.resizeWindow(w, request) {
					v.announceInteraction(eventValueChanged)
				}
			}
			resizeRequests = resizeRequests[:0]
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
	return renderSizedViewPNG(path, view, image.Pt(windowWidth, windowHeight))
}

func renderSizedViewPNG(path string, view *view, size image.Point) error {
	w, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		return err
	}
	defer w.Release()
	var ops op.Ops
	view.layout(layout.Context{
		Ops: &ops, Now: time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	})
	if err := w.Frame(&ops); err != nil {
		return err
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
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
