package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestParentEntryNavigation(t *testing.T) {
	for _, show := range []bool{false, true} {
		name := "default hidden"
		if show {
			name = "opted in"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "child")
			if err := os.Mkdir(child, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(child, "first.txt"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			v := newLocationView([]panelState{{Path: root}}, 0, nil)
			defer v.loader.close()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingParentEntry, settings.Value{Bool: show})
			v.applySettings()
			waitLoaded(t, v, 0)
			p := &v.panels[0]
			if p.items[p.selected].name != "child" {
				t.Fatal("initial navigation did not select first real item")
			}
			send, _ := panelInputFrames(v)
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			send()
			if p.path != child || p.items[p.selected].name != "first.txt" || p.items[0].file.Parent != show {
				t.Fatalf("folder entry selection or parent visibility incorrect: %+v", p.items)
			}
			send(pressed(key.NameDeleteBackward))
			waitLoaded(t, v, 0)
			send()
			if p.path != root || p.items[p.selected].name != "child" {
				t.Fatal("going up did not select the folder just left")
			}
			if err := os.Remove(filepath.Join(child, "first.txt")); err != nil {
				t.Fatal(err)
			}
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			send()
			if show {
				if p.selected != 0 || len(p.items) != 1 || !p.items[0].file.Parent {
					t.Fatal("empty folder did not select its optional parent row")
				}
			} else if p.selected != -1 || len(p.items) != 0 {
				t.Fatal("empty folder retained a hidden parent or selection")
			}
		})
	}
}

func TestParentEntrySettingProjectsCacheAndPersists(t *testing.T) {
	v, reads := visibilityView(t)
	// Start from application defaults, retaining a real item's marks and viewport.
	v.applySettings()
	p := &v.panels[0]
	selectName(t, p, "visible.txt")
	p.toggleMark(p.selected)
	p.list.Position.First, p.list.Position.Offset = p.selected, 3
	p.setFilter("visible")
	v.closedPanels = []closedPanel{{panel: p.clone()}}
	beforeReads := reads.Load()
	check := func(show bool) {
		t.Helper()
		for _, current := range []*panel{p, &v.panels[1], &v.closedPanels[0].panel} {
			if current.items[0].file.Parent != show || !slices.ContainsFunc(current.entries, func(file browser.Entry) bool { return file.Parent }) {
				t.Fatal("setting did not project the cached parent in live and closed panels")
			}
		}
		if p.items[p.selected].name != "visible.txt" || !p.marks[p.selected] || len(p.marks) != 1 ||
			p.items[p.list.Position.First].name != "visible.txt" || p.list.Position.Offset != 3 || p.filter != "visible" || reads.Load() != beforeReads {
			t.Fatal("parent setting disturbed browsing state or reread folders")
		}
	}
	check(false)
	v.startSettingsWidget(settingParentEntry)
	v.adjustSetting(settingParentEntry, 0)
	check(true)
	v.finishSettingsWidget(layout.Context{}, false)
	check(false)
	v.startSettingsWidget(settingParentEntry)
	v.adjustSetting(settingParentEntry, 0)
	v.finishSettingsWidget(layout.Context{}, true)
	check(true)
	dir := t.TempDir()
	if err := settings.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !values.Get(settings.CatalogueDefault, settingParentEntry).Bool {
		t.Fatal("parent entry preference did not persist", err)
	}
	// Hiding the selected parent chooses a real item; showing it retains that item.
	p.setFilter("")
	p.selected = 0
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingParentEntry, settings.Value{Bool: false})
	v.applySettings()
	if p.selected < 0 || p.items[p.selected].file.Parent {
		t.Fatal("hiding selected parent did not select a surviving item")
	}
}
