package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
	logger.Info("rendering Slidev presentation", slog.Int("slides", len(deck.Slides)))
	if err := slidev.RenderFile(ctx, flags.Arg(0), deck, slidev.RenderOptions{ChromePath: *chromePath, SlidevPath: *slidevPath}); err != nil {
		return fmt.Errorf("render slides: %w", err)
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
	return nil
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
