package app

import (
	"image"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestFileIconsLoadWithoutChangingRowHeightAndReuseResults(t *testing.T) {
	for _, missing := range []bool{false, true} {
		v := newPreviewView()
		v.loader = newDirectoryLoader(browser.ReadLocation, nil)
		var calls atomic.Int32
		v.loader.readFileIcon = func(path string, size int) image.Image {
			calls.Add(1)
			if missing {
				return nil
			}
			return image.NewRGBA(image.Rect(0, 0, size, size))
		}
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 24))}
		item := entry{file: browser.Entry{Name: "sample.exe"}}
		other := entry{file: browser.Entry{Name: "another.exe"}}
		row := func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }
		first := v.withFileIcon(gtx, &v.panels[0], item, false, row)
		v.withFileIcon(gtx, &v.panels[0], other, false, row)
		v.withFileIcon(gtx, &v.panels[0], item, false, row)
		v.submitFileIcons()
		select {
		case result := <-v.loader.results:
			if len(result.request.icons) != 2 || calls.Load() != 2 {
				t.Fatal("visible icons were not resolved in one deduplicated batch")
			}
			v.acceptLoad(result)
		case <-time.After(3 * time.Second):
			v.loader.close()
			t.Fatal("icon load did not complete")
		}
		v.fileIconWanted = v.fileIconWanted[:0]
		second := v.withFileIcon(gtx, &v.panels[0], item, false, row)
		v.withFileIcon(gtx, &v.panels[0], other, false, row)
		v.submitFileIcons()
		if first.Size != image.Pt(400, 24) || second.Size != first.Size || v.fileIconPending || calls.Load() != 2 {
			t.Fatalf("icon changed row size or retried cached result: first=%v second=%v pending=%v", first.Size, second.Size, v.fileIconPending)
		}
		v.finishOperations()
	}
}

func TestParentIconsUseActualParentPathAndSharedCache(t *testing.T) {
	folder := t.TempDir()
	root := filepath.VolumeName(folder) + string(filepath.Separator)
	for _, parent := range []string{folder, root} {
		v := newPreviewView()
		v.panels[0].path = filepath.Join(parent, "child")
		v.loader = newDirectoryLoader(browser.ReadLocation, nil)
		v.loader.readFileIcon = func(path string, size int) image.Image {
			if path != parent {
				t.Errorf("parent icon lookup = %q, want %q", path, parent)
			}
			return image.NewRGBA(image.Rect(0, 0, size, size))
		}
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 24))}
		item := entry{file: browser.Entry{Name: "..", Directory: true, Parent: true}}
		row := func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }
		v.withFileIcon(gtx, &v.panels[0], item, false, row)
		v.submitFileIcons()
		select {
		case result := <-v.loader.results:
			if len(result.request.icons) != 1 || result.icons[0] == nil {
				t.Fatal("parent icon did not use the directory worker")
			}
			v.acceptFileIcons(result)
		case <-time.After(3 * time.Second):
			v.loader.close()
			t.Fatal("parent icon did not load")
		}
		v.fileIconWanted = nil
		dims := v.withFileIcon(gtx, &v.panels[0], item, false, row)
		if len(v.fileIconWanted) != 0 || dims.Size != image.Pt(400, 24) {
			t.Fatal("parent icon bypassed the cache or changed row dimensions")
		}
		v.finishOperations()
	}
}

func TestFileIconCacheSeparatesFileAndFolderAtSamePath(t *testing.T) {
	v := newPreviewView()
	v.loader = newDirectoryLoader(browser.ReadLocation, nil)
	defer v.loader.close()
	v.loader.readFileIcon = func(string, int) image.Image {
		return image.NewRGBA(image.Rect(0, 0, 16, 16))
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 24))}
	row := func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }
	for _, directory := range []bool{false, true, false, true} {
		v.fileIconWanted = nil
		v.withFileIcon(gtx, &v.panels[0], entry{file: browser.Entry{Name: "reused", Directory: directory}}, false, row)
		if len(v.fileIcons) < 2 {
			if len(v.fileIconWanted) != 1 || v.fileIconWanted[0].directory != directory {
				t.Fatal("file and folder reused the same cached icon")
			}
			v.submitFileIcons()
			select {
			case result := <-v.loader.results:
				v.acceptFileIcons(result)
			case <-time.After(3 * time.Second):
				t.Fatal("icon load did not complete")
			}
		} else if len(v.fileIconWanted) != 0 {
			t.Fatal("file or folder icon was not cached")
		}
	}
}

func TestIconsSettingsDefaultToggleCancelAndSave(t *testing.T) {
	v := newPreviewView()
	if !v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool {
		t.Fatal("icons did not default to On")
	}
	found := false
	for _, command := range v.settingsCatalogue() {
		if command.label() == "Appearance › Icons" {
			command.execute(v)
			found = true
			break
		}
	}
	s := v.settingsInteraction()
	if !found || s == nil || len(s.fields) != 2 || s.fields[0].Label != "Enabled" || s.fields[0].Control != "toggle" || s.fields[1].Key != settingIconOriginalColor || s.fields[1].Control != "stepper-progress" {
		t.Fatal("Icons did not offer Enabled and Original color")
	}
	v.adjustSetting(settingIconOriginalColor, -1)
	if v.settingsValues.Get(settings.CatalogueDefault, settingIconOriginalColor).Number != 79 {
		t.Fatal("Original color did not default to 80 or preview live")
	}
	v.loader = newDirectoryLoader(browser.ReadLocation, nil)
	defer v.loader.close()
	v.loader.readFileIcon = func(string, int) image.Image { return nil }
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 24))}
	item := entry{file: browser.Entry{Name: "sample.exe"}}
	width := 0
	row := func(gtx layout.Context) layout.Dimensions {
		width = gtx.Constraints.Max.X
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	v.withFileIcon(gtx, &v.panels[0], item, false, row)
	iconWidth := width
	v.adjustSetting(settingIconsEnabled, 0)
	v.submitFileIcons() // An already collected request must also stay idle.
	v.fileIconWanted = nil
	v.withFileIcon(gtx, &v.panels[0], item, false, row)
	if width != 400 || iconWidth >= width || len(v.fileIconWanted) != 0 || v.fileIconPending {
		t.Fatal("Off retained icon spacing or requested icons")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.settingsValues.Get(settings.CatalogueDefault, settingIconOriginalColor).Number != 80 {
		t.Fatal("Cancel did not restore the default color amount")
	}
	v.withFileIcon(gtx, &v.panels[0], item, false, row)
	if width != iconWidth || len(v.fileIconWanted) != 1 {
		t.Fatal("Cancel did not restore icon display")
	}
	v.loader.close()
	v.loader = nil
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget("appearance.icons")
	v.adjustSetting(settingIconsEnabled, 0)
	v.previewSettingValue(settingIconOriginalColor, settings.Value{Number: 25})
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	if values.Get(settings.CatalogueDefault, settingIconsEnabled).Bool {
		t.Fatal("saved Off was not retained on load")
	}
	if values.Get(settings.CatalogueDefault, settingIconOriginalColor).Number != 25 {
		t.Fatal("saved icon color amount was not retained on load")
	}
}

func TestFileIconCacheKeepsRecentlyViewedIconsWhenFull(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{readFileIcon: func(string, int) image.Image { return nil }}
	v.fileIcons = make(map[fileIconKey]fileIconEntry)
	keys := make([]fileIconKey, fileIconCacheLimit)
	for i := range keys {
		keys[i] = fileIconKey{path: filepath.Join(v.panels[0].path, strconv.Itoa(i)), size: 16}
	}
	v.acceptFileIcons(loadResult{request: loadRequest{icons: keys}, icons: make([]image.Image, len(keys))})
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 24))}
	v.withFileIcon(gtx, &v.panels[0], entry{file: browser.Entry{Name: "0"}}, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	newKey := fileIconKey{path: "new", size: 16}
	v.acceptFileIcons(loadResult{request: loadRequest{icons: []fileIconKey{newKey}}, icons: []image.Image{nil}})
	_, recent := v.fileIcons[keys[0]]
	_, oldest := v.fileIcons[keys[1]]
	_, added := v.fileIcons[newKey]
	if !recent || oldest || !added || len(v.fileIcons) != fileIconCacheLimit {
		t.Fatal("cache overflow discarded recent icons instead of only the oldest")
	}
}

func TestNameEditorsAndSettingsPreviewsRequestIcons(t *testing.T) {
	for _, kind := range []nameOperation{nameCreateFile, nameCreateFolder, nameRename} {
		v := newPreviewView()
		v.panels[0].clearMarks()
		v.loader = &directoryLoader{readFileIcon: func(string, int) image.Image { return nil }}
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 300))}
		v.startName(gtx, kind)
		v.nameEditor.SetText("new.txt")
		v.panel(gtx, 0)
		found := false
		for _, key := range v.fileIconWanted {
			if kind == nameRename {
				found = found || !key.sample && key.path == filepath.Join(v.panels[0].path, v.nameInteraction().oldName)
			} else {
				found = found || key.sample && key.directory == (kind == nameCreateFolder)
			}
		}
		if !found {
			t.Fatalf("name operation %d omitted its icon", kind)
		}
	}
	for _, group := range []string{settingThemeSelect, settingThemeGenerator, settingThemeCustomize, "appearance.contrast_guard", "appearance.autocolor", "appearance.autocolor.selection", "appearance.autocolor.special_files"} {
		v := newPreviewView()
		v.loader = &directoryLoader{readFileIcon: func(string, int) image.Image { return nil }}
		v.startSettingsWidget(group)
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 500))}
		v.panel(gtx, 0)
		if len(v.fileIconWanted) == 0 {
			t.Fatalf("%s preview omitted icons", group)
		}
		for _, key := range v.fileIconWanted {
			if !key.sample {
				t.Fatalf("%s queried a real path for a sample", group)
			}
		}
		v.fileIconWanted = nil
		v.previewSettingValue(settingIconsEnabled, settings.Value{Bool: false})
		v.panel(gtx, 0)
		if len(v.fileIconWanted) != 0 {
			t.Fatalf("%s ignored disabled icons", group)
		}
	}
}

func TestSampleIconsShareAssociationsAndUseDirectoryWorker(t *testing.T) {
	v := newPreviewView()
	v.loader = newDirectoryLoader(browser.ReadLocation, nil)
	defer v.loader.close()
	v.loader.readFileTypeIcon = func(name string, size int, directory bool) image.Image {
		if name != "sample.txt" && !directory {
			t.Errorf("unexpected association lookup %q", name)
		}
		return image.NewRGBA(image.Rect(0, 0, size, size))
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 24))}
	width := 0
	content := func(gtx layout.Context) layout.Dimensions {
		width = gtx.Constraints.Max.X
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	for _, file := range []browser.Entry{{Name: "notes.txt"}, {Name: ".hidden.TXT", Hidden: true}, {Name: "documents", Directory: true}} {
		v.withRowIcon(gtx, &panel{}, entry{file: file}, false, true, content)
	}
	if width >= 400 || len(v.fileIconWanted) != 2 {
		t.Fatal("sample rows lost spacing or repeated association lookups")
	}
	v.submitFileIcons()
	select {
	case result := <-v.loader.results:
		v.acceptFileIcons(result)
		for _, icon := range v.fileIcons {
			if icon.source == nil {
				t.Fatal("worker did not supply association artwork")
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sample icons did not complete")
	}
}
