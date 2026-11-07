package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Serial inference bounds CPU and memory when several local users search.
var dinoSlot = make(chan struct{}, 1)

type dinoImage struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	SHA  string `json:"sha"`
}

func (a *App) dinoRanks(ctx context.Context, queries, gallery []dinoImage) ([]string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	select {
	case dinoSlot <- struct{}{}:
		defer func() { <-dinoSlot }()
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}

	python := a.Config.ImageDINOPython
	if python == "" {
		python = "python3"
	}
	worker := filepath.Join(a.Config.Root, "后端", "tools", "open_image_eval", "retrieve.py")
	cache := filepath.Join(a.Config.DataDir, "image-embeddings")
	cmd := exec.CommandContext(ctx, python, worker, "--model", a.Config.ImageDINOModel, "--cache", cache)
	cmd.Env = append(os.Environ(), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "OPENBLAS_NUM_THREADS=1", "VECLIB_MAXIMUM_THREADS=1")
	input, err := json.Marshal(map[string][]dinoImage{"queries": queries, "gallery": gallery})
	if err != nil {
		return nil, 0, err
	}
	cmd.Stdin = bytes.NewReader(input)
	raw, err := cmd.Output()
	if err != nil {
		return nil, 0, err
	}
	var result struct {
		IDs     []string `json:"ids"`
		Skipped int      `json:"skipped"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, 0, err
	}
	if len(result.IDs) == 0 || result.Skipped < 0 || len(result.IDs)+result.Skipped != len(gallery) {
		return nil, 0, fmt.Errorf("incomplete DINOv2 ranking")
	}
	ids := make(map[string]bool, len(gallery))
	for _, item := range gallery {
		ids[item.ID] = true
	}
	for _, id := range result.IDs {
		if !ids[id] {
			return nil, 0, fmt.Errorf("invalid DINOv2 candidate")
		}
		delete(ids, id)
	}
	return result.IDs, result.Skipped, nil
}

// Preserve strong fingerprints, then reserve space for both visual and lexical
// recall. The returned order only controls which images get verified.
func fuseImageCandidates(items []Attachment, ranks []string, strong map[string]bool, limit int) []Attachment {
	byID := make(map[string]Attachment, len(items))
	chosen := make(map[string]bool, limit)
	out := make([]Attachment, 0, limit)
	add := func(item Attachment) {
		if len(out) < limit && !chosen[item.ID] {
			chosen[item.ID] = true
			out = append(out, item)
		}
	}
	for _, item := range items {
		byID[item.ID] = item
		if strong[item.ID] {
			add(item)
		}
	}
	visualEnd := len(out) + (3*(limit-len(out))+3)/4
	for _, id := range ranks {
		if len(out) >= visualEnd {
			break
		}
		if item, ok := byID[id]; ok {
			add(item)
		}
	}
	for _, item := range items {
		add(item)
		if len(out) == limit {
			break
		}
	}
	return out
}
