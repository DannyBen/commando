package app

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func TestReassignmentStagesAndSavesBothCommands(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", bindingSettingsCatalogue().Save(dir, values) }
	}
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	p, s := openShortcutWidget(t, v, send, "Next metadata column")
	original := v.settingsValues.Clone()
	send(pressed(key.NameReturn))
	send(modified("V", key.ModCtrl))
	question := v.confirmationInteraction()
	if question == nil || question.message != "Ctrl+V is assigned to Paste." || question.selected != 0 || !router.Source().Focused(question) || v.settingsWidget() != s {
		t.Fatalf("question=%+v focused=%v widget=%p expected=%p", question, router.Source().Focused(question), v.settingsWidget(), s)
	}
	send(pressed(key.NameReturn))
	if v.settingsInteraction() != s || !router.Source().Focused(&v.focus) || s.capturing || !s.dirty {
		t.Fatalf("accept current=%T focused=%v capturing=%v dirty=%v", v.interaction.current, router.Source().Focused(&v.focus), s.capturing, s.dirty)
	}
	draft := view{settingsValues: s.draft}
	paste := draft.resolveCommandBindings(commandByID(v, "entry.paste"))
	if paste.effectiveShortcut(0).String() != "Shift+Insert" || paste.effectiveShortcut(1).Name != "" || v.shortcutSettingText(s, s.fields[0].Key) != "Ctrl+V" {
		t.Fatal("reassignment lost or failed to promote the remaining shortcut")
	}
	if !reflect.DeepEqual(v.settingsValues.Clone(), original) {
		t.Fatal("accept changed active bindings before Save")
	}
	send(modified(key.NameReturn, key.ModCtrl))
	if v.paletteInteraction() != p {
		t.Fatal("Save did not restore palette")
	}
	loaded, err := settings.Load(dir, bindingSettingsCatalogue())
	if err != nil || findBindingConflict(loaded) != nil {
		t.Fatal("both commands were not saved consistently", err)
	}
	check := view{settingsValues: loaded}
	if check.resolveCommandBindings(*s.bindingCommand).effectiveShortcut(0).String() != "Ctrl+V" || check.resolveCommandBindings(commandByID(v, "entry.paste")).effectiveShortcut(0).String() != "Shift+Insert" {
		t.Fatal("saved reassignment did not survive reload")
	}
}

func commandByID(v *view, id string) command {
	for _, c := range bindingCommands(v.settingsValues) {
		if c.id == id {
			return c
		}
	}
	panic("missing command: " + id)
}

func TestReassignmentCancellationAndFailedSaveKeepAssignments(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	_, s := openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	send(modified("C", key.ModCtrl))
	send(pressed(key.NameEscape))
	if v.settingsInteraction() != s || s.capturing || s.dirty || s.draft.Overridden(s.fields[0].Key) {
		t.Fatalf("declining: current=%T capture=%v dirty=%v overridden=%v", v.interaction.current, s.capturing, s.dirty, s.draft.Overridden(s.fields[0].Key))
	}
	send(pressed(key.NameReturn))
	send(modified("C", key.ModCtrl))
	send(pressed(key.NameReturn))
	v.prepareSettingsSave = func(settings.Values) func() (string, error) {
		return func() (string, error) { return "", os.ErrPermission }
	}
	send(modified(key.NameReturn, key.ModCtrl))
	if v.settingsInteraction() != s || s.errorText != "Settings save: access denied" || v.resolveCommandBindings(commandByID(v, "entry.copy")).effectiveShortcut(0).String() != "Ctrl+C" {
		t.Fatalf("failed save current=%T error=%q copy=%s", v.interaction.current, s.errorText, v.resolveCommandBindings(commandByID(v, "entry.copy")).effectiveShortcut(0).String())
	}
	// Whole-widget Cancel discards both sides of the accepted reassignment.
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	if v.settingsValues.Overridden(s.fields[0].Key) || v.settingsValues.Overridden(bindingSettingKey("entry.copy", 0)) {
		t.Fatal("Cancel retained part of the reassignment")
	}
}

func TestConfirmationOwnsKeyboardAndPointer(t *testing.T) {
	v := newPreviewView()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	_, s := openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	send(modified("C", key.ModCtrl))
	question := v.confirmationInteraction()
	if question == nil {
		t.Fatal("missing question")
	}
	selected, active := s.selected, v.active
	positions := make([]layout.Position, len(v.panels))
	for i := range v.panels {
		positions[i] = v.panels[i].list.Position
	}
	send(modified("T", key.ModCtrl), modified("2", key.ModAlt), pressed(key.NameDownArrow))
	point := f32.Pt(20, 300)
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: point, Scroll: f32.Pt(0, 120)})
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	if v.confirmationInteraction() != question || s.selected != selected || v.active != active {
		t.Fatal("background shortcut or outside click escaped the decision")
	}
	for i := range v.panels {
		if v.panels[i].list.Position != positions[i] {
			t.Fatal("scroll reached a panel")
		}
	}
	send(pressed(key.NameTab))
	send(pressed(key.NameTab)) // A held key's repeat must not toggle back to Reassign.
	if question.selected != 1 {
		t.Fatalf("Tab repeat selected=%d focus=%v", question.selected, router.Source().Focused(question))
	}
	send(key.Event{Name: key.NameTab, State: key.Release})
	send(pressed(key.NameTab))
	if question.selected != 0 {
		t.Fatal("Tab after release did not move focus")
	}
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameReturn))
	if v.settingsInteraction() != s || s.dirty || !router.Source().Focused(&v.focus) {
		t.Fatal("Enter on Cancel failed to restore the unmodified widget")
	}
}

func TestConfirmationProtectsReservedKeys(t *testing.T) {
	for _, name := range []key.Name{key.NameUpArrow, key.NameEscape} {
		v := newPreviewView()
		send, _ := panelInputFrames(v)
		_, s := openShortcutWidget(t, v, send, "Next metadata column")
		candidate := s.draft.Clone()
		// Up is implicit row navigation; Escape is a protected command binding.
		if err := candidate.Set(bindingSettingsCatalogue(), s.fields[0].Key, settings.Value{Text: (shortcut{Name: name}).String()}); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := reassignBindings(candidate, s.bindingCommand.id, 0); ok {
			t.Fatal("reserved shortcut could be reassigned", name)
		}
	}
}

func TestConfirmationFitsShortWindowsAndTallDrawer(t *testing.T) {
	for _, size := range []image.Point{image.Pt(800, 480), image.Pt(320, 140), image.Pt(240, 70)} {
		v := newPreviewView()
		v.startSettingsWidget("appearance.font")
		drawer := v.settingsInteraction()
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
		q := &confirmationSession{message: strings.Repeat("A long command name ", 30), acceptLabel: "Reassign"}
		if !v.startConfirmation(gtx, q) {
			t.Fatal("could not suspend drawer")
		}
		v.layout(gtx)
		bounds := q.surface.bounds
		if !bounds.In(image.Rectangle{Max: size}) || bounds.Empty() || v.settingsWidget() != drawer {
			t.Fatal("alert does not fit independently of drawer", size, bounds)
		}
		if size.Y < 200 && q.messageList.Position.Length <= bounds.Dy() {
			t.Fatalf("long explanation: size=%v bounds=%v position=%+v", size, bounds, q.messageList.Position)
		}
	}
}

func TestConfirmationPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if dir == "" {
		t.Skip("optional headless confirmation preview")
	}
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	send(modified("T", key.ModCtrl))
	if err := renderViewPNG(filepath.Join(dir, "confirmation.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmationIgnoresInvocationRepeatsAndSupportsMouse(t *testing.T) {
	for _, accept := range []bool{true, false} {
		v := newPreviewView()
		v.startSettingsWidget("appearance.font")
		drawer := v.settingsInteraction()
		send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
		var ops op.Ops
		called := false
		q := &confirmationSession{message: "Continue?", acceptLabel: "Continue", held: map[key.Name]bool{key.NameReturn: true}, accept: func(layout.Context) { called = true }}
		if !v.startConfirmation(layout.Context{Ops: &ops}, q) {
			t.Fatal("could not open decision")
		}
		send()
		send(pressed(key.NameReturn), pressed(key.NameReturn))
		if v.confirmationInteraction() != q || called {
			t.Fatal("invocation repeats accepted the decision")
		}
		measure := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		footerHeight := v.textHeight(measure, v.appearance.SmallTextSize) + measure.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
		rowHeight := v.textHeight(measure, v.appearance.TextSize) + measure.Dp(2*max(0, v.appearance.RowPaddingVertical))
		x := q.surface.bounds.Min.X + q.surface.bounds.Dx()/4
		if !accept {
			x += q.surface.bounds.Dx() / 2
		}
		point := f32.Pt(float32(x), float32(q.surface.bounds.Max.Y-footerHeight-rowHeight/2-1))
		send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
		if called != accept || v.settingsInteraction() != drawer || !router.Source().Focused(&v.focus) {
			t.Fatal("button failed to resolve and return ownership", accept, called)
		}
	}
}
