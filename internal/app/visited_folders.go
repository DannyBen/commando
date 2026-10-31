package app

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/dannyben/commando/internal/fuzzy"
)

const visitedFoldersLimit = 500
const visitedFoldersFile = "history.json"
const previousVisitedFoldersFile = "visited-folders.json"

type visitedFolders []string // Most recently visited first.

func loadVisitedFolders(dataDir string, enabled bool) (visitedFolders, error) {
	if !enabled {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dataDir, visitedFoldersFile))
	if errors.Is(err, os.ErrNotExist) {
		data, err = os.ReadFile(filepath.Join(dataDir, previousVisitedFoldersFile))
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, err
	}
	var history visitedFolders
	for i := len(paths) - 1; i >= 0; i-- {
		history.remember(paths[i])
	}
	return history, nil
}

func saveVisitedFolders(dataDir string, history visitedFolders, enabled bool) error {
	if !enabled {
		return nil
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dataDir, ".history-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := json.NewEncoder(f).Encode(history); err != nil {
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
	return os.Rename(f.Name(), filepath.Join(dataDir, visitedFoldersFile))
}

func (h *visitedFolders) remember(path string) {
	if !filepath.IsAbs(path) {
		return
	}
	path = filepath.Clean(path)
	if filepath.Dir(path) == path {
		return // A filesystem or drive root is not a useful jump target.
	}
	paths := make(visitedFolders, 0, min(len(*h)+1, visitedFoldersLimit))
	paths = append(paths, path)
	for _, old := range *h {
		if !samePath(path, old) && len(paths) < visitedFoldersLimit {
			paths = append(paths, old)
		}
	}
	*h = paths
}

func (h *visitedFolders) forget(path string) {
	for i, old := range *h {
		if samePath(path, old) {
			*h = append((*h)[:i], (*h)[i+1:]...)
			return
		}
	}
}

// Folder names rank before broader path matches. Recency breaks equal scores.
func (h visitedFolders) best(query string) string {
	matches := h.matches(query)
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func (h visitedFolders) matches(query string) []string {
	if query == "" {
		return nil
	}
	type candidate struct {
		path  string
		score float64
	}
	var candidates []candidate
	for _, path := range h {
		name := filepath.Base(path)
		score := math.Inf(-1)
		if result, ok := fuzzy.Match(query, name); ok {
			score = 1000 + result.Score
		} else if result, ok := fuzzy.Match(query, path); ok {
			score = result.Score
		}
		if !math.IsInf(score, -1) {
			candidates = append(candidates, candidate{path, score})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	paths := make([]string, len(candidates))
	for i, candidate := range candidates {
		paths[i] = candidate.path
	}
	return paths
}
