package browser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/color"
	"image/png"
	"unsafe"

	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Opt in on a Windows desktop. This test replaces the system clipboard.
func TestWindowsFileClipboardRoundTrip(t *testing.T) {
	if os.Getenv("COMMANDO_TEST_CLIPBOARD") != "1" {
		t.Skip("requires COMMANDO_TEST_CLIPBOARD=1 on a Windows desktop; replaces clipboard")
	}
	paths := []string{filepath.Join(t.TempDir(), "Å file.txt"), filepath.Join(t.TempDir(), "📁 folder")}
	for _, move := range []bool{false, true} {
		snapshot, err := WriteFileClipboard(paths, move)
		if err != nil {
			t.Fatal(err)
		}
		read, err := ReadFileClipboard()
		if err != nil || read.Move != move || !reflect.DeepEqual(read.Paths, paths) || read.Sequence != snapshot.Sequence {
			t.Fatalf("native round trip: %+v %v", read, err)
		}
	}
	cut, _ := ReadFileClipboard()
	partial, err := CompleteFileClipboard(cut, paths[:1])
	if err != nil || !reflect.DeepEqual(partial.Paths, paths[1:]) {
		t.Fatalf("native partial completion: %+v %v", partial, err)
	}
	read, err := ReadFileClipboard()
	if err != nil || !reflect.DeepEqual(read, partial) {
		t.Fatalf("native partial payload: %+v %v", read, err)
	}
	newer, err := WriteFileClipboard(paths, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteFileClipboard(partial, paths[1:]); err != nil {
		t.Fatal(err)
	}
	read, err = ReadFileClipboard()
	if err != nil || !reflect.DeepEqual(read, newer) {
		t.Fatal("native completion replaced newer clipboard")
	}
	cut, err = WriteFileClipboard(paths, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteFileClipboard(cut, paths); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileClipboard(); !errors.Is(err, ErrNoClipboardFiles) {
		t.Fatal("native completed cut retained file payload")
	}
}

// Opt in on a Windows desktop: the clipboard contents are replaced.
func TestWindowsImageClipboardRoundTrip(t *testing.T) {
	if os.Getenv("COMMANDO_TEST_CLIPBOARD") != "1" {
		t.Skip("requires COMMANDO_TEST_CLIPBOARD=1 on a Windows desktop; replaces clipboard")
	}
	dib := imageDIB(1, -1, 32, 124, []byte{30, 20, 10, 128})
	binary.LittleEndian.PutUint32(dib[16:], 3)
	binary.LittleEndian.PutUint32(dib[56:], 0x73524742) // LCS_sRGB
	for i, mask := range []uint32{0xff0000, 0xff00, 0xff, 0xff000000} {
		binary.LittleEndian.PutUint32(dib[40+i*4:], mask)
	}
	encoded, err := clipboardPNG(dib, true)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString("PNG")
	format, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	for _, payload := range []struct {
		format uintptr
		data   []byte
	}{{17, dib}, {format, encoded}} {
		err := withFileClipboard(true, func() error {
			handle, err := clipboardMemory(payload.data)
			if err != nil {
				return err
			}
			if ok, _, err := emptyClipboard.Call(); ok == 0 {
				globalFree.Call(handle)
				return err
			}
			if ok, _, err := setClipboardData.Call(payload.format, handle); ok == 0 {
				globalFree.Call(handle)
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sequence := FileClipboardSequence()
		snapshot, err := ReadFileClipboard()
		if err != nil || len(snapshot.Paths) != 0 || snapshot.Move || snapshot.Sequence != sequence {
			t.Fatalf("image snapshot: %+v %v", snapshot, err)
		}
		img, err := png.Decode(bytes.NewReader(snapshot.ImagePNG))
		if err != nil {
			t.Fatal(err)
		}
		if got := color.NRGBAModel.Convert(img.At(0, 0)); got != (color.NRGBA{10, 20, 30, 128}) {
			t.Fatal("native image pixel changed", got)
		}
		if FileClipboardSequence() != sequence {
			t.Fatal("read changed clipboard")
		}
	}
}
