package app

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/io/key"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func setGuard(t *testing.T, v *view, permanent bool, name, value string) {
	t.Helper()
	group := "guards.recycle_confirmation."
	if permanent {
		group = "guards.delete_confirmation."
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, group+name, settings.Value{Text: value}); err != nil {
		t.Fatal(err)
	}
}

func TestRemovalGuardsChooseStrongestMatchingRule(t *testing.T) {
	labels := []string{"OFF", "ON", "ON (Cancel default)"}
	for single := guardOff; single <= guardCancel; single++ {
		for directories := guardOff; directories <= guardCancel; directories++ {
			for multiple := guardOff; multiple <= guardCancel; multiple++ {
				v := newPreviewView()
				for _, permanent := range []bool{false, true} {
					setGuard(t, v, permanent, "single_files", labels[single])
					setGuard(t, v, permanent, "directories", labels[directories])
					setGuard(t, v, permanent, "multiple_entries", labels[multiple])
					for _, selection := range []struct {
						count int
						dirs  bool
						want  int
					}{{1, false, single}, {1, true, directories}, {3, false, multiple}, {3, true, max(directories, multiple)}} {
						if got := v.removalGuard(permanent, selection.count, selection.dirs); got != selection.want {
							t.Fatalf("single=%d dirs=%d multi=%d selection=%+v got=%d", single, directories, multiple, selection, got)
						}
					}
				}
			}
		}
	}
}

func TestPermanentDeleteDefaultsCancelAndDispatchesOnlyAfterApproval(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3)}
	p := &v.panels[0]
	before, marks := len(p.items), len(p.marks)
	send, _ := panelInputFrames(v)
	send(modified(key.NameDeleteForward, key.ModShift))
	q := v.confirmationInteraction()
	if q == nil || q.selected != 1 || q.acceptLabel != "Delete permanently" || len(v.loader.requests) != 0 || p.loading {
		t.Fatal("Shift+Delete did not wait with Cancel selected")
	}
	send(pressed(key.NameReturn))
	if !v.interaction.idle() || len(v.loader.requests) != 0 || len(p.items) != before || len(p.marks) != marks {
		t.Fatal("Enter on Cancel mutated the selection")
	}
	send(modified(key.NameDeleteForward, key.ModShift))
	send(pressed(key.NameLeftArrow))
	send(pressed(key.NameReturn))
	if len(v.loader.requests) != 1 {
		t.Fatal("approved deletion was not submitted")
	}
	request := <-v.loader.requests
	if request.removal == nil || !request.removal.permanent || !p.loading || !v.mutationsBusy() {
		t.Fatal("deletion bypassed the shared mutation worker")
	}
	v.acceptLoad(loadResult{request: request, removed: browser.RemovalResult{Names: request.removal.names[:1]}, err: os.ErrPermission})
	if len(p.items) != before-1 || p.feedback.Current().Kind != feedback.Error || p.feedback.Current().Text != "1 deleted; Access denied" {
		t.Fatal("partial deletion did not reconcile confirmed items", p.feedback.Current())
	}
}

func TestRecycleGuardCapturesSelectionAndCanDefaultToAction(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3)}
	setGuard(t, v, false, "multiple_entries", "ON")
	send, _ := panelInputFrames(v)
	p := &v.panels[0]
	var names []string
	for row, item := range p.items {
		if p.marks[row] {
			names = append(names, item.file.Name)
		}
	}
	send(pressed(key.NameDeleteForward))
	q := v.confirmationInteraction()
	if q == nil || q.selected != 0 || len(v.loader.requests) != 0 {
		t.Fatal("recycle guard did not focus its action")
	}
	send(pressed(key.NameEscape))
	if len(v.loader.requests) != 0 {
		t.Fatal("Escape dispatched recycling")
	}
	send(pressed(key.NameDeleteForward))
	send(pressed(key.NameReturn))
	request := <-v.loader.requests
	if request.removal.permanent || !reflect.DeepEqual(request.removal.names, names) {
		t.Fatal("recycle confirmation retargeted its captured selection")
	}
}

func TestPermanentDeleteCanDisableGuardsAndRejectsEditorsAndBusy(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3)}
	for _, name := range []string{"single_files", "directories", "multiple_entries"} {
		setGuard(t, v, true, name, "OFF")
	}
	send, _ := panelInputFrames(v)
	send(modified(key.NameDeleteForward, key.ModShift))
	if len(v.loader.requests) != 1 || v.confirmationInteraction() != nil {
		t.Fatal("OFF guards did not permit direct dispatch")
	}
	send(modified(key.NameDeleteForward, key.ModShift))
	if len(v.loader.requests) != 1 {
		t.Fatal("busy deletion dispatched again")
	}
	request := <-v.loader.requests
	v.acceptLoad(loadResult{request: request, err: browser.ErrDeleteUnavailable})
	send(modified("D", key.ModAlt))
	send(modified(key.NameDeleteForward, key.ModShift))
	if !v.locationEditing() || len(v.loader.requests) != 0 {
		t.Fatal("Shift+Delete escaped an editor")
	}
}

func TestDeleteWorkerUsesOwnBackendAndSharedAliasReconciliation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root, root}, nil)
	defer v.finishOperations()
	waitLoaded(t, v, 0)
	waitLoaded(t, v, 1)
	for _, name := range []string{"single_files", "directories", "multiple_entries"} {
		setGuard(t, v, true, name, "OFF")
	}
	v.loader.recycle = func(string, []string) (browser.RemovalResult, error) {
		t.Error("permanent delete used recycle backend")
		return browser.RemovalResult{}, nil
	}
	v.loader.delete = func(path string, names []string) (browser.RemovalResult, error) {
		return browser.RemovalResult{Directory: path, Names: names[:1]}, os.ErrPermission
	}
	p := &v.panels[0]
	selectName(t, p, "a")
	p.toggleMark(p.selected)
	selectName(t, p, "b")
	p.toggleMark(p.selected)
	selectName(t, p, "c")
	selectName(t, &v.panels[1], "c")
	send, _ := panelInputFrames(v)
	send(modified(key.NameDeleteForward, key.ModShift))
	waitLoaded(t, v, 0)
	send()
	for i := range v.panels {
		if len(v.panels[i].items) != 3 || v.panels[i].items[v.panels[i].selected].file.Name != "c" {
			t.Fatal("deletion lost aliases or surviving selection")
		}
	}
	if len(p.marks) != 1 {
		t.Fatal("deletion lost surviving marks")
	}
}

func TestGuardsWidgetsSaveCancelAndPersist(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.CatalogueDefault.Save(dir, values) }
	}
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@recycle confirmation"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.title != "Guards › Recycle confirmation" || len(s.fields) != 3 {
		t.Fatal("guard widget missing from settings")
	}
	send(pressed(key.NameRightArrow)) // Single files: OFF -> ON.
	send(modified(key.NameReturn, key.ModCtrl))
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || loaded.Get(settings.CatalogueDefault, "guards.recycle_confirmation.single_files").Text != "ON" {
		t.Fatal("guard did not persist", err)
	}
	v.startSettingsWidget("guards.delete_confirmation")
	send(pressed(key.NameLeftArrow))
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	if v.removalGuard(true, 1, false) != guardCancel {
		t.Fatal("Cancel failed to restore delete guard")
	}
}

func TestPaletteDeleteInvocationCannotAcceptItsOwnConfirmation(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 2)}
	for _, name := range []string{"single_files", "directories", "multiple_entries"} {
		setGuard(t, v, true, name, "ON")
	}
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "Entry Delete permanently"})
	send(pressed(key.NameReturn))
	q := v.confirmationInteraction()
	if q == nil {
		t.Fatal("palette did not open confirmation")
	}
	send(pressed(key.NameReturn))
	if v.confirmationInteraction() != q || len(v.loader.requests) != 0 {
		t.Fatal("held palette Enter accepted deletion")
	}
	send(key.Event{Name: key.NameReturn, State: key.Release})
	send(pressed(key.NameReturn))
	if len(v.loader.requests) != 1 {
		t.Fatal("fresh Enter did not accept deletion")
	}
}

func TestRemovalGuardPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if dir == "" {
		t.Skip("optional guard previews")
	}
	v := newPreviewView()
	v.startSettingsWidget("guards.delete_confirmation")
	if err := renderViewPNG(filepath.Join(dir, "delete-guards.png"), v); err != nil {
		t.Fatal(err)
	}
	send, _ := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed(key.NameEscape))
	v.loader = &directoryLoader{requests: make(chan loadRequest, 2)}
	send(modified(key.NameDeleteForward, key.ModShift))
	if err := renderViewPNG(filepath.Join(dir, "delete-confirmation.png"), v); err != nil {
		t.Fatal(err)
	}
}
