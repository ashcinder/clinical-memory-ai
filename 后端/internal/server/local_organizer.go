package server

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) organizeLocally(ctx context.Context, system, prompt string) (string, error) {
	if !regularFile(a.Config.LocalOrganizerModel) {
		return "", apiError{503, "离线整理模型不可读，请检查服务配置"}
	}
	cli, err := exec.LookPath(a.Config.LocalOrganizerCLI)
	if err != nil {
		return "", apiError{503, "找不到本机整理推理程序 llama-cli"}
	}
	if len([]rune(system))+len([]rune(prompt)) > 16000 {
		return "", apiError{422, "本批材料超过离线模型上下文，请按就诊或主题分批整理"}
	}
	if !a.localOrganizerBusy.CompareAndSwap(false, true) {
		return "", apiError{429, "正在本机整理另一份材料，请稍后重试"}
	}
	defer a.localOrganizerBusy.Store(false)

	dir, err := os.MkdirTemp("", "linji-organizer-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	promptFile, systemFile := filepath.Join(dir, "materials.txt"), filepath.Join(dir, "instructions.txt")
	if err := os.WriteFile(promptFile, []byte(prompt), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(systemFile, []byte(system), 0600); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 155*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, "--model", a.Config.LocalOrganizerModel,
		"--conversation", "--single-turn", "--simple-io", "--no-display-prompt", "--no-show-timings",
		"--no-perf", "--log-disable", "--no-warmup", "--reasoning", "off",
		"--ctx-size", "16384", "--no-context-shift", "--n-predict", "2400", "--temp", "0",
		"--system-prompt-file", systemFile, "--file", promptFile)
	cmd.Stdin = bytes.NewReader(nil)
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return "", apiError{504, "本机整理超时，请减少本批材料后重试"}
	}
	if err != nil {
		return "", apiError{502, "本机模型运行失败，请检查 llama-cli 与模型版本，或减少本批材料"}
	}
	// The CLI may truncate its prompt echo or include a banner. Accept only a
	// complete result object at the end, never an example from the echoed input.
	response := strings.TrimSpace(string(output))
	response = strings.TrimSpace(strings.TrimSuffix(response, "Exiting..."))
	response = strings.TrimSpace(strings.TrimSuffix(response, "```"))
	for i := strings.IndexByte(response, '{'); i >= 0; i = strings.IndexByte(response, '{') {
		response = response[i:]
		var result struct {
			Title string          `json:"title"`
			Items json.RawMessage `json:"items"`
		}
		if json.Unmarshal([]byte(response), &result) == nil && result.Title != "" && len(result.Items) > 0 {
			return response, nil
		}
		response = response[1:]
	}
	return "", apiError{502, "本机模型未返回完整草稿，请减少本批材料或重试"}
}
