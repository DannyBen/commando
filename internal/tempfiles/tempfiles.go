// Package tempfiles owns disposable Commando files in system temporary storage.
package tempfiles

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const openRetention = 7 * 24 * time.Hour

func NewSession() (string, error) { return newDirectory("session") }
func NewOpen() (string, error)    { return newDirectory("open") }

func newDirectory(area string) (string, error) {
	directory := filepath.Join(os.TempDir(), "Commando", area)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(directory, "copy-*")
}

// Remove clears read-only copies too. WalkDir never follows symbolic links.
func Remove(directory string) error {
	_ = filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			if info, err := entry.Info(); err == nil && info.Mode().Perm()&0222 == 0 {
				_ = os.Chmod(name, 0600)
			}
		}
		return nil
	})
	return os.RemoveAll(directory)
}

// Clean runs only while the primary instance owns no active operations: at
// startup and after shutdown drains. Locked files are left for the next pass.
// Open copies may outlive Commando; their age limit is a retention policy,
// not evidence that an external viewer has finished using them.
func Clean() { clean(time.Now()) }

func clean(now time.Time) {
	root := filepath.Join(os.TempDir(), "Commando")
	_ = Remove(filepath.Join(root, "session"))
	removeExpired(filepath.Join(root, "open"), "copy-", now)
	// Copies created before the shared temporary directory was introduced.
	removeExpired(os.TempDir(), "commando-archive-", now)
}

func removeExpired(parent, prefix string, now time.Time) {
	entries, _ := os.ReadDir(parent)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) >= openRetention {
			_ = Remove(filepath.Join(parent, entry.Name()))
		}
	}
}
