//go:build showroom

package app

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gioui.org/op/paint"
	"github.com/dannyben/commando/internal/browser"
)

// Fixtures supply original artwork only. Production row layout and color
// adjustment remain responsible for every captured icon, without a worker.
func (r showroomRequest) loadIcons(t *testing.T, v *view) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.Icons, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest []struct {
		Name      string            `json:"name"`
		Directory bool              `json:"directory"`
		Images    map[string]string `json:"images"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	icons := make(map[string]map[int]fileIconEntry)
	v.fileIcons = make(map[fileIconKey]fileIconEntry)
	for _, entry := range manifest {
		key := strings.ToLower(filepath.Ext(entry.Name))
		if entry.Directory {
			key = "folder"
		}
		images := make(map[int]fileIconEntry)
		for pixels, name := range entry.Images {
			size, err := strconv.Atoi(pixels)
			if err != nil || size <= 0 || filepath.Base(name) != name {
				t.Fatal("invalid icon manifest")
			}
			file, err := os.Open(filepath.Join(r.Icons, name))
			if err != nil {
				t.Fatal(err)
			}
			img, decodeErr := png.Decode(file)
			file.Close()
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if img.Bounds().Size() != image.Pt(size, size) {
				t.Fatal("icon size does not match manifest")
			}
			icon := fileIconEntry{image: paint.NewImageOp(img), source: img}
			images[size] = icon
			v.fileIcons[sampleFileIconKey(browser.Entry{Name: entry.Name, Directory: entry.Directory}, size)] = icon
		}
		icons[key] = images
		icons[strings.ToLower(entry.Name)] = images
	}
	for folder, files := range r.Folders {
		for _, file := range files {
			key := strings.ToLower(file.Name)
			images := icons[key]
			if images == nil {
				images = icons[strings.ToLower(filepath.Ext(file.Name))]
			}
			if file.Directory {
				images = icons["folder"]
			}
			if len(images) == 0 {
				t.Fatalf("missing icon fixture for %s", file.Name)
			}
			for size, icon := range images {
				v.fileIcons[fileIconKey{path: filepath.Join(folder, file.Name), size: size}] = icon
			}
		}
	}
}
