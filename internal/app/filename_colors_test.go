package app

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/dannyben/commando/internal/browser"
)

func TestFilenameColorIdentities(t *testing.T) {
	cases := []struct {
		name string
		dir  bool
		key  string
	}{
		{"notes.TXT", false, "extension:.txt"},
		{"notes.txt", false, "extension:.txt"},
		{"archive.tar.gz", false, "extension:.gz"},
		{".env", false, "category:._extless_dotfile"},
		{".config.json", false, "extension:.json"},
		{"README", false, "category:._extless"},
		{"name.", false, "category:._extless"},
		{"folder.txt", true, "category:._dir"},
		{"real._dir", false, "extension:._dir"},
	}
	for _, tc := range cases {
		if got := filenameColorKey(browser.Entry{Name: tc.name, Directory: tc.dir}); got != tc.key {
			t.Errorf("%s (folder=%v): got %s, want %s", tc.name, tc.dir, got, tc.key)
		}
	}
	s := defaultFilenameColors
	s.salt = 0
	before := s.filename(browser.Entry{Name: "a.txt"})
	_ = s.filename(browser.Entry{Name: "new.neverseen"})
	if before != s.filename(browser.Entry{Name: "elsewhere.TXT"}) {
		t.Fatal("color changed with filename, extension case, or encounter order")
	}
	// Freeze the salted mapping across restarts and platforms.
	if got := s.filename(browser.Entry{Name: "folder", Directory: true}).h; math.Abs(got-160.33905647800177) > 1e-9 {
		t.Fatalf("folder hue changed: %v", got)
	}
}

func TestFilenameHueSaltShufflesRatherThanRotates(t *testing.T) {
	s := defaultFilenameColors
	s.salt = 0
	txt := s.filename(browser.Entry{Name: "a.txt"})
	png := s.filename(browser.Entry{Name: "a.png"})
	s.salt = 1
	nextTXT := s.filename(browser.Entry{Name: "a.txt"})
	nextPNG := s.filename(browser.Entry{Name: "a.png"})
	if nextTXT.h == txt.h || nextPNG.h == png.h ||
		math.Abs(wrapHue(nextTXT.h-txt.h)-wrapHue(nextPNG.h-png.h)) < 1e-9 {
		t.Fatal("salt did not change extension color relationships")
	}
	if nextTXT.l != txt.l || nextTXT.c != txt.c || nextPNG.l != png.l || nextPNG.c != png.c {
		t.Fatal("salt changed lightness or chroma")
	}
	s.salt = 0
	if s.filename(browser.Entry{Name: "another.PNG"}) != png {
		t.Fatal("restoring salt did not restore the original color")
	}
	s.salt = 1000000
	if s.filename(browser.Entry{Name: "a.png"}).h == png.h {
		t.Fatal("large salt was discarded")
	}
}

func TestFilenameColorModifiers(t *testing.T) {
	s := defaultFilenameColors
	base := s.filename(browser.Entry{Name: "plain.json"})
	both := s.filename(browser.Entry{Name: ".hidden.json", Hidden: true})
	if both.h != base.h || math.Abs(both.l-(base.l+s.hiddenLightnessOffset+s.dotfileLightnessOffset)) > 1e-9 ||
		math.Abs(both.c-base.c*s.hiddenChromaMultiplier*s.dotfileChromaMultiplier) > 1e-9 {
		t.Fatalf("overlapping modifiers lost hue or composed incorrectly: %+v", both)
	}
	s.hiddenLightnessOffset, s.dotfileLightnessOffset = 0, 0
	s.hiddenChromaMultiplier, s.dotfileChromaMultiplier = 1, 1
	if got := s.filename(browser.Entry{Name: ".hidden.json", Hidden: true}); got != base {
		t.Fatalf("neutral modifiers changed the color: %+v", got)
	}
	s.hiddenLightnessOffset, s.hiddenChromaMultiplier = 2, -1
	if got := s.filename(browser.Entry{Name: "hidden.json", Hidden: true}); got.l != 1 || got.c != 0 {
		t.Fatalf("invalid lightness/chroma escaped clamping: %+v", got)
	}
	s.hiddenLightnessOffset = -2
	if got := s.filename(browser.Entry{Name: "hidden.json", Hidden: true}); got.l != 0 {
		t.Fatalf("negative lightness escaped clamping: %+v", got)
	}
}

func TestOKLCHConversionAndGamut(t *testing.T) {
	// Independent conversion example from W3C CSS Color 4, section 11.
	if got := (oklchColor{0.65, 0.15, 270}).rgb(); got != (color.NRGBA{R: 0x6c, G: 0x88, B: 0xea, A: 255}) {
		t.Fatalf("reference conversion: got %v", got)
	}
	for _, l := range []float64{0, 0.1, 0.56, 0.9, 1} {
		for h := 0.0; h < 360; h += 5 {
			original := oklchColor{l, 0.5, h}
			mapped := original.gamutMapped()
			if !inSRGB(mapped.linearRGB()) || mapped.l != l || mapped.h != h || mapped.c > original.c {
				t.Fatalf("gamut mapping changed lightness/hue or left sRGB: %+v", mapped)
			}
			if mapped.rgb().A != 255 {
				t.Fatal("generated color is not opaque")
			}
		}
	}
	if (oklchColor{0, 0, 0}).rgb() != rgb(0) || (oklchColor{1, 0, 0}).rgb() != rgb(0xffffff) {
		t.Fatal("black/white conversion failed")
	}
}

func TestEntryColorPrecedence(t *testing.T) {
	s, theme := defaultFilenameColors, defaultTheme
	file := browser.Entry{Name: ".example.json", Hidden: true}
	bg, name, metadata := theme.entryColors(file, false, false)
	if bg != theme.Colors.Surface || name != s.filename(file).rgb() || metadata != name {
		t.Fatal("ordinary row changed surface or lost matching filename/metadata colors")
	}
	activeBG, activeName, activeMetadata := theme.entryColors(file, true, false)
	if activeBG != s.activeBackground(s.filename(file)).rgb() || activeName != theme.Colors.Surface || activeMetadata != activeName {
		t.Fatal("active row lost adjusted type background or panel-surface foreground")
	}
	// Inversion follows the configured surface, rather than hard-coding white.
	theme.Colors.Surface = rgb(0xeeeeee)
	_, activeName, activeMetadata = theme.entryColors(file, true, false)
	if activeName != theme.Colors.Surface || activeMetadata != activeName {
		t.Fatal("active filename/metadata did not follow the panel surface")
	}
	theme = defaultTheme
	for _, active := range []bool{false, true} {
		cutBG, cutName, cutMetadata := theme.entryColors(file, active, true)
		wantBG, wantName := bg, theme.Colors.CutFilename
		if active {
			wantBG, wantName = activeBG, theme.Colors.SelectedCutFilename
		}
		if cutBG != wantBG || cutName != wantName || cutMetadata != cutName {
			t.Fatal("cut precedence changed the type background or mismatched filename/metadata")
		}
		parentBG, parentName, parentMetadata := theme.entryColors(browser.Entry{Name: "..", Parent: true}, active, false)
		folderBG, folderName, folderMetadata := theme.entryColors(browser.Entry{Name: "folder", Directory: true}, active, false)
		if parentBG != folderBG || parentName != folderName || parentMetadata != folderMetadata {
			t.Fatal("parent row did not match ordinary directory colors")
		}
		for _, special := range []browser.Entry{{Name: "C:", Path: `C:\`}} {
			specialBG, specialName, specialMetadata := theme.entryColors(special, active, false)
			wantBG, wantName = theme.Colors.Surface, theme.Colors.Filename
			if active {
				wantBG, wantName = theme.Colors.Selected, theme.Colors.SelectedName
			}
			if specialBG != wantBG || specialName != wantName || specialMetadata != specialName {
				t.Fatal("drive styling changed or mismatched filename/metadata")
			}
		}
	}
}

func TestActiveBackgroundLightness(t *testing.T) {
	value := oklchColor{0.56, 0.15, 120}
	for _, offset := range []float64{0, 0.1, -0.1, 2, -2} {
		s := defaultFilenameColors
		s.activeBackgroundLightnessOffset = offset
		got := s.activeBackground(value)
		if got.l != clampLightness(value.l+offset) || got.c != value.c || got.h != value.h {
			t.Fatalf("offset %v changed hue/chroma or failed to clamp lightness: %+v", offset, got)
		}
	}
}

// Optional rendered review exercises category/modifier/mark/cut combinations
// using the production file list, without reading folders or saving state.
func TestFilenameColorPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_COLORS")
	if directory == "" {
		t.Skip("optional headless filename color review")
	}
	files := []browser.Entry{
		{Name: "..", Parent: true, Directory: true},
		{Name: "folder.with.dots", Directory: true},
		{Name: "README"}, {Name: ".env"},
		{Name: "normal.json"}, {Name: "hidden.json", Hidden: true},
		{Name: ".normal.json"}, {Name: ".hidden.json", Hidden: true},
		{Name: ".folder", Directory: true},
		{Name: "hidden-folder", Directory: true, Hidden: true},
		{Name: "cut.json"}, {Name: "marked.txt"},
		{Name: "photo.png"}, {Name: "archive.tar.gz"},
		{Name: "real._dir"}, {Name: "document.pdf"},
	}
	for _, review := range []struct {
		name string
		row  int
	}{{"folder", 1}, {"hidden-dotfile", 7}, {"cut", 10}} {
		panels := samplePanels()
		for i := range panels {
			panels[i].items = make([]entry, len(files))
			panels[i].selected = review.row
			for row, file := range files {
				panels[i].items[row] = entry{name: file.Name, size: "1 KB", file: file}
			}
		}
		v := newView(panels)
		for i := range v.panels {
			v.panels[i].toggleMark(11)
		}
		v.cutClipboard = browser.FileClipboard{Move: true, Paths: []string{filepath.Join(v.panels[0].path, "cut.json")}}
		if err := renderViewPNG(filepath.Join(directory, "colors-"+review.name+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDisabledAutoColorUsesSemanticThemeColors(t *testing.T) {
	theme := defaultTheme
	generated := defaultFilenameColors
	generated.enabled = false
	theme.FilenameColors = &generated
	theme.Colors.Metadata = rgb(0x123456)
	theme.Colors.SelectedMetadata = rgb(0xabcdef)
	for _, file := range []browser.Entry{
		{Name: "..", Directory: true, Parent: true}, {Name: "folder", Directory: true},
		{Name: "notes.txt"}, {Name: ".env"}, {Name: "hidden.json", Hidden: true},
		{Name: "C:", Path: `C:\`},
	} {
		for _, active := range []bool{false, true} {
			for _, cut := range []bool{false, true} {
				bg, name, metadata := theme.entryColors(file, active, cut)
				wantBG, wantName, wantMetadata := theme.Colors.Surface, theme.Colors.Filename, theme.Colors.Metadata
				if active {
					wantBG, wantName, wantMetadata = theme.Colors.Selected, theme.Colors.SelectedName, theme.Colors.SelectedMetadata
				}
				if cut {
					wantName = theme.Colors.CutFilename
					if active {
						wantName = theme.Colors.SelectedCutFilename
					}
					wantMetadata = wantName
				}
				if bg != wantBG || name != wantName || metadata != wantMetadata {
					t.Fatalf("%s active=%v cut=%v did not use semantic colors", file.Name, active, cut)
				}
			}
		}
	}
}
