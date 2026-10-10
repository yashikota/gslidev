package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/yashikota/gslidev/internal/google"
	"github.com/yashikota/gslidev/internal/slidev"
)

var Version string

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gslidev:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("gslidev", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	presentationID := flags.String("presentation-id", "", "Google Slides presentation ID")
	title := flags.String("title", "", "title for a newly created presentation")
	folderID := flags.String("folder-id", "", "Google Drive folder ID for a newly created presentation and temporary images")
	profile := flags.String("profile", "", "OAuth profile name")
	chromePath := flags.String("chrome-path", "", "path to Chrome or Chromium")
	slidevPath := flags.String("slidev-path", "", "path to the slidev executable")
	verbose := flags.Bool("verbose", false, "enable debug logging")
	output := flags.String("output", "", "write rendered PNGs and notes locally without Google authorization")
	pdf := flags.Bool("pdf", false, "export PDF from Slidev and upload it beside the Google Slides presentation")
	version := flags.Bool("version", false, "print version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *version {
		fmt.Println(versionString())
		return nil
	}
	if flags.NArg() != 1 {
		return errors.New("usage: gslidev [flags] slides.md")
	}
	if *pdf && *output != "" {
		return errors.New("--pdf uploads to Google Drive and cannot be combined with --output")
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	deck, err := slidev.ParseFile(flags.Arg(0))
	if err != nil {
		return fmt.Errorf("parse Slidev markdown: %w", err)
	}
	if *presentationID != "" {
		deck.PresentationID = *presentationID
	}
	if *title != "" {
		deck.Title = *title
	}
	if *folderID != "" {
		deck.FolderID = *folderID
	}
	logger.Info("rendering Slidev presentation")
	if err := slidev.RenderFile(ctx, flags.Arg(0), deck, slidev.RenderOptions{ChromePath: *chromePath, SlidevPath: *slidevPath}); err != nil {
		return fmt.Errorf("render slides: %w", err)
	}
	logger.Info("rendered Slidev presentation", slog.Int("slides", len(deck.Slides)))
	if *output != "" {
		return writePreview(*output, deck)
	}
	var pdfData []byte
	if *pdf {
		logger.Info("exporting PDF from Slidev")
		pdfData, err = slidev.ExportPDFFile(ctx, flags.Arg(0), slidev.RenderOptions{ChromePath: *chromePath, SlidevPath: *slidevPath})
		if err != nil {
			return fmt.Errorf("export Slidev PDF: %w", err)
		}
	}

	client, err := google.New(ctx, *profile)
	if err != nil {
		return fmt.Errorf("initialize Google client: %w", err)
	}
	id, err := client.Sync(ctx, deck)
	if err != nil {
		return fmt.Errorf("sync Google Slides: %w", err)
	}
	fmt.Printf("https://docs.google.com/presentation/d/%s/edit\n", id)
	if *pdf {
		file, err := client.UploadPDF(ctx, id, pdfData)
		if err != nil {
			return fmt.Errorf("Google Slides synced, but PDF upload failed: %w", err)
		}
		fmt.Printf("https://drive.google.com/file/d/%s/view\n", file.Id)
	}
	return nil
}

func writePreview(dir string, deck *slidev.Deck) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	manifest := make([]map[string]any, len(deck.Slides))
	for i, page := range deck.Slides {
		stem := fmt.Sprintf("%03d", i+1)
		if err := os.WriteFile(filepath.Join(dir, stem+".png"), page.Screenshot, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, stem+".txt"), []byte(page.Notes), 0o644); err != nil {
			return err
		}
		var gifs []map[string]any
		for j, gif := range page.GIFs {
			name := fmt.Sprintf("%s-%02d.gif", stem, j+1)
			if err := os.WriteFile(filepath.Join(dir, name), gif.Data, 0o644); err != nil {
				return err
			}
			gifs = append(gifs, map[string]any{"file": name, "x": gif.X, "y": gif.Y, "width": gif.Width, "height": gif.Height})
		}
		manifest[i] = map[string]any{"slide": i + 1, "image": stem + ".png", "notes": stem + ".txt", "gifs": gifs}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644)
}

func versionString() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "(devel)"
}
