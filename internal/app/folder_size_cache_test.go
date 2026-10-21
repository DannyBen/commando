package app

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func readSizeRecords(t *testing.T, dir string) []folderSize {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "cache", "folders.json"))
	if err != nil {
		t.Fatal(err)
	}
	var records []folderSize
	if err = json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	return records
}

func TestFolderSizeCacheCleanupAcrossAllEntries(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	fresh := folderSize{Path: filepath.Join(dir, "fresh"), Bytes: 123, Files: 2, CalculatedAt: now.Add(-30 * time.Minute)}
	expired := folderSize{Path: filepath.Join(dir, "deleted-and-never-visited"), CalculatedAt: now.Add(-time.Hour)}
	future := folderSize{Path: filepath.Join(dir, "future"), CalculatedAt: now.Add(time.Hour)}
	if err := os.Mkdir(filepath.Join(dir, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal([]folderSize{fresh, expired, future})
	if err := os.WriteFile(filepath.Join(dir, "cache", "folders.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	cache, err := loadFolderSizes(dir, now)
	if err != nil || len(cache) != 1 {
		t.Fatalf("load=%v, %v", cache, err)
	}
	if records := readSizeRecords(t, dir); len(records) != 1 || records[0].Path != fresh.Path {
		t.Fatal("startup did not persist whole-cache cleanup")
	}
	before := cache[folderSizeKey(fresh.Path)].CalculatedAt
	p := panel{path: dir, folderSizes: cache}
	file := browser.Entry{Name: "fresh", Directory: true}
	if _, ok := p.cachedFolderSize(file, now); !ok {
		t.Fatal("fresh cache unavailable")
	}
	if !cache[folderSizeKey(fresh.Path)].CalculatedAt.Equal(before) {
		t.Fatal("lookup extended the lifetime")
	}
	cache[folderSizeKey(expired.Path)] = expired
	if err := saveFolderSizes(dir, cache, now); err != nil {
		t.Fatal(err)
	}
	if len(readSizeRecords(t, dir)) != 1 {
		t.Fatal("save retained expired unvisited entry")
	}
	if err := saveFolderSizes(dir, cache, now.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(readSizeRecords(t, dir)) != 0 {
		t.Fatal("boundary did not expire cache")
	}
}

func TestFolderSizeCacheMissingAndMalformed(t *testing.T) {
	dir := t.TempDir()
	cache, err := loadFolderSizes(dir, time.Now())
	if err != nil || len(cache) != 0 {
		t.Fatal(cache, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "cache")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing cache created files")
	}
	if err = os.Mkdir(filepath.Join(dir, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "cache", "folders.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	cache, err = loadFolderSizes(dir, time.Now())
	if err == nil || len(cache) != 0 {
		t.Fatal("corrupt cache trusted")
	}
}

func TestFolderSizeCacheNavigationRestartAndRecalculation(t *testing.T) {
	root, dataDir := t.TempDir(), t.TempDir()
	for _, name := range []string{"alpha", "beta", "uncalculated"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{"alpha/file": "abc", "alpha/.empty": "", "beta/file": "12345", "loose.txt": "12"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	start := func() *view {
		v := newLocationView([]panelState{{Path: root, Metadata: "extension"}, {Path: root}}, 0, nil, dataDir)
		t.Cleanup(v.finishOperations)
		waitLoaded(t, v, 0)
		waitLoaded(t, v, 1)
		return v
	}
	v := start()
	p := &v.panels[0]
	for _, name := range []string{"alpha", "beta", "loose.txt"} {
		selectName(t, p, name)
		p.toggleMark(p.selected)
	}
	setTestShortcut(t, v, "entry.calculate_size", 0, "Ctrl+F9")
	send, _ := panelInputFrames(v)
	calculate := func() {
		t.Helper()
		send(modified(key.NameF9, key.ModCtrl))
		deadline := time.After(5 * time.Second)
		for !v.interaction.idle() {
			select {
			case result := <-v.loader.results:
				v.acceptLoad(result)
			case <-deadline:
				t.Fatal("calculation did not complete")
			}
		}
	}
	calculate()
	if got, _ := p.status(); got != "4 files · 10 B" {
		t.Fatalf("measured total=%q", got)
	}
	records := readSizeRecords(t, dataDir)
	if len(records) != 2 {
		t.Fatalf("cache entries=%v", records)
	}
	for _, record := range records {
		if record.CalculatedAt.IsZero() || record.Files < 1 {
			t.Fatal("measurement lacks timestamp or file count")
		}
	}
	p.feedback.Dismiss()
	if got, _ := p.status(); got != "≈4 files · 10 B" {
		t.Fatalf("cached selection=%q", got)
	}
	for i := range v.panels {
		other := &v.panels[i]
		selectName(t, other, "alpha")
		if got := metadataSize.value(other.displayEntry(other.items[other.selected])); got != "≈3 B" {
			t.Fatalf("panel %d size=%q", i, got)
		}
	}
	p.clearMarks()
	if got, _ := p.status(); got != "≈2 files · 3 B" {
		t.Fatalf("folder footer=%q", got)
	}
	p.metadata = metadataSize
	if got, _ := p.status(); got != "≈2 files" {
		t.Fatalf("size duplicated in footer=%q", got)
	}
	p.sortByMetadata = true
	p.toggleMark(p.selected)
	v.applyFolderSizes(v.folderSizes.clone())
	if p.items[p.selected].file.Name != "alpha" || len(p.marks) != 1 || p.items[0].file.Name != "beta" {
		t.Fatal("size sort lost selection/marks or ignored cached sizes")
	}
	p.clearMarks()
	p.metadata = metadataExtension
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send(pressed(key.NameDeleteBackward))
	waitLoaded(t, v, 0)
	selectName(t, p, "alpha")
	if got, _ := p.status(); got != "≈2 files · 3 B" {
		t.Fatalf("navigation lost cache=%q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha", "file"), []byte("changed size"), 0600); err != nil {
		t.Fatal(err)
	}
	send(modified("R", key.ModCtrl))
	waitLoaded(t, v, 0)
	selectName(t, p, "alpha")
	if got := p.displayEntry(p.items[p.selected]).size; got != "≈3 B" {
		t.Fatal("directory refresh invalidated measurement")
	}
	calculate()
	p.feedback.Dismiss()
	if got, _ := p.status(); got != "≈2 files · 12 B" {
		t.Fatalf("explicit calculation reused cache=%q", got)
	}
	v.finishOperations()
	reopened := start()
	p = &reopened.panels[0]
	selectName(t, p, "alpha")
	if got := p.displayEntry(p.items[p.selected]).size; got != "≈12 B" {
		t.Fatalf("restart lost cache=%q", got)
	}
	p.toggleMark(p.selected)
	selectName(t, p, "uncalculated")
	p.toggleMark(p.selected)
	if got, _ := p.status(); got != "" {
		t.Fatal("partial marked total shown")
	}
	p.clearMarks()
	selectName(t, p, "alpha")
	record := reopened.folderSizes[folderSizeKey(filepath.Join(root, "alpha"))]
	record.CalculatedAt = time.Now().Add(-folderSizeLifetime)
	reopened.folderSizes[folderSizeKey(record.Path)] = record
	if got, _ := p.status(); got != "" {
		t.Fatal("expired folder footer shown")
	}
	if got := p.displayEntry(p.items[p.selected]).size; got != "—" {
		t.Fatal("expired column shown")
	}
	reopened.expireFolderSizes(time.Now())
	if _, exists := reopened.folderSizes[folderSizeKey(record.Path)]; exists {
		t.Fatal("expired record retained in memory")
	}
}

func TestFolderSizeCacheFailedBatchesDoNotPublish(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		dir := t.TempDir()
		paths := []string{filepath.Join(dir, "first"), filepath.Join(dir, "second")}
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		l := &directoryLoader{folderCacheDir: dir, folderSizes: make(folderSizeCache)}
		l.calculateSize = func(context.Context, []string) (browser.SizeResult, error) {
			calls++
			if calls == 2 {
				if cancelled {
					cancel()
				} else {
					return browser.SizeResult{}, fs.ErrPermission
				}
			}
			return browser.SizeResult{Bytes: 3, Files: 1}, nil
		}
		total, cache, err := l.calculateCachedSize(&sizeSession{ctx: ctx, paths: paths, folders: map[string]bool{paths[0]: true, paths[1]: true}})
		cancel()
		if err == nil || cache != nil || total != (browser.SizeResult{}) || len(l.folderSizes) != 0 {
			t.Fatal("failed batch published partial cache")
		}
		if _, err := os.Stat(filepath.Join(dir, "cache", "folders.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("failed batch wrote cache")
		}
	}
}

func TestFolderSizeCacheExpiryKeepsSizeSortAndMarks(t *testing.T) {
	v := selectionView()
	p := &v.panels[0]
	p.path = t.TempDir()
	p.items = append(p.items, entry{name: "z-folder", file: browser.Entry{Name: "z-folder", Directory: true}})
	p.metadata = metadataSize
	p.sortByMetadata = true
	record := folderSize{Path: filepath.Join(p.path, "z-folder"), Bytes: 20, CalculatedAt: time.Now()}
	v.folderSizes[folderSizeKey(record.Path)] = record
	p.resort()
	selectName(t, p, "z-folder")
	p.toggleMark(p.selected)
	if !strings.HasPrefix(p.displayEntry(p.items[0]).size, "≈") {
		t.Fatal("known folder not sorted first")
	}
	before := slices.Clone(p.selectedNames())
	record.CalculatedAt = record.CalculatedAt.Add(-time.Hour)
	v.folderSizes[folderSizeKey(record.Path)] = record
	v.expireFolderSizes(time.Now())
	if p.items[0].file.Name != "alpha" || p.items[p.selected].file.Name != "z-folder" || !slices.Equal(before, p.selectedNames()) {
		t.Fatal("expiry lost order, selection or marks")
	}
}
