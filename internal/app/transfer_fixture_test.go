package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/dannyben/commando/internal/browser"
)

// Tests asserting resolved operation paths need physical temporary roots.
// Windows may return an 8.3 spelling from TempDir while the shell expands it.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// A disposable-fixture stand-in for the external Windows backend. It has no
// conflict, cancellation, staging or fallback policy; those belong to Windows.
// UI tests exercise dispatch and reconciliation, rather than a second engine.
func fixtureTransfer(request browser.TransferRequest) (browser.TransferResult, error) {
	target, err := resolveFileLocation(request.Directory)
	result := browser.TransferResult{Directory: target, Attempted: true}
	if err != nil {
		return result, err
	}
	names := request.Names
	if request.Duplicate && len(names) == 0 {
		plan, err := browser.PlanDuplicate(request.Paths, request.Directory)
		if err != nil {
			return result, err
		}
		names = plan.Names
	}
	for i, original := range request.Paths {
		parent, err := resolveFileLocation(filepath.Dir(original))
		if err != nil {
			return result, err
		}
		source := filepath.Join(parent, filepath.Base(original))
		name := filepath.Base(source)
		if len(names) > 0 {
			name = names[i]
		}
		destination := filepath.Join(target, name)
		if request.Move {
			err = os.Rename(source, destination)
		} else {
			err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				relative, err := filepath.Rel(source, path)
				if err != nil {
					return err
				}
				dst := filepath.Join(destination, relative)
				if entry.IsDir() {
					return os.Mkdir(dst, 0700)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return os.WriteFile(dst, data, 0600)
			})
		}
		if err != nil {
			return result, err
		}
		info, err := os.Lstat(destination)
		if err != nil {
			return result, err
		}
		file := browser.Entry{Name: name, Directory: info.IsDir(), Bytes: info.Size(), SizeKnown: info.Mode().IsRegular(), Modified: info.ModTime()}
		result.Items = append(result.Items, browser.TransferredItem{Source: original, Entry: file, Moved: request.Move})
		result.Summary.Requested = len(request.Paths)
		result.Summary.Completed++
		result.Changes = append(result.Changes, browser.TransferredPath{Source: source, Destination: destination, Directory: info.IsDir(), Moved: request.Move})
	}
	return result, nil
}

func writeBatchFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertBatchFile(t *testing.T, path, want string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Fatalf("%s: %q %v", path, data, err)
	}
}
