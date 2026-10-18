package browser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// CreateShortcuts creates ordinary Windows shell links, without copying or
// moving their targets. Existing destination entries are never replaced.
func CreateShortcuts(paths []string, directory string) (TransferResult, error) {
	return createShortcuts(paths, directory, shortcutData)
}

func createShortcuts(paths []string, directory string, encode func(string) ([]byte, error)) (result TransferResult, err error) {
	prepared, err := prepareTransfer(TransferRequest{Paths: paths, Directory: directory})
	if err != nil {
		return result, err
	}
	result.Directory = prepared.Directory
	result.Summary.Requested = len(paths)
	for i, target := range prepared.Paths {
		var data []byte
		var item Entry
		if _, err = os.Stat(target); err == nil {
			data, err = encode(target)
		}
		if err == nil {
			result.Attempted = true
			for number := 1; ; number++ {
				name := filepath.Base(target) + " - Shortcut"
				if number > 1 {
					name += fmt.Sprintf(" (%d)", number)
				}
				item, err = CreateFileData(prepared.Directory, name+".lnk", data)
				if !errors.Is(err, fs.ErrExist) {
					break
				}
			}
		}
		if err != nil {
			result.Summary.Failed = 1
			result.Summary.Unfinished = len(paths) - i - 1
			return result, err
		}
		result.Items = append(result.Items, TransferredItem{Source: paths[i], Entry: item})
		result.Summary.Completed++
	}
	return result, nil
}
