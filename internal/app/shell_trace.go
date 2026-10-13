package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Optional diagnostics separate app dispatch, shell resolution, extension
// acquisition and menu population. No filenames or user menu-wait time.
func openShellTrace(dataDir string) (func(shellCompletion), func() error, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "shell-menu.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, err
	}
	encoder := json.NewEncoder(f)
	return func(result shellCompletion) {
		ms := func(d time.Duration) float64 { return d.Seconds() * 1000 }
		timing := result.result.Timings
		event := struct {
			Time      time.Time `json:"time"`
			Queue     float64   `json:"queueMs"`
			Resolve   float64   `json:"resolveMs"`
			Object    float64   `json:"objectMs"`
			Populate  float64   `json:"populateMs"`
			Ready     float64   `json:"readyMs"`
			Cancelled bool      `json:"cancelled"`
			Error     string    `json:"error,omitempty"`
		}{Time: time.Now(), Queue: ms(result.queue), Resolve: ms(timing.Resolve), Object: ms(timing.Object), Populate: ms(timing.Populate), Ready: ms(result.queue + timing.Ready), Cancelled: result.result.Cancelled}
		if result.err != nil {
			event.Error = result.err.Error()
		}
		encoder.Encode(event)
	}, f.Close, nil
}
