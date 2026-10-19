package app

import (
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/feedback"
)

func TestArchivePaletteMasksPreserveOriginalShortcutDialogName(t *testing.T) {
	v, _ := archiveView(t)
	selectName(t, &v.panels[0], "a2.txt")
	setTestShortcut(t, v, "entry.recycle", 0, "Ctrl+F9")
	setTestShortcut(t, v, "entry.recycle", 1, "Ctrl+F10")
	send, _ := panelInputFrames(v)
	palette, widget := openShortcutWidget(t, v, send, "Delete from archive")
	if len(palette.results) != 1 || palette.results[0].command.id != "entry.recycle" || palette.results[0].command.shortcut() != "Ctrl+F9 / Ctrl+F10" {
		t.Fatal("palette did not retain Recycle identity and bindings")
	}
	if widget.title != "Entry › Recycle › Shortcuts" || widget.bindingCommand.name != "Recycle" || widget.bindingCommand.id != "entry.recycle" {
		t.Fatal("shortcut dialog presented mask as a separate command", widget.title)
	}
	// Editing the masked entry saves the original global binding key.
	send(pressed(key.NameReturn), key.Event{Name: key.NameReturn, State: key.Release})
	send(modified("F11", key.ModCtrl), key.Event{Name: "F11", Modifiers: key.ModCtrl, State: key.Release})
	send(modified(key.NameReturn, key.ModCtrl))
	// The real worker saves before the palette is restored.
	if v.settingsInteraction() != nil && v.settingsInteraction().pending {
		v.acceptLoad(<-v.loader.results)
	}
	if v.paletteInteraction() == nil {
		t.Fatal("shortcut save did not return to palette")
	}
	send(pressed(key.NameEscape))
	v.parentFolder()
	waitLoaded(t, v, 0)
	c := v.resolveCommandBindings(commandByID(v, "entry.recycle"))
	if c.label() != "Entry › Recycle" || c.effectiveShortcut(0).Name != "F11" || c.effectiveShortcut(0).Modifiers != key.ModCtrl {
		t.Fatal("masked edit did not change the original command globally", c.shortcut())
	}
	send(pressed("`"))
	visible := map[string]string{}
	for _, result := range v.paletteInteraction().results {
		visible[result.command.id] = result.command.label()
	}
	if visible["entry.recycle"] != "Entry › Recycle" || visible["entry.delete_permanently"] != "Entry › Delete permanently" {
		t.Fatal("normal folder retained archive presentation")
	}

}

func TestArchivePaletteHidesPermanentDeleteWithoutDisablingItsBindings(t *testing.T) {
	for _, binding := range []struct {
		name key.Name
		slot int
		id   string
	}{
		{"F9", 0, "entry.recycle"}, {"F10", 1, "entry.recycle"},
		{"F7", 0, "entry.delete_permanently"}, {"F8", 1, "entry.delete_permanently"},
	} {
		v, _ := archiveView(t)
		selectName(t, &v.panels[0], "a2.txt")
		setTestShortcut(t, v, "entry.recycle", 0, "Ctrl+F9")
		setTestShortcut(t, v, "entry.recycle", 1, "Ctrl+F10")
		setTestShortcut(t, v, "entry.delete_permanently", 0, "Ctrl+F7")
		setTestShortcut(t, v, "entry.delete_permanently", 1, "Ctrl+F8")
		send, _ := panelInputFrames(v)
		send(pressed("`"), key.EditEvent{Text: "delete"})
		p := v.paletteInteraction()
		if len(p.results) != 1 || p.results[0].command.id != "entry.recycle" {
			t.Fatal("palette retained redundant deletion action", p.results)
		}
		p.filter("")
		for _, result := range p.results {
			if result.command.id == "entry.delete_permanently" {
				t.Fatal("hidden command appeared in blank palette")
			}
		}
		if !v.resolveCommandBindings(commandByID(v, "entry.delete_permanently")).matches(modified(binding.name, key.ModCtrl)) && binding.id == "entry.delete_permanently" {
			t.Fatal("hidden command lost configured binding")
		}
		send(pressed(key.NameEscape))
		send(pressed(key.NameDeleteForward), key.Event{Name: key.NameDeleteForward, State: key.Release}, modified(key.NameDeleteForward, key.ModShift))
		if !v.interaction.idle() {
			t.Fatal("reassigned default deletion keys remained active")
		}
		send(modified(binding.name, key.ModCtrl))
		q := v.confirmationInteraction()
		if q == nil || q.selected != 1 || q.acceptLabel != "Delete from archive" {
			t.Fatal("configured binding did not use permanent-delete guards", binding)
		}
		send(pressed(key.NameLeftArrow), pressed(key.NameReturn))
		waitLoaded(t, v, 0)
		if message := v.panels[0].feedback.Current(); message == nil || message.Kind != feedback.Success {
			t.Fatal("configured archive deletion did not complete", binding, message)
		}
	}
}
