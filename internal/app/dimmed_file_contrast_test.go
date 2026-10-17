package app

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestFileContrastRolesKeepSelectedProtectedAndRespectAutoColors(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		theme := defaultTheme
		colors := defaultFilenameColors
		colors.enabled = enabled
		theme.FilenameColors = &colors
		for _, file := range []browser.Entry{
			{Name: "notes.txt"}, {Name: "hidden.txt", Hidden: true}, {Name: ".notes.txt"}, {Name: ".hidden.txt", Hidden: true},
			{Name: "hidden-folder", Directory: true, Hidden: true}, {Name: "..", Parent: true, Directory: true}, {Name: "C:", Path: `C:\`},
		} {
			for _, selected := range []bool{false, true} {
				for _, cut := range []bool{false, true} {
					got := theme.entryTextRoles(file, selected, cut)
					if selected {
						if got.filename != contrastPrimary || got.metadata != contrastPrimary {
							t.Fatal("selected entry lost primary contrast", file.Name)
						}
						continue
					}
					dimmed := !file.Parent && file.Path == "" && (cut || enabled && (file.Hidden || file.Name[0] == '.'))
					if dimmed {
						if got.filename != contrastDimmedFile || got.metadata != contrastDimmedFile {
							t.Fatal("dimmed filename and metadata disagree", file.Name, enabled, cut)
						}
					} else {
						if got.filename != contrastPrimary {
							t.Fatal("ordinary filename weakened", file.Name)
						}
						wantMetadata := contrastSecondary
						if cut || enabled && file.Path == "" {
							wantMetadata = contrastPrimary
						}
						if got.metadata != wantMetadata {
							t.Fatal("ordinary metadata changed", file.Name)
						}
					}
				}
			}
		}
	}
}

func TestDimmedFileGuardAllowsSofterTextAndStillEnforcesItsFloor(t *testing.T) {
	for _, theme := range builtInThemes {
		v := newPreviewView()
		v.settingsValues = theme.values.Clone()
		v.applySettings()
		surface := v.appearance.Colors.Surface
		// Coincident text forces both policies to resolve to their contrast floor.
		dim := v.resolvedTextFor(contrastDimmedFile, surface, surface)
		normal := v.resolvedTextFor(contrastPrimary, surface, surface)
		dimScore := math.Abs(perceptualContrast(dim, surface))
		normalScore := math.Abs(perceptualContrast(normal, surface))
		if dimScore < 30 || dimScore > 32 || normalScore < 75 || normalScore-dimScore < 40 {
			t.Fatalf("%s: dimming not distinct while guarded: dim=%g normal=%g", theme.name, dimScore, normalScore)
		}
	}
}

func TestDimmedFileRendererUsesSameRoleForNameAndMetadata(t *testing.T) {
	for _, mode := range []metadataColumn{metadataNone, metadataExtension, metadataSize, metadataModified, metadataCreated} {
		v := newPreviewView()
		v.settingsValues = builtInThemes[0].values.Clone()
		_ = v.settingsValues.Set(settings.CatalogueDefault, settingHiddenLightness, settings.Value{Number: 100})
		v.applySettings()
		file := previewFile("hidden-notes.txt")
		file.Hidden = true
		bg, name, metadata := v.appearance.entryColors(file, false, false)
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 40))}
		v.fileRowSurface(gtx, gtx.Constraints.Max, bg, color.NRGBA{}, func(gtx layout.Context) layout.Dimensions {
			return v.rowContents(gtx, entry{name: file.Name, size: "1 KB", file: file}, name, metadata, mode, v.appearance.entryTextRoles(file, false, false), false)
		})
		dimKey := textContrastPair{name, bg, themeContrastRules[contrastDimmedFile], color.NRGBA{}}
		if _, ok := v.textContrast[dimKey]; !ok {
			t.Fatal("filename did not use dimmed rule", mode)
		}
		for _, role := range []contrastRole{contrastPrimary, contrastSecondary} {
			if _, ok := v.textContrast[textContrastPair{name, bg, themeContrastRules[role], color.NRGBA{}}]; ok {
				t.Fatal("name or matching metadata was guarded a second time with a stronger role", mode)
			}
		}
		if v.textRole != contrastPrimary {
			t.Fatal("file role leaked into surrounding UI")
		}
	}
}

func TestDimmedFilePreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional dimmed file previews")
	}
	for i, theme := range builtInThemes {
		v := newPreviewView()
		v.settingsValues = theme.values.Clone()
		offset := 100.0
		if i == 1 {
			offset = -100
		}
		_ = v.settingsValues.Set(settings.CatalogueDefault, settingHiddenLightness, settings.Value{Number: offset})
		_ = v.settingsValues.Set(settings.CatalogueDefault, settingHiddenSaturation, settings.Value{Number: 0.25})
		v.applySettings()
		v.startSettingsWidget("appearance.autocolor.special_files")
		if err := renderViewPNG(filepath.Join(dir, theme.name+"-hidden-dimmed.png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
