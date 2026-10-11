//go:build !windows

package app

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gioapp "gioui.org/app"
	"gioui.org/unit"
)

func TestDevelopmentWindowGeometry(t *testing.T) {
	tracker := windowTracker{value: windowState{Width: 900, Height: 500, Maximized: true}}
	n := nativeWindow{state: &tracker}
	var config gioapp.Config
	for _, option := range n.options() {
		option(unit.Metric{PxPerDp: 2}, &config)
	}
	if config.Size != image.Pt(1800, 1000) || config.Mode != gioapp.Maximized {
		t.Fatalf("saved window options were not applied: %+v", config)
	}
	for _, mode := range []gioapp.WindowMode{gioapp.Maximized, gioapp.Minimized} {
		n.configure(nil, gioapp.ConfigEvent{Config: gioapp.Config{Mode: mode}})
		n.configure(nil, gioapp.FrameEvent{Size: image.Pt(4000, 2000), Metric: unit.Metric{PxPerDp: 2}})
		if got := tracker.snapshot(); got.Width != 900 || got.Height != 500 || !got.Maximized {
			t.Fatalf("maximizing/minimizing overwrote the normal size: %+v", got)
		}
	}
	n.configure(nil, gioapp.ConfigEvent{Config: gioapp.Config{Mode: gioapp.Windowed}})
	n.configure(nil, gioapp.FrameEvent{Size: image.Pt(2000, 1200), Metric: unit.Metric{PxPerDp: 2}})
	if got := tracker.snapshot(); got.Width != 1000 || got.Height != 600 || got.Maximized {
		t.Fatalf("normal frame did not save dp size: %+v", got)
	}
}

func TestDefaultDevelopmentDataFolder(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS uses Application Support")
	}
	chosen := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", chosen)
	got, err := defaultDataDir()
	if err != nil || got != filepath.Join(chosen, "CONTROL") {
		t.Fatalf("data folder=%q err=%v", got, err)
	}
	if _, err := os.Stat(chosen); !os.IsNotExist(err) {
		t.Fatal("resolving the data folder created it")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"", "relative-path"} {
		t.Setenv("XDG_DATA_HOME", base)
		got, err := defaultDataDir()
		if err != nil || got != filepath.Join(home, ".local", "share", "CONTROL") {
			t.Fatalf("default data folder=%q err=%v", got, err)
		}
	}
}
