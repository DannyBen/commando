package browser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func imageDIB(width, height int32, depth uint16, header int, pixels []byte) []byte {
	data := make([]byte, header+len(pixels))
	binary.LittleEndian.PutUint32(data, uint32(header))
	binary.LittleEndian.PutUint32(data[4:], uint32(width))
	binary.LittleEndian.PutUint32(data[8:], uint32(height))
	binary.LittleEndian.PutUint16(data[12:], 1)
	binary.LittleEndian.PutUint16(data[14:], depth)
	copy(data[header:], pixels)
	return data
}

func TestClipboardDIBOrientationPaddingAndOpaqueRGB(t *testing.T) {
	for _, depth := range []uint16{24, 32} {
		for _, topDown := range []bool{false, true} {
			height := int32(2)
			pixels := []byte{0, 0, 255, 0, 255, 0, 0, 0} // red then blue, padded or unused high byte
			if topDown {
				height = -height
			} else {
				pixels = []byte{255, 0, 0, 0, 0, 0, 255, 0}
			}
			encoded, err := clipboardPNG(imageDIB(1, height, depth, 40, pixels), true)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			for y, want := range []color.NRGBA{{255, 0, 0, 255}, {0, 0, 255, 255}} {
				if got := color.NRGBAModel.Convert(decoded.At(0, y)); got != want {
					t.Fatalf("depth=%d topDown=%v row=%d: %v", depth, topDown, y, got)
				}
			}
		}
	}
}

func TestClipboardDIBExplicitAlphaAndBitfields(t *testing.T) {
	data := imageDIB(1, -1, 32, 124, []byte{30, 20, 10, 128})
	binary.LittleEndian.PutUint32(data[16:], 3)
	for i, mask := range []uint32{0xff0000, 0xff00, 0xff, 0xff000000} {
		binary.LittleEndian.PutUint32(data[40+i*4:], mask)
	}
	img, err := decodeClipboardDIB(data)
	if err != nil || img.NRGBAAt(0, 0) != (color.NRGBA{10, 20, 30, 128}) {
		t.Fatalf("alpha: %v %v", img, err)
	}
	data = imageDIB(1, 1, 16, 40, []byte{0, 248, 0, 0})
	masks := make([]byte, 12)
	for i, mask := range []uint32{0xf800, 0x7e0, 0x1f} {
		binary.LittleEndian.PutUint32(masks[i*4:], mask)
	}
	data = append(append(data[:40:40], masks...), data[40:]...)
	binary.LittleEndian.PutUint32(data[16:], 3)
	img, err = decodeClipboardDIB(data)
	if err != nil || img.NRGBAAt(0, 0) != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatalf("565: %v %v", img, err)
	}
}

func TestClipboardDIBPalette(t *testing.T) {
	for _, depth := range []uint16{1, 4, 8} {
		pixels := []byte{0, 0, 0, 0}
		pixels[0] = 1 << (8 - depth)
		data := imageDIB(1, 1, depth, 40, append([]byte{0, 0, 0, 0, 30, 20, 10, 0}, pixels...))
		binary.LittleEndian.PutUint32(data[32:], 2)
		img, err := decodeClipboardDIB(data)
		if err != nil || img.NRGBAAt(0, 0) != (color.NRGBA{10, 20, 30, 255}) {
			t.Fatalf("palette depth %d: %v %v", depth, img, err)
		}
	}
}

func TestClipboardImageRejectsMalformedAndOversizedData(t *testing.T) {
	valid := imageDIB(1, 1, 32, 40, []byte{0, 0, 0, 0})
	cases := [][]byte{nil, valid[:39], valid[:43], imageDIB(1<<30, 1, 32, 40, nil), imageDIB(1, -1<<31, 32, 40, nil), imageDIB(0, 1, 32, 40, nil)}
	for _, field := range []struct {
		offset int
		value  uint32
	}{{0, 41}, {12, 0}, {16, 1}, {32, 1 << 30}} {
		data := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(data[field.offset:], field.value)
		cases = append(cases, data)
	}
	badMasks := imageDIB(1, 1, 32, 56, []byte{0, 0, 0, 0})
	binary.LittleEndian.PutUint32(badMasks[16:], 3)
	binary.LittleEndian.PutUint32(badMasks[40:], 0xff)
	binary.LittleEndian.PutUint32(badMasks[44:], 0xff)
	cases = append(cases, badMasks)
	for i, data := range cases {
		if _, err := clipboardPNG(data, true); !errors.Is(err, ErrClipboardImage) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
	if _, err := clipboardPNG([]byte("not PNG"), false); !errors.Is(err, ErrClipboardImage) {
		t.Fatal("invalid PNG accepted")
	}
}

func TestClipboardPNGPreservesTransparencyAndBytes(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{10, 20, 30, 128})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	got, err := clipboardPNG(data, false)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("PNG changed", err)
	}
	if _, err := clipboardPNG(data[:len(data)-10], false); !errors.Is(err, ErrClipboardImage) {
		t.Fatal("truncated PNG accepted")
	}
}

func TestCreateFileDataPreservesDestinations(t *testing.T) {
	root := t.TempDir()
	data := []byte("captured contents")
	entry, err := CreateFileData(root, "image.png", data)
	if err != nil || entry.Bytes != int64(len(data)) || !entry.SizeKnown {
		t.Fatalf("created metadata: %+v %v", entry, err)
	}
	if _, err := CreateFileData(root, "image.png", []byte("replacement")); !errors.Is(err, os.ErrExist) {
		t.Fatal("collision accepted", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "image.png"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("existing destination changed")
	}
	if _, err := CreateFileData(root, "../escaped.png", data); !errors.Is(err, ErrInvalidName) {
		t.Fatal("path accepted")
	}
}
