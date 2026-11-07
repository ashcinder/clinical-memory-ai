package server

import (
	"slices"
	"testing"
)

func TestFuseImageCandidatesKeepsFingerprintAndLexicalResults(t *testing.T) {
	var items []Attachment
	for _, id := range []string{"text-1", "exact", "text-2", "visual-1", "visual-2", "visual-3", "visual-4", "visual-5", "visual-6"} {
		items = append(items, Attachment{ID: id})
	}
	ranks := []string{"visual-1", "exact", "visual-2", "visual-1", "visual-3", "visual-4", "visual-5", "visual-6"}
	got := fuseImageCandidates(items, ranks, map[string]bool{"exact": true}, 5)
	var ids []string
	for _, item := range got {
		ids = append(ids, item.ID)
	}
	want := []string{"exact", "visual-1", "visual-2", "visual-3", "text-1"}
	if !slices.Equal(ids, want) {
		t.Fatalf("candidates = %v, want %v", ids, want)
	}
}
