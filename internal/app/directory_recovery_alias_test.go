package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestTransferRefreshPreservesExistingAliasAndMarkedFiles(t *testing.T) {
	v, source, _ := clipboardView(t)
	attachTestFileClipboard(v)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Skipf("directory aliases unavailable: %v", err)
	}
	v.load(2, alias, "")
	waitLoaded(t, v, 2)
	p := &v.panels[2]
	selectName(t, p, "a.txt")
	p.toggleMark(p.selected)
	selectName(t, &v.panels[0], "a.txt")
	send, _ := panelInputFrames(v)
	send(modified("C", key.ModCtrl))
	waitLoaded(t, v, 0)
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	if p.path != alias || p.items[p.selected].file.Name != "a.txt" || !p.marks[p.selected] {
		t.Fatal("ordinary transfer refresh replaced the alias or lost its file identity")
	}
}

func TestRecyclingRecoversNestedAndAliasPanelsWithoutNotifications(t *testing.T) {
	v, source, _ := clipboardView(t)
	folder := filepath.Join(source, "folder")
	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "alias")
	if err := os.Symlink(folder, alias); err != nil {
		t.Skipf("directory aliases unavailable: %v", err)
	}
	v.load(1, filepath.Join(folder, "nested"), "")
	v.load(2, filepath.Join(alias, "nested"), "")
	waitLoaded(t, v, 1)
	waitLoaded(t, v, 2)
	for pi := 1; pi < 3; pi++ {
		selectName(t, &v.panels[pi], "file")
		v.panels[pi].toggleMark(v.panels[pi].selected)
	}
	moved := filepath.Join(t.TempDir(), "recycled")
	v.loader.recycle = func(directory string, names []string) (browser.RecycleResult, error) {
		if err := os.Rename(folder, moved); err != nil {
			return browser.RecycleResult{}, err
		}
		return browser.RecycleResult{Directory: directory, Names: names}, nil
	}
	selectName(t, &v.panels[0], "folder")
	v.recycleSelected()
	waitLoaded(t, v, 0)
	message := v.panels[0].feedback.Current()
	runAutomaticRefresh(v, time.Now())
	finishAutomaticRefresh(t, v)
	if v.panels[1].path != source || v.panels[2].path != aliasParent ||
		len(v.panels[1].marks) != 0 || len(v.panels[2].marks) != 0 || v.active != 0 || v.panels[0].feedback.Current() != message {
		t.Fatal("confirmed recycling stranded open descendants or their aliases")
	}
}
