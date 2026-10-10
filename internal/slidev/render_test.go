package slidev

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt in with real Slidev and Chromium paths. No Google credentials are used.
func TestRenderResolvedPages(t *testing.T) {
	cli, chrome := os.Getenv("GSLIDEV_TEST_SLIDEV"), os.Getenv("GSLIDEV_TEST_CHROME")
	if cli == "" || chrome == "" {
		t.Skip("set GSLIDEV_TEST_SLIDEV and GSLIDEV_TEST_CHROME for browser integration test")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "slides.md")
	markdown := "---\ntitle: Render test\ntheme: default\nfonts:\n  provider: none\n---\n## Equation\n\n$$\nE = mc^2\n$$\n\n<v-click>Final click</v-click>\n\n<!-- First note -->\n\n---\nlayout: center\n---\n## Second page\n\nSecond body\n\n<!-- Second note -->\n\n---\nsrc: ./included.md\n---\n"
	if err := os.WriteFile(input, []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	// A text search also finds this selector inside Vite's injected <style>.
	// Page lookup must use a CSS query rather than DOM.performSearch.
	if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte(`[data-slidev-no="2"] .slidev-layout { font-size: 24px; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "included.md"), []byte("## Imported page\n\nImported body\n\n<!-- Third note -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deck, err := ParseFile(input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := RenderFile(ctx, input, deck, RenderOptions{SlidevPath: cli, ChromePath: chrome}); err != nil {
		t.Fatal(err)
	}
	if len(deck.Slides) != 3 {
		t.Fatalf("resolved %d slides, want 3", len(deck.Slides))
	}
	for i, note := range []string{"First note", "Second note", "Third note"} {
		if deck.Slides[i].Notes != note {
			t.Fatalf("page %d notes = %q", i+1, deck.Slides[i].Notes)
		}
		image, err := png.Decode(bytes.NewReader(deck.Slides[i].Screenshot))
		if err != nil {
			t.Fatalf("page %d PNG: %v", i+1, err)
		}
		if image.Bounds().Dx() < 100 || image.Bounds().Dy() < 100 {
			t.Fatalf("page %d image too small", i+1)
		}
		// A valid PNG with only the background is still a broken capture.
		minR, maxR := uint32(65535), uint32(0)
		for y := image.Bounds().Min.Y; y < image.Bounds().Max.Y; y += 4 {
			for x := image.Bounds().Min.X; x < image.Bounds().Max.X; x += 4 {
				r, _, _, _ := image.At(x, y).RGBA()
				if r < minR {
					minR = r
				}
				if r > maxR {
					maxR = r
				}
			}
		}
		if maxR-minR < 10000 {
			t.Fatalf("page %d contains only a blank background", i+1)
		}
		if i > 0 && bytes.Equal(deck.Slides[i].Screenshot, deck.Slides[i-1].Screenshot) {
			t.Fatalf("page %d captured the previous page", i+1)
		}
	}
}
