package app

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/settings"
)

func TestExtensionCommandNameKeepsConfiguredOrder(t *testing.T) {
	for _, tc := range []struct {
		extensions []string
		want       string
	}{
		{nil, ""},
		{[]string{"*"}, "*"},
		{[]string{".TXT", "md"}, ".TXT md"},
		{[]string{"png", "jpg", "tiff"}, "png jpg tiff"},
		{[]string{"png", "jpg", "tiff", "gif"}, "png jpg tiff +1"},
		{[]string{"png", "jpg", "tiff", "gif", "webp"}, "png jpg tiff +2"},
		{append([]string{"png", "jpg", "bmp"}, slices.Repeat([]string{"ext"}, 10)...), "png jpg bmp +10"},
	} {
		before := slices.Clone(tc.extensions)
		if got := extensionCommandName(tc.extensions); got != tc.want || !slices.Equal(before, tc.extensions) {
			t.Fatalf("extensions %q became %q; want %q without changing order", before, got, tc.want)
		}
	}
}

func TestCompactProgramRulesRetainHiddenSearchAndIdentity(t *testing.T) {
	for _, collection := range []string{settings.EditorCollection, settings.CompareCollection} {
		for _, prefix := range []string{"", "@"} {
			t.Run(collection+prefix, func(t *testing.T) {
				v := newPreviewView()
				v.applySettings()
				extensions := []string{"PNG", "jpg", "tiff", "gif", "webp"}
				args := settings.SelectedFilesToken
				if collection == settings.CompareCollection {
					args = settings.FirstFileToken + " " + settings.SecondFileToken
				}
				rows := []map[string]any{
					{"id": "images", "ext": extensions, "path": "tool.exe", "args": args},
					{"id": "other", "ext": []string{"PNG", "jpg", "tiff", "bmp", "ico"}, "path": "other.exe", "args": args},
				}
				v.settingsValues.SetCollection(collection, rows)
				id := "settings." + collection + ".rules.images"
				setTestShortcut(t, v, id, 0, "Ctrl+Y")
				query := prefix + "webp"
				send, _ := panelInputFrames(v)
				send(pressed("`"), key.EditEvent{Text: query})
				p := v.paletteInteraction()
				if p == nil || len(p.results) != 1 || p.results[0].command.id != id {
					t.Fatal("hidden extension did not find the correct rule", p)
				}
				c := p.results[0].command
				if c.name != "PNG jpg tiff +2" || c.shortcut() != "Ctrl+Y" {
					t.Fatal("compact label lost authored order, count or binding", c)
				}
				marker := utf8.RuneCountInString(c.label()[:strings.LastIndex(c.label(), "+")])
				if !slices.Equal(p.results[0].match.Indices, []int{marker, marker + 1}) {
					t.Fatal("hidden hit did not highlight +N", p.results[0].match.Indices)
				}
				for _, search := range []string{"PNG", "png webp", "+2"} {
					send(replacePalette(v, prefix+search))
					index := slices.IndexFunc(p.results, func(result commandMatch) bool { return result.command.id == id })
					if index < 0 {
						t.Fatal("visible, combined or count search lost the rule", search)
					}
					for _, match := range p.results[index].match.Indices {
						if match < 0 || match >= utf8.RuneCountInString(p.results[index].command.label()) {
							t.Fatal("highlight points beyond the compact label", search, match)
						}
					}
				}
				send(replacePalette(v, prefix+"ico"))
				if !slices.ContainsFunc(p.results, func(result commandMatch) bool { return result.command.id == "settings."+collection+".rules.other" }) {
					t.Fatal("rules with identical summaries lost their separate identities")
				}
				send(replacePalette(v, query), modified("E", key.ModCtrl))
				if widget := v.settingsInteraction(); widget == nil || widget.bindingCommand.id != id || !strings.HasSuffix(widget.title, "PNG jpg tiff +2 › Shortcuts") {
					t.Fatal("compact rule did not use its original shortcut identity")
				}
				send(pressed(key.NameEscape))
				send(pressed(key.NameReturn))
				if widget := v.settingsInteraction(); widget == nil || widget.collection != collection || widget.item != 0 || v.settingText(collection+".ext") != strings.Join(extensions, " ") {
					t.Fatal("opening the compact rule did not reveal all configured extensions")
				}
				send(modified(key.NameReturn, key.ModCtrl))
				if !v.interaction.idle() || !reflect.DeepEqual(v.settingsValues.Collection(collection)[0]["ext"], extensions) {
					t.Fatal("saving rewrote or dropped configured extensions")
				}
				send(modified("Y", key.ModCtrl))
				if widget := v.settingsInteraction(); widget == nil || widget.item != 0 || v.settingText(collection+".ext") != strings.Join(extensions, " ") {
					t.Fatal("shortening changed the assigned rule shortcut")
				}
			})
		}
	}
}

func TestCompactProgramRulePreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if dir == "" {
		t.Skip("optional compact program-rule previews")
	}
	for _, theme := range builtInThemes {
		for _, width := range []int{320, 800} {
			for _, prefix := range []string{"", "@"} {
				v := newPreviewView()
				v.panels = v.panels[:1]
				v.settingsValues = theme.values.Clone()
				v.settingsValues.SetCollection(settings.EditorCollection, []map[string]any{{
					"id": "images", "ext": []string{"png", "jpg", "tiff", "gif", "webp", "bmp", "ico", "svg", "avif", "heic", "raw", "psd", "xcf"},
					"path": "images.exe", "args": settings.SelectedFilesToken,
				}})
				v.applySettings()
				v.startPalette(layout.Context{Ops: &op.Ops{}})
				v.commandSearch.SetText(prefix + "webp")
				v.paletteInteraction().filter(prefix + "webp")
				name := fmt.Sprintf("palette-rule-%s-%d-%s.png", theme.name, width, prefix)
				if err := renderSizedViewPNG(filepath.Join(dir, name), v, image.Pt(width, 480)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
