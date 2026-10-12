package shellprobe

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

// Samples remain after closing: targets can read dropped data asynchronously,
// and keeping the directory allows inspection of transfers and the session log.
func sampleFiles() (string, error) {
	root, err := os.MkdirTemp("", "commando-shell-probe-")
	if err != nil {
		return "", err
	}
	for _, folder := range []string{"source", "source/Folder sample", "destination"} {
		if err := os.Mkdir(filepath.Join(root, filepath.FromSlash(folder)), 0o700); err != nil {
			return "", err
		}
	}
	for name, text := range map[string]string{
		"alpha.txt":                "Commando shell integration sample alpha.\r\n",
		"two words.txt":            "Commando sample with spaces in its name.\r\n",
		"café-日本語.txt":             "Commando Unicode filename sample.\r\n",
		"Folder sample/inside.txt": "Commando folder drag sample.\r\n",
	} {
		if err := os.WriteFile(filepath.Join(root, "source", filepath.FromSlash(name)), []byte(text), 0o600); err != nil {
			return "", err
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			c := color.RGBA{40, 110, 220, 255}
			if (x/16+y/16)%2 != 0 {
				c = color.RGBA{240, 170, 50, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(filepath.Join(root, "source", "image.png"))
	if err != nil {
		return "", err
	}
	encodeErr := png.Encode(f, img)
	closeErr := f.Close()
	if encodeErr != nil {
		return "", encodeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return root, nil
}

// fileList is independent of native APIs and owns current/marked identities.
type fileList struct {
	items   []browser.Entry
	current string
	marks   map[string]bool
}

func (l *fileList) replace(items []browser.Entry) {
	oldIndex := l.index()
	l.items = nil
	for _, item := range items {
		if !item.Parent {
			l.items = append(l.items, item)
		}
	}
	for name := range l.marks {
		found := false
		for _, item := range l.items {
			if item.Name == name {
				found = true
				break
			}
		}
		if !found {
			delete(l.marks, name)
		}
	}
	if l.index() < 0 {
		l.current = ""
		if len(l.items) > 0 {
			l.current = l.items[min(max(oldIndex, 0), len(l.items)-1)].Name
		}
	}
}

func (l *fileList) index() int {
	for i, item := range l.items {
		if item.Name == l.current {
			return i
		}
	}
	return -1
}

func (l *fileList) selectItem(name string, toggle bool) {
	if name == "" {
		return
	}
	l.current = name
	if toggle {
		if l.marks == nil {
			l.marks = make(map[string]bool)
		}
		if l.marks[name] {
			delete(l.marks, name)
		} else {
			l.marks[name] = true
		}
	} else if !l.marks[name] {
		// Clicking/dragging a marked item keeps the group. An unmarked item
		// becomes the sole subject, including when right-clicked.
		clear(l.marks)
	}
}

func (l *fileList) paths(folder string) []string {
	var paths []string
	for _, item := range l.items {
		if l.marks[item.Name] || len(l.marks) == 0 && item.Name == l.current {
			paths = append(paths, filepath.Join(folder, item.Name))
		}
	}
	return paths
}
