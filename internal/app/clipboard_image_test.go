package app

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func testImagePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{20, 40, 60, 128})
	img.SetNRGBA(1, 0, color.NRGBA{200, 100, 50, 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestImagePasteNamesCapturedPNGAndReconcilesPanels(t *testing.T) {
	v, root, _ := clipboardView(t)
	captured := testImagePNG(t)
	current := captured
	v.loader.readClipboard = func() (browser.FileClipboard, error) { return browser.FileClipboard{ImagePNG: current}, nil }
	selectName(t, &v.panels[0], "a.txt")
	v.panels[0].toggleMark(v.panels[0].selected)
	selectName(t, &v.panels[2], "z.txt")
	v.panels[2].toggleMark(v.panels[2].selected)
	router, send := clipboardFrames(v)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	if n := v.nameInteraction(); n == nil || n.kind != nameCreateFile || v.nameEditor.Text() != "file.png" || v.nameEditor.SelectedText() != "file" || !router.Source().Focused(&v.nameEditor) {
		t.Fatal("image did not open shared file draft")
	}
	if _, err := os.Stat(filepath.Join(root, "file.png")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("image saved before confirmation")
	}
	current = []byte("clipboard replaced after capture")
	send(pressed(key.NameTab), modified(key.NameTab, key.ModShift))
	if !router.Source().Focused(&v.nameEditor) {
		t.Fatal("Tab left name editor")
	}
	send(replaceCreate(v, "capture.png"), pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	data, err := os.ReadFile(filepath.Join(root, "capture.png"))
	if err != nil || !bytes.Equal(data, captured) {
		t.Fatal("saved new clipboard instead of captured image", err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil || color.NRGBAModel.Convert(decoded.At(0, 0)) != (color.NRGBA{20, 40, 60, 128}) {
		t.Fatal("PNG pixels changed")
	}
	for pi := range []int{0, 1, 2} {
		if pi == 1 {
			continue
		}
		p := &v.panels[pi]
		wantMarks := 0
		if pi == 2 {
			wantMarks = 1
		}
		if len(p.marks) != wantMarks {
			t.Fatal("creation left active marks or changed another panel's marks")
		}
		want := "capture.png"
		if pi == 2 {
			want = "z.txt"
		}
		if p.items[p.selected].name != want {
			t.Fatal("wrong selection after creation")
		}
		if !slices.ContainsFunc(p.items, func(e entry) bool { return e.name == "capture.png" && e.file.Bytes == int64(len(captured)) }) {
			t.Fatal("alias listing or metadata stale")
		}
	}
	if v.nameInteraction() != nil || v.transferPending || v.mutationsBusy() || !bytes.Equal(current, []byte("clipboard replaced after capture")) {
		t.Fatal("completion stayed busy or changed clipboard")
	}
}

func TestImagePasteCancelAndCollisionRetry(t *testing.T) {
	for _, retry := range []bool{false, true} {
		v, root, _ := clipboardView(t)
		captured := testImagePNG(t)
		v.loader.readClipboard = func() (browser.FileClipboard, error) { return browser.FileClipboard{ImagePNG: captured}, nil }
		send, _ := panelInputFrames(v)
		send(modified("V", key.ModCtrl))
		waitLoaded(t, v, 0)
		send()
		if retry {
			if err := os.WriteFile(filepath.Join(root, "file.png"), []byte("existing"), 0600); err != nil {
				t.Fatal(err)
			}
			send(pressed(key.NameReturn))
			waitCreated(t, v)
			send()
			if n := v.nameInteraction(); n == nil || n.pending || !errors.Is(n.err, os.ErrExist) || v.nameEditor.Text() != "file.png" {
				t.Fatal("collision discarded draft")
			}
			data, _ := os.ReadFile(filepath.Join(root, "file.png"))
			if string(data) != "existing" {
				t.Fatal("collision replaced file")
			}
			send(replaceCreate(v, "retry.png"), pressed(key.NameReturn))
			waitCreated(t, v)
			send()
			data, err := os.ReadFile(filepath.Join(root, "retry.png"))
			if err != nil || !bytes.Equal(data, captured) {
				t.Fatal("retry lost captured contents")
			}
		} else {
			before := slices.Clone(v.panels[0].items)
			send(pressed(key.NameEscape))
			if v.nameInteraction() != nil || !slices.Equal(before, v.panels[0].items) || v.transferPending {
				t.Fatal("cancel changed listing or retained draft")
			}
			if _, err := os.Stat(filepath.Join(root, "file.png")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("cancel created file")
			}
		}
	}
}

func TestImagePasteArrivalDefersBehindEditorAndCancelsAfterLeavingPane(t *testing.T) {
	for _, leave := range []bool{false, true} {
		v := newPreviewView()
		v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
		send, _ := panelInputFrames(v)
		send(modified("V", key.ModCtrl))
		request := <-v.loader.requests
		if leave {
			send(pressed(key.NameRightArrow))
		}
		send(modified("D", key.ModAlt))
		v.location.SetText("draft path")
		v.location.SetCaret(1, 4)
		v.acceptLoad(loadResult{request: request, clipboard: browser.FileClipboard{ImagePNG: testImagePNG(t)}})
		send()
		start, end := v.location.Selection()
		if !v.locationEditing() || v.location.Text() != "draft path" || start != 1 || end != 4 {
			t.Fatal("image arrival disturbed editor")
		}
		send(pressed(key.NameEscape))
		send()
		if leave {
			if v.nameInteraction() != nil || v.pendingNameDraft != nil || v.transferPending {
				t.Fatal("image followed focus to other panel")
			}
		} else if v.nameInteraction() == nil || v.nameEditor.Text() != "file.png" {
			t.Fatal("image draft did not follow editor dismissal")
		}
	}
}

func TestFilePasteTakesPriorityOverImageRepresentation(t *testing.T) {
	for _, sameFolder := range []bool{false, true} {
		v, root, target := clipboardView(t)
		captured := testImagePNG(t)
		v.loader.readClipboard = func() (browser.FileClipboard, error) {
			return browser.FileClipboard{Paths: []string{filepath.Join(root, "a.txt")}, ImagePNG: captured}, nil
		}
		if !sameFolder {
			v.active = 1
		}
		send, _ := panelInputFrames(v)
		send(modified("V", key.ModCtrl))
		waitLoaded(t, v, v.active)
		send()
		if sameFolder {
			if v.nameInteraction() == nil || v.nameInteraction().kind != nameDuplicate || v.nameEditor.Text() != "a - Copy.txt" {
				t.Fatal("image overrode same-folder file duplication")
			}
		} else {
			data, err := os.ReadFile(filepath.Join(target, "a.txt"))
			if err != nil || string(data) != "alpha" || v.nameInteraction() != nil {
				t.Fatal("image overrode file paste")
			}
		}
	}
}
