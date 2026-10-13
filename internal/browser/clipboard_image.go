package browser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"math/bits"
)

var ErrClipboardImage = errors.New("unsupported clipboard image")

// Bound native payloads and decoded images before allocating pixel buffers.
const maxClipboardImageBytes = 256 << 20

func clipboardPNG(data []byte, dib bool) ([]byte, error) {
	if !dib {
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || !validImageSize(int64(config.Width), int64(config.Height)) {
			return nil, ErrClipboardImage
		}
		if _, err := png.Decode(bytes.NewReader(data)); err != nil {
			return nil, ErrClipboardImage
		}
		return data, nil
	}
	img, err := decodeClipboardDIB(data)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func validImageSize(width, height int64) bool {
	return width > 0 && height > 0 && width <= maxClipboardImageBytes/4 && height <= maxClipboardImageBytes/4/width
}

// Packed Windows DIBs contain a header, optional masks/palette, then padded
// pixel rows. BI_RGB's unused high byte is opaque; explicit alpha masks retain
// transparency. Both top-down and bottom-up images become ordinary Go images.
func decodeClipboardDIB(data []byte) (*image.NRGBA, error) {
	if len(data) < 40 {
		return nil, ErrClipboardImage
	}
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }
	header := int(u32(0))
	if header != 40 && header != 52 && header != 56 && header != 108 && header != 124 || header > len(data) {
		return nil, ErrClipboardImage
	}
	width, height := int64(int32(u32(4))), int64(int32(u32(8)))
	topDown := height < 0
	if topDown {
		height = -height
	}
	if !validImageSize(width, height) || binary.LittleEndian.Uint16(data[12:]) != 1 {
		return nil, ErrClipboardImage
	}
	depth := int(binary.LittleEndian.Uint16(data[14:]))
	compression := u32(16)
	if compression != 0 && compression != 3 || depth != 1 && depth != 4 && depth != 8 && depth != 16 && depth != 24 && depth != 32 {
		return nil, ErrClipboardImage
	}
	offset := header
	masks := [4]uint32{0xff0000, 0xff00, 0xff, 0}
	if depth == 16 {
		masks = [4]uint32{0x7c00, 0x3e0, 0x1f, 0}
	}
	if compression == 3 {
		if depth != 16 && depth != 32 {
			return nil, ErrClipboardImage
		}
		maskOffset := 40
		if header == 40 {
			offset += 12
		}
		if offset > len(data) {
			return nil, ErrClipboardImage
		}
		for i := 0; i < 3; i++ {
			masks[i] = u32(maskOffset + i*4)
		}
	}
	if header >= 56 && depth == 32 {
		masks[3] = u32(52)
	}
	if depth == 16 || depth == 32 {
		var used uint32
		for i, mask := range masks {
			if mask == 0 {
				if i < 3 {
					return nil, ErrClipboardImage
				}
				continue
			}
			shifted := mask >> bits.TrailingZeros32(mask)
			if shifted&(shifted+1) != 0 || mask&used != 0 || depth == 16 && mask>>16 != 0 {
				return nil, ErrClipboardImage
			}
			used |= mask
		}
	}
	colors := int64(u32(32))
	if depth <= 8 && colors == 0 {
		colors = 1 << depth
	}
	if depth <= 8 && colors > 1<<depth || colors*4 > int64(len(data)-offset) {
		return nil, ErrClipboardImage
	}
	palette := data[offset : offset+int(colors)*4]
	offset += int(colors) * 4
	stride := ((width*int64(depth) + 31) / 32) * 4
	if stride*height > int64(len(data)-offset) {
		return nil, ErrClipboardImage
	}
	img := image.NewNRGBA(image.Rect(0, 0, int(width), int(height)))
	component := func(value, mask uint32) byte {
		if mask == 0 {
			return 255
		}
		shift := bits.TrailingZeros32(mask)
		maximum := uint64(mask >> shift)
		return byte((uint64((value&mask)>>shift)*255 + maximum/2) / maximum)
	}
	for y := 0; y < int(height); y++ {
		sourceY := y
		if !topDown {
			sourceY = int(height) - 1 - y
		}
		row := data[offset+sourceY*int(stride) : offset+(sourceY+1)*int(stride)]
		for x := 0; x < int(width); x++ {
			dest := img.Pix[y*img.Stride+x*4:][:4]
			switch depth {
			case 1, 4, 8:
				index := int(row[x*depth/8]>>(8-depth-(x*depth%8))) & ((1 << depth) - 1)
				if int64(index) >= colors {
					return nil, ErrClipboardImage
				}
				copy(dest, []byte{palette[index*4+2], palette[index*4+1], palette[index*4], 255})
			case 24:
				copy(dest, []byte{row[x*3+2], row[x*3+1], row[x*3], 255})
			default:
				var value uint32
				if depth == 16 {
					value = uint32(binary.LittleEndian.Uint16(row[x*2:]))
				} else {
					value = binary.LittleEndian.Uint32(row[x*4:])
				}
				for i, mask := range masks {
					dest[i] = component(value, mask)
				}
			}
		}
	}
	return img, nil
}
