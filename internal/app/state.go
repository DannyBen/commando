package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// All saved data lives under the dataDir resolved at startup. State and future
// settings receive that directory rather than choosing their own locations.
type sessionState struct {
	Window      windowState  `json:"window"`
	Panels      []panelState `json:"panels"`
	ActivePanel int          `json:"activePanel"`
}

type panelState struct {
	Path     string `json:"path"`
	Drives   bool   `json:"drives,omitempty"`
	Metadata string `json:"metadata,omitempty"`
}

type windowState struct {
	X          int  `json:"x,omitempty"`
	Y          int  `json:"y,omitempty"`
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Positioned bool `json:"positioned,omitempty"`
	Maximized  bool `json:"maximized,omitempty"`
}

// Native Windows callbacks and the app goroutine share the last placement.
type windowTracker struct {
	mu    sync.Mutex
	value windowState
}

func (t *windowTracker) snapshot() windowState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.value
}

func (t *windowTracker) set(value windowState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.value = value
}

func (s windowState) valid() bool {
	// Reject corrupt dimensions before they reach a native API or renderer.
	return s.Width > 0 && s.Height > 0 && s.Width <= 32768 && s.Height <= 32768 &&
		s.X >= -1<<30 && s.X <= 1<<30 && s.Y >= -1<<30 && s.Y <= 1<<30
}

func loadState(dataDir string) (sessionState, error) {
	if dataDir == "" {
		return sessionState{}, fmt.Errorf("data folder is empty")
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return sessionState{}, nil
	}
	if err != nil {
		return sessionState{}, err
	}
	var state sessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return sessionState{}, err
	}
	if !state.Window.valid() {
		state.Window = windowState{}
	}
	return state, nil
}

func saveState(dataDir string, state sessionState) error {
	if dataDir == "" {
		return fmt.Errorf("data folder is empty")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dataDir, ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dataDir, "state.json"))
}

func startingLocations(args []string, saved []panelState) ([]panelState, error) {
	paths, err := startingPaths(args)
	if err != nil {
		return nil, err
	}
	locations := make([]panelState, len(paths))
	for i, path := range paths {
		locations[i].Path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		// Keep the existing CLI contract: supplied paths define all panels,
		// with omitted paths repeating the first supplied folder.
		// Column choices belong to the panel independently of its location.
		if i < len(saved) {
			locations[i].Metadata = saved[i].Metadata
		}
		if len(args) == 0 && i < len(saved) && filepath.IsAbs(saved[i].Path) {
			locations[i] = saved[i]
		}
	}
	return locations, nil
}

func (v *view) sessionState(window windowState) sessionState {
	state := sessionState{Window: window, ActivePanel: v.active, Panels: make([]panelState, len(v.panels))}
	for i, p := range v.panels {
		state.Panels[i] = panelState{Path: p.path, Drives: p.drives, Metadata: p.metadata.stateName()}
	}
	return state
}
