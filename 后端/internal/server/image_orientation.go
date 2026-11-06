package server

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

// Honor phone-camera JPEG orientation before hashing or sending a normalized
// image to a model. A wrapper avoids allocating a full-resolution rotated copy.
func orientJPEG(src image.Image, raw []byte) image.Image {
	o := jpegOrientation(raw)
	if o < 2 || o > 8 {
		return src
	}
	return orientedImage{src, o}
}

func jpegOrientation(raw []byte) int {
	if len(raw) < 4 || raw[0] != 0xff || raw[1] != 0xd8 {
		return 1
	}
	for i := 2; i+4 <= len(raw); {
		if raw[i] != 0xff {
			return 1
		}
		marker := raw[i+1]
		i += 2
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0xff {
			i--
			continue
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		n := int(binary.BigEndian.Uint16(raw[i : i+2]))
		if n < 2 || i+n > len(raw) {
			return 1
		}
		payload := raw[i+2 : i+n]
		i += n
		if marker != 0xe1 || len(payload) < 14 || !bytes.Equal(payload[:6], []byte("Exif\x00\x00")) {
			continue
		}
		tiff := payload[6:]
		var order binary.ByteOrder
		switch string(tiff[:2]) {
		case "II":
			order = binary.LittleEndian
		case "MM":
			order = binary.BigEndian
		default:
			return 1
		}
		if order.Uint16(tiff[2:4]) != 42 {
			return 1
		}
		offset := uint64(order.Uint32(tiff[4:8]))
		if offset+2 > uint64(len(tiff)) {
			return 1
		}
		count := int(order.Uint16(tiff[offset : offset+2]))
		start := int(offset) + 2
		for j := 0; j < count && start+12 <= len(tiff); j++ {
			entry := tiff[start : start+12]
			start += 12
			if order.Uint16(entry[:2]) == 0x112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
				v := int(order.Uint16(entry[8:10]))
				if v >= 1 && v <= 8 {
					return v
				}
				return 1
			}
		}
	}
	return 1
}

type orientedImage struct {
	image.Image
	orientation int
}

func (i orientedImage) Bounds() image.Rectangle {
	b := i.Image.Bounds()
	if i.orientation >= 5 {
		return image.Rect(0, 0, b.Dy(), b.Dx())
	}
	return image.Rect(0, 0, b.Dx(), b.Dy())
}
func (i orientedImage) At(x, y int) color.Color {
	b := i.Image.Bounds()
	w, h := b.Dx(), b.Dy()
	switch i.orientation {
	case 2:
		x = w - 1 - x
	case 3:
		x, y = w-1-x, h-1-y
	case 4:
		y = h - 1 - y
	case 5:
		x, y = y, x
	case 6:
		x, y = y, h-1-x
	case 7:
		x, y = w-1-y, h-1-x
	case 8:
		x, y = w-1-y, x
	}
	return i.Image.At(b.Min.X+x, b.Min.Y+y)
}
