package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dannyben/commando/internal/settings"
)

const dataFolderPointer = "data-folder.txt"

var errDataFolderPath = errors.New("choose an absolute folder")

// The default folder remains the bootstrap location. The active folder stays
// fixed for the running session; chosen is the folder for the next launch.
type dataFolderStore struct{ base, active, chosen string }

// The bootstrap pointer owns the location. A destination's existing settings
// may name another folder; loading them must never cause another redirect.
func (s *dataFolderStore) restoreLocation(values *settings.Values) {
	values.Reset(settingDataFolder)
	if !samePath(s.active, s.base) {
		_ = values.Set(settings.CatalogueDefault, settingDataFolder, settings.Value{Text: s.active})
	}
}

func resolveDataFolder(base string) (string, error) {
	data, err := os.ReadFile(filepath.Join(base, dataFolderPointer))
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return base, err
	}
	path := strings.TrimRight(string(data), "\r\n")
	if !filepath.IsAbs(path) {
		return base, errDataFolderPath
	}
	info, err := os.Stat(path)
	if err != nil {
		return base, err
	}
	if !info.IsDir() {
		return base, fmt.Errorf("data location is not a folder")
	}
	return filepath.Clean(path), nil
}

func (s *dataFolderStore) save(values settings.Values, state sessionState) (string, error) {
	target := values.Get(settings.CatalogueDefault, settingDataFolder).Text
	if target == "" {
		target = s.base
	}
	if !filepath.IsAbs(target) {
		return "", errDataFolderPath
	}
	target = filepath.Clean(target)
	// Folder identity handles alternate spellings, symlinks and drive aliases.
	if sameDataFolder(target, s.active) {
		target = s.active
	} else if sameDataFolder(target, s.base) {
		target = s.base
	}
	if !samePath(target, s.chosen) {
		if err := os.MkdirAll(target, 0700); err != nil {
			return "", err
		}
	}
	if err := settings.Save(s.active, values); err != nil {
		return "", err
	}
	if samePath(target, s.chosen) {
		if !samePath(target, s.active) {
			return "Restart Commando", nil
		}
		return "", nil
	}
	if !samePath(target, s.active) {
		if err := saveState(s.active, state); err != nil {
			return "", err
		}
		if err := copyDataFiles(s.active, target); err != nil {
			return "", err
		}
	}
	// Publish the startup location last. A failed copy leaves the running and
	// next-launch locations unchanged; completed destination files are retained.
	if samePath(target, s.base) {
		if err := os.Remove(filepath.Join(s.base, dataFolderPointer)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	} else if err := saveDataFolderPointer(s.base, target); err != nil {
		return "", err
	}
	s.chosen = target
	if !samePath(target, s.active) {
		return "Restart Commando", nil
	}
	return "", nil
}

func sameDataFolder(a, b string) bool {
	first, err := os.Stat(a)
	if err != nil {
		return false
	}
	second, err := os.Stat(b)
	return err == nil && os.SameFile(first, second)
}

func copyDataFiles(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		// Only application-owned data, never unrelated files or the bootstrap
		// pointer. Register new persisted data here when adding another store.
		managed := name == "settings.toml" || name == "state.json" || name == "shell-menu.jsonl" ||
			strings.HasPrefix(name, "transfer-trace-") && strings.HasSuffix(name, ".jsonl")
		if !managed {
			continue
		}
		if err := copyDataFile(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyDataFile(source, destination string) error {
	// Existing entries win, including links. Never open them for writing.
	if _, err := os.Lstat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("data source is not a regular file")
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// Bound diagnostic-log copying to its captured size even if it is growing.
	_, copyErr := io.CopyN(out, in, info.Size())
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		// Remove only the incomplete file created by this call, never a skipped
		// existing entry or a completed copy from an earlier attempt.
		os.Remove(destination)
	}
	return copyErr
}

func saveDataFolderPointer(base, target string) error {
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(base, ".data-folder-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(target + "\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(base, dataFolderPointer))
}
