package server

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) ocrDir() string {
	if a.Config.OCRDataDir != "" {
		return a.Config.OCRDataDir
	}
	return filepath.Join(a.Config.DataDir, "models", "tessdata")
}
func (a *App) asrModel() string {
	if a.Config.LocalASRModel != "" {
		return a.Config.LocalASRModel
	}
	return filepath.Join(a.Config.DataDir, "models", "ggml-base.bin")
}
func regularFile(path string) bool {
	s, e := os.Stat(path)
	return e == nil && s.Mode().IsRegular() && s.Size() > 0
}
func (a *App) nativeOCR() string {
	return filepath.Join(a.Config.DataDir, "models", "tools", "linji-ocr")
}
func (a *App) localPDFText(ctx context.Context, path string) (string, error) {
	if program, err := exec.LookPath("pdftotext"); err == nil {
		return commandText(ctx, program, "-f", "1", "-l", "100", "-enc", "UTF-8", path, "-")
	}
	return commandText(ctx, filepath.Join(a.Config.DataDir, "models", "tools", "linji-pdf"), path)
}
func (a *App) localOCRReady() bool {
	if regularFile(a.nativeOCR()) {
		return true
	}
	_, err := exec.LookPath("tesseract")
	return err == nil && regularFile(filepath.Join(a.ocrDir(), "chi_sim.traineddata")) && regularFile(filepath.Join(a.ocrDir(), "eng.traineddata"))
}
func (a *App) localASRReady() bool {
	_, err := exec.LookPath("whisper-cli")
	_, convertErr := exec.LookPath("ffmpeg")
	_, probeErr := exec.LookPath("ffprobe")
	return err == nil && convertErr == nil && probeErr == nil && regularFile(a.asrModel())
}
func (a *App) localOCR(ctx context.Context, path string) (string, error) {
	if !a.localOCRReady() {
		return "", apiError{503, "本地 OCR 未就绪，请运行后端/setup-local-search.sh，或手动补充文字；图片仍可通过指纹检索"}
	}
	if regularFile(a.nativeOCR()) {
		text, err := commandText(ctx, a.nativeOCR(), path)
		if err != nil {
			return "", apiError{502, "本地图片文字识别失败，可重新提取或手动补充文字"}
		}
		return strings.TrimSpace(text), nil
	}
	// The command is local only; no document or API key is passed to a network service.
	text, err := commandText(ctx, "tesseract", path, "stdout", "--tessdata-dir", a.ocrDir(), "-l", "chi_sim+eng", "--psm", "11")
	if err != nil {
		return "", apiError{502, "本地图片文字识别失败，可重新提取或手动补充文字"}
	}
	return strings.TrimSpace(text), nil
}
func (a *App) localTranscribe(parent context.Context, path string) (string, error) {
	if !a.localASRReady() {
		return "", apiError{503, "本地音频转写未就绪，请运行后端/setup-local-search.sh，或手动补充转写文字"}
	}
	duration, probeErr := commandText(parent, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	seconds, parseErr := strconv.ParseFloat(strings.TrimSpace(duration), 64)
	if probeErr != nil || parseErr != nil || seconds <= 0 {
		return "", apiError{422, "音频无法解码，请检查文件内容"}
	}
	if seconds > 120 {
		return "", apiError{422, "本地音频转写支持 120 秒以内的录音，请分段上传或手动补充文字"}
	}
	dir, err := os.MkdirTemp("", "linji-asr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(parent, 150*time.Second)
	defer cancel()
	wav := filepath.Join(dir, "audio.wav")
	// Limit recognition to 120 seconds and bound decoding duration/size before inference.
	convert := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", path, "-t", "120", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav)
	convert.Stdout = io.Discard
	convert.Stderr = io.Discard
	if err = convert.Run(); err != nil {
		return "", apiError{422, "音频无法解码，请使用有效的 WAV、MP3、M4A 或 WebM 文件"}
	}
	output := filepath.Join(dir, "transcript")
	cmd := exec.CommandContext(ctx, "whisper-cli", "-m", a.asrModel(), "-f", wav, "-l", "auto", "--prompt", "中文资料检索录音，使用简体中文。常见专有词：合谷 LI4、足三里 ST36、内关 PC6。", "-otxt", "-of", output, "-nt", "-np", "-t", "4", "-ng")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return "", apiError{502, "本地音频转写失败或超时，请缩短录音后重试"}
	}
	raw, err := os.ReadFile(output + ".txt")
	if err != nil {
		return "", apiError{502, "本地转写未生成文字"}
	}
	return strings.TrimSpace(string(raw)), nil
}
