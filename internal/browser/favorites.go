package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Favorites are ordinary Windows shortcuts in the application's data folder.
// Reading a missing store does not create it or modify any shortcut.
func ReadFavorites(directory string) ([]Entry, error) {
	return readFavorites(directory, ShortcutTarget)
}

func readFavorites(directory string, resolve func(string) (string, error)) ([]Entry, error) {
	listing, err := ReadDirectory(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, file := range listing.Entries {
		if file.Directory || !strings.EqualFold(filepath.Ext(file.Name), ".lnk") {
			continue
		}
		file.Favorite, file.Path = true, filepath.Join(directory, file.Name)
		file.Target, _ = resolve(file.Path)
		entries = append(entries, file)
	}
	return entries, nil
}

func CreateFavorite(directory, name, target string) (Entry, error) {
	return createFavorite(directory, name, target, shortcutData)
}

func createFavorite(directory, name, target string, encode func(string) ([]byte, error)) (Entry, error) {
	if err := ValidateName(name); err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return Entry{}, err
	}
	if !info.IsDir() {
		return Entry{}, fs.ErrInvalid
	}
	data, err := encode(target)
	if err != nil {
		return Entry{}, err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return Entry{}, err
	}
	file, err := CreateFileData(directory, name+".lnk", data)
	if err == nil {
		file.Favorite, file.Target, file.Path = true, target, filepath.Join(directory, file.Name)
	}
	return file, err
}
