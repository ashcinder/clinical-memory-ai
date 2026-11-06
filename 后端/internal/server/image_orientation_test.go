package server

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func TestJPEGOrientation(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for orientation := 1; orientation <= 8; orientation++ {
			tiff := make([]byte, 26)
			if order == binary.LittleEndian {
				copy(tiff, "II")
			} else {
				copy(tiff, "MM")
			}
			order.PutUint16(tiff[2:4], 42)
			order.PutUint32(tiff[4:8], 8)
			order.PutUint16(tiff[8:10], 1)
			order.PutUint16(tiff[10:12], 0x112)
			order.PutUint16(tiff[12:14], 3)
			order.PutUint32(tiff[14:18], 1)
			order.PutUint16(tiff[18:20], uint16(orientation))
			raw := []byte{0xff, 0xd8, 0xff, 0xe1, 0, 34}
			raw = append(raw, []byte("Exif\x00\x00")...)
			raw = append(raw, tiff...)
			raw = append(raw, 0xff, 0xd9)
			if got := jpegOrientation(raw); got != orientation {
				t.Fatalf("orientation %d got %d", orientation, got)
			}
			for n := 0; n < len(raw); n++ {
				_ = jpegOrientation(raw[:n])
			}
		}
	}
	src := image.NewGray(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.SetGray(x, y, color.Gray{uint8(y*2 + x + 1)})
		}
	}
	expected := map[int][]uint8{2: {2, 1, 4, 3, 6, 5}, 3: {6, 5, 4, 3, 2, 1}, 4: {5, 6, 3, 4, 1, 2}, 5: {1, 3, 5, 2, 4, 6}, 6: {5, 3, 1, 6, 4, 2}, 7: {6, 4, 2, 5, 3, 1}, 8: {2, 4, 6, 1, 3, 5}}
	for orientation, want := range expected {
		im := orientedImage{src, orientation}
		b := im.Bounds()
		index := 0
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				if got := color.GrayModel.Convert(im.At(x, y)).(color.Gray).Y; got != want[index] {
					t.Fatalf("orientation %d index %d got %d want %d", orientation, index, got, want[index])
				}
				index++
			}
		}
	}
}
