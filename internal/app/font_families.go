package app

import (
	"os"
	"strings"

	"github.com/dannyben/commando/internal/settings"
	fontapi "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
)

// Font footprints normalize names for matching. Read just the metadata of one
// face per family to retain the installed font's spelling in suggestions.
func readFontFamilies() []string {
	cache, _ := os.UserCacheDir()
	fonts, err := fontscan.SystemFonts(nil, cache)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	var buffer []byte
	for _, fp := range fonts {
		if seen[fp.Family] {
			continue
		}
		file, err := os.Open(fp.Location.File)
		if err != nil {
			continue
		}
		loaders, err := opentype.NewLoaders(file)
		if err == nil && int(fp.Location.Index) < len(loaders) {
			var desc fontapi.Description
			desc, buffer = fontapi.Describe(loaders[fp.Location.Index], buffer)
			if desc.Family != "" {
				names = append(names, desc.Family)
				seen[fp.Family] = true
			}
		}
		file.Close()
	}
	return completionCandidates(names)
}

func (v *view) requestFontFamilies() {
	if v.fontFamilies != nil || v.fontFamiliesLoading || v.loader == nil {
		return
	}
	v.fontFamiliesLoading = v.loader.submit(loadRequest{fontFamilies: true, action: actionBrowse})
}

func (v *view) fontFamilyChoices() []string {
	bundled := string(v.appearance.Font[0].Font.Typeface)
	return completionCandidates(append([]string{bundled}, v.fontFamilies...))
}

func (v *view) cycleFontFamily(direction int) {
	s := v.settingsInteraction()
	if s == nil || s.editing || s.pending {
		return
	}
	names := s.completion.candidates
	if len(names) < 2 {
		return
	}
	current := strings.TrimSpace(v.settingText(settingFontFamily))
	index := -1
	for i, name := range names {
		if strings.EqualFold(name, current) {
			index = i
			break
		}
	}
	if index < 0 {
		index = 0
		if direction < 0 {
			index = len(names) - 1
		}
	} else {
		index = (index + direction + len(names)) % len(names)
	}
	if v.setWidgetValue(settingFontFamily, settings.Value{Text: names[index]}) == nil {
		v.applySettings()
		v.announceInteraction(eventValueChanged)
	}
}
