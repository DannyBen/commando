// export-icons captures original Windows shell icons for Showroom fixtures.
package main

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dannyben/commando/internal/browser"
)

type exportedIcon struct {
	Name      string            `json:"name"`
	Directory bool              `json:"directory"`
	Images    map[string]string `json:"images"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: export-icons.exe INPUT_FOLDER OUTPUT_FOLDER")
		os.Exit(1)
	}
	if err := exportIcons(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func exportIcons(input, output string) error {
	input, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if strings.EqualFold(input, output) {
		return fmt.Errorf("use a separate output folder")
	}
	entries, err := os.ReadDir(input)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("input folder is empty")
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	read := browser.NewFileIconReader()
	var manifest []exportedIcon
	for _, entry := range entries {
		path := filepath.Join(input, entry.Name())
		if strings.EqualFold(path, output) {
			continue
		}
		item := exportedIcon{Name: entry.Name(), Directory: entry.IsDir(), Images: make(map[string]string)}
		for _, size := range []int{16, 32, 48} {
			img := read(path, size)
			if img == nil {
				return fmt.Errorf("cannot read icon: %s", entry.Name())
			}
			name := fmt.Sprintf("%s.%d.png", entry.Name(), size)
			file, err := os.Create(filepath.Join(output, name))
			if err != nil {
				return err
			}
			encodeErr := png.Encode(file, img)
			closeErr := file.Close()
			if encodeErr != nil {
				return encodeErr
			}
			if closeErr != nil {
				return closeErr
			}
			item.Images[strconv.Itoa(size)] = name
		}
		manifest = append(manifest, item)
		fmt.Println("Exported", entry.Name())
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "manifest.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("Saved %d icons at 16, 32 and 48 pixels to %s\n", len(manifest), output)
	return nil
}
