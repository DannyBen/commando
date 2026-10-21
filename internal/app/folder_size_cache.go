package app

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/diagnostics"
)

const folderSizeLifetime = time.Hour

type folderSize struct {
	Path         string    `json:"path"`
	Bytes        int64     `json:"bytes"`
	Files        int64     `json:"files"`
	CalculatedAt time.Time `json:"calculated_at"`
}

type folderSizeCache map[string]folderSize

func (c folderSizeCache) clone() folderSizeCache { return maps.Clone(c) }

func (l *directoryLoader) saveFolderSizeCache() {
	err := saveFolderSizes(l.folderCacheDir, l.folderSizes, time.Now())
	diagnostics.Record(diagnostics.Storage, 0, "FolderSizeCacheSaved", map[string]any{"error": diagnostics.Error(err), "entries": len(l.folderSizes)})
}

func folderSizeKey(path string) string { return transferNameKey(filepath.Clean(path)) }

func (s folderSize) valid(now time.Time) bool {
	return filepath.IsAbs(s.Path) && s.Bytes >= 0 && s.Files >= 0 && !s.CalculatedAt.IsZero() &&
		!now.Before(s.CalculatedAt) && now.Before(s.CalculatedAt.Add(folderSizeLifetime))
}

func (c folderSizeCache) prune(now time.Time) {
	for key, record := range c {
		if !record.valid(now) {
			delete(c, key)
		}
	}
}

// Disk access is confined to the directory worker. Missing caches stay missing.
func loadFolderSizes(dataDir string, now time.Time) (folderSizeCache, error) {
	cache := make(folderSizeCache)
	data, err := os.ReadFile(filepath.Join(dataDir, "cache", "folders.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cache, nil
	}
	if err != nil {
		return cache, err
	}
	var records []folderSize
	if err = json.Unmarshal(data, &records); err != nil {
		return cache, err
	}
	for _, record := range records {
		if record.valid(now) {
			cache[folderSizeKey(record.Path)] = record
		}
	}
	if len(cache) != len(records) {
		err = saveFolderSizes(dataDir, cache, now)
	}
	return cache, err
}

func saveFolderSizes(dataDir string, cache folderSizeCache, now time.Time) error {
	cache.prune(now)
	records := make([]folderSize, 0, len(cache))
	for _, record := range cache {
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b folderSize) int { return strings.Compare(a.Path, b.Path) })
	dir := filepath.Join(dataDir, "cache")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".folders-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(records); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(dir, "folders.json"))
}

func (p *panel) cachedFolderSize(file browser.Entry, now time.Time) (folderSize, bool) {
	if !p.nativeFolder() || !file.Directory || file.Parent || file.Name == ".." {
		return folderSize{}, false
	}
	record, ok := p.folderSizes[folderSizeKey(filepath.Join(p.path, file.Name))]
	return record, ok && record.valid(now)
}

func (p *panel) entrySize(file browser.Entry, now time.Time) (int64, bool) {
	if file.Directory {
		record, ok := p.cachedFolderSize(file, now)
		return record.Bytes, ok
	}
	return file.Bytes, file.SizeKnown
}

func (p *panel) displayEntry(item entry) entry {
	if p.drives {
		item.size = p.driveSpaceLabel(item.file)
		return item
	}
	if record, ok := p.cachedFolderSize(item.file, time.Now()); ok {
		item.size = "≈" + formatSize(record.Bytes)
	}
	return item
}

func (v *view) applyFolderSizes(cache folderSizeCache) {
	clear(v.folderSizes)
	for key, record := range cache {
		v.folderSizes[key] = record
	}
	for i := range v.panels {
		p := &v.panels[i]
		if p.sortByMetadata && p.metadata == metadataSize && !p.loading {
			p.resort()
		}
	}
}

// No timer or filesystem reads: retire expired measurements on the next frame.
func (v *view) expireFolderSizes(now time.Time) {
	if !v.interaction.idle() {
		return
	}
	count := len(v.folderSizes)
	v.folderSizes.prune(now)
	if count != len(v.folderSizes) {
		for i := range v.panels {
			p := &v.panels[i]
			if p.sortByMetadata && p.metadata == metadataSize && !p.loading {
				p.resort()
			}
		}
	}
}
