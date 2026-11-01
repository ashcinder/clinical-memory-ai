package server

import (
	xdraw "golang.org/x/image/draw"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) attachmentThumbnail(w http.ResponseWriter, r *http.Request, u User) {
	item := ownedAttachment(a.DB, r.PathValue("id"), u.ID, false)
	if !strings.HasPrefix(item.MIME, "image/") {
		fail(415, "该材料不是图片")
	}
	file, err := os.Open(filepath.Join(a.Config.DataDir, "uploads", item.StoredName))
	if os.IsNotExist(err) {
		fail(404, "图片原件不存在")
	}
	check(err)
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 50000000 {
		fail(400, "图片损坏或尺寸过大")
	}
	_, err = file.Seek(0, 0)
	check(err)
	source, _, err := image.Decode(file)
	if err != nil {
		fail(400, "无法读取图片预览")
	}
	width, height := config.Width, config.Height
	if max(width, height) > 384 {
		if width >= height {
			height = max(1, height*384/width)
			width = 384
		} else {
			width = max(1, width*384/height)
			height = 384
		}
	}
	preview := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(preview, preview.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(preview, preview.Bounds(), source, source.Bounds(), draw.Over, nil)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "inline")
	check(jpeg.Encode(w, preview, &jpeg.Options{Quality: 82}))
}
