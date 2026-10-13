package app

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dannyben/commando/internal/browser"
)

func TestAdjacentOpeningDoesNotActivateRetainedRow(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			v := newPreviewView()
			v.active = 1
			v.panels[1].selected = 1 // Folder in the source.
			v.panels[2].selected = 2 // Old file in the destination.
			p := &v.panels[2]
			before := slices.Clone(p.items)
			oldPath := p.path
			v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
			v.openAdjacent(1)
			if v.active != 2 || !p.loading || !p.navigating || p.selected != 2 || !slices.Equal(p.items, before) {
				t.Fatal("destination did not retain its state while loading with immediate focus")
			}
			// Optional rendered regression check: the old file's active fill must
			// be absent during loading, and return if the read fails.
			checkFill := func(name string, want bool) {
				if os.Getenv("COMMANDO_PREVIEW_ADJACENT") == "" {
					return
				}
				path := filepath.Join(t.TempDir(), name+".png")
				if err := renderViewPNG(path, v); err != nil {
					t.Fatal(err)
				}
				f, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.Decode(f)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
				bg, _, _ := v.appearance.entryColors(before[2].file, true, false)
				found := false
				for y := 70; y < 440; y++ {
					found = found || color.NRGBAModel.Convert(img.At(780, y)) == bg
				}
				if found != want {
					t.Fatalf("%s: retained row active fill present=%v, want %v", name, found, want)
				}
			}
			checkFill("pending", false)
			request := <-v.loader.requests
			result := loadResult{request: request, directory: browser.Directory{Path: request.path}}
			if failed {
				result.err = os.ErrPermission
			}
			v.acceptLoad(result)
			if p.loading || p.navigating {
				t.Fatal("completion left the destination's selection suppressed")
			}
			if failed {
				if p.path != oldPath || p.selected != 2 || !slices.Equal(p.items, before) {
					t.Fatal("failed navigation lost the retained listing or selection")
				}
				checkFill("failed", true)
			} else if p.path != request.path || len(p.items) != 0 {
				t.Fatal("successful navigation did not replace the old listing")
			}
		})
	}
}

func TestRefreshKeepsActiveRowVisible(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
	v.refreshPanel()
	if !v.panels[v.active].loading || v.panels[v.active].navigating {
		t.Fatal("refresh suppressed the current row's active highlight")
	}
}
