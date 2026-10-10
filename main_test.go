package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yashikota/gslidev/internal/slidev"
)

func TestPDFCannotUseLocalOnlyOutput(t *testing.T) {
	err := run(context.Background(), []string{"--pdf", "--output", "preview", "does-not-exist.md"})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("must reject before rendering or Google authorization: %v", err)
	}
}

func TestPreviewKeepsNotesAndGIFPlacement(t *testing.T) {
	dir := t.TempDir()
	note := "数式の読み上げ原稿\n以上になります。ありがとうございました。"
	deck := &slidev.Deck{Slides: []slidev.Slide{{Screenshot: []byte("png"), Notes: note, GIFs: []slidev.Media{{Data: []byte("gif"), X: .1, Y: .2, Width: .3, Height: .4}}}}}
	if err := writePreview(dir, deck); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(dir, "001.txt"))
	if err != nil || string(text) != note {
		t.Fatalf("notes = %q, error = %v", text, err)
	}
	gif, err := os.ReadFile(filepath.Join(dir, "001-01.gif"))
	if err != nil || string(gif) != "gif" {
		t.Fatalf("GIF = %q, error = %v", gif, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest []struct {
		Image string
		Notes string
		GIFs  []struct{ X, Y, Width, Height float64 }
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 || manifest[0].Image != "001.png" || manifest[0].Notes != "001.txt" || len(manifest[0].GIFs) != 1 || manifest[0].GIFs[0].X != .1 {
		t.Fatalf("manifest = %#v", manifest)
	}
}
