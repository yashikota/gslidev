package google

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yashikota/gslidev/internal/slidev"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"
)

const (
	defaultSlideWidthPT  = 720.0
	defaultSlideHeightPT = 405.0
	maxBatchRequests     = 500
)

type uploadedFile struct {
	ID  string
	URL string
}

// Sync replaces the target presentation with rendered slide images and notes.
func (c *Client) Sync(ctx context.Context, deck *slidev.Deck) (string, error) {
	if len(deck.Slides) == 0 {
		return "", errors.New("presentation has no slides")
	}
	presentationID := deck.PresentationID
	if presentationID == "" {
		name := deck.Title
		if name == "" {
			name = "Untitled"
		}
		file, err := c.drive.Files.Create(&drive.File{
			Name:     name,
			MimeType: "application/vnd.google-apps.presentation",
			Parents:  nonEmptySlice(deck.FolderID),
		}).SupportsAllDrives(true).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("create presentation: %w", err)
		}
		presentationID = file.Id
	} else if deck.Title != "" {
		if _, err := c.drive.Files.Update(presentationID, &drive.File{Name: deck.Title}).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
			return "", fmt.Errorf("update presentation title: %w", err)
		}
	}

	presentation, err := c.slides.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("get presentation: %w", err)
	}
	pageWidth, pageHeight := pageSize(presentation)
	if err := c.replaceSlides(ctx, presentation, len(deck.Slides)); err != nil {
		return "", err
	}
	presentation, err = c.slides.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("refresh presentation: %w", err)
	}
	if len(presentation.Slides) != len(deck.Slides) {
		return "", fmt.Errorf("created %d slides, want %d", len(presentation.Slides), len(deck.Slides))
	}

	var uploaded []uploadedFile
	cleanup := func() error {
		var cleanupErr error
		for _, file := range uploaded {
			if err := c.drive.Files.Delete(file.ID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete temporary image %s: %w", file.ID, err))
			}
		}
		return cleanupErr
	}
	defer func() { _ = cleanup() }()

	requests := make([]*slides.Request, 0, len(deck.Slides)*3)
	for i, rendered := range deck.Slides {
		page := presentation.Slides[i]
		png, err := c.uploadImage(ctx, deck.FolderID, fmt.Sprintf("gslidev-%d.png", i+1), "image/png", rendered.Screenshot)
		if err != nil {
			return "", fmt.Errorf("upload screenshot for slide %d: %w", i+1, err)
		}
		uploaded = append(uploaded, png)
		requests = append(requests, imageRequest(page.ObjectId, png.URL, 0, 0, pageWidth, pageHeight))

		for gifIndex, gif := range rendered.GIFs {
			file, err := c.uploadImage(ctx, deck.FolderID, fmt.Sprintf("gslidev-%d-%d.gif", i+1, gifIndex+1), gif.MIME, gif.Data)
			if err != nil {
				return "", fmt.Errorf("upload GIF for slide %d: %w", i+1, err)
			}
			uploaded = append(uploaded, file)
			requests = append(requests, imageRequest(page.ObjectId, file.URL,
				gif.X*pageWidth, gif.Y*pageHeight, gif.Width*pageWidth, gif.Height*pageHeight))
		}

		notesID := speakerNotesID(page)
		if notesID == "" {
			return "", fmt.Errorf("speaker notes body not found on slide %d", i+1)
		}
		requests = append(requests, &slides.Request{DeleteText: &slides.DeleteTextRequest{
			ObjectId: notesID, TextRange: &slides.Range{Type: "ALL"},
		}})
		if rendered.Notes != "" {
			requests = append(requests, &slides.Request{InsertText: &slides.InsertTextRequest{
				ObjectId: notesID, Text: rendered.Notes,
			}})
		}
	}
	if err := c.batchUpdate(ctx, presentationID, requests); err != nil {
		return "", fmt.Errorf("update slides: %w", err)
	}
	if err := cleanup(); err != nil {
		uploaded = nil
		return "", err
	}
	uploaded = nil
	return presentationID, nil
}

func (c *Client) replaceSlides(ctx context.Context, presentation *slides.Presentation, count int) error {
	deleteRequests := make([]*slides.Request, 0, len(presentation.Slides))
	for _, page := range presentation.Slides {
		deleteRequests = append(deleteRequests, &slides.Request{DeleteObject: &slides.DeleteObjectRequest{ObjectId: page.ObjectId}})
	}
	if err := c.batchUpdate(ctx, presentation.PresentationId, deleteRequests); err != nil {
		return fmt.Errorf("delete existing slides: %w", err)
	}

	var blankLayout string
	for _, layout := range presentation.Layouts {
		if layout.LayoutProperties != nil && layout.LayoutProperties.Name == "BLANK" {
			blankLayout = layout.ObjectId
			break
		}
	}
	createRequests := make([]*slides.Request, count)
	for i := range createRequests {
		create := &slides.CreateSlideRequest{InsertionIndex: int64(i)}
		if blankLayout != "" {
			create.SlideLayoutReference = &slides.LayoutReference{LayoutId: blankLayout}
		}
		createRequests[i] = &slides.Request{CreateSlide: create}
	}
	if err := c.batchUpdate(ctx, presentation.PresentationId, createRequests); err != nil {
		return fmt.Errorf("create slides: %w", err)
	}
	return nil
}

func (c *Client) batchUpdate(ctx context.Context, presentationID string, requests []*slides.Request) error {
	for start := 0; start < len(requests); start += maxBatchRequests {
		end := min(start+maxBatchRequests, len(requests))
		if _, err := c.slides.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
			Requests: requests[start:end],
		}).Context(ctx).Do(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) uploadImage(ctx context.Context, folderID, name, mime string, data []byte) (uploadedFile, error) {
	if len(data) == 0 {
		return uploadedFile{}, errors.New("image data is empty")
	}
	file := &drive.File{Name: name, MimeType: mime, Parents: nonEmptySlice(folderID)}
	uploaded, err := c.drive.Files.Create(file).Media(bytes.NewReader(data)).SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		return uploadedFile{}, err
	}
	if _, err := c.drive.Permissions.Create(uploaded.Id, &drive.Permission{Type: "anyone", Role: "reader"}).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
		_ = c.drive.Files.Delete(uploaded.Id).SupportsAllDrives(true).Context(ctx).Do()
		return uploadedFile{}, fmt.Errorf("make image readable: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = c.drive.Files.Delete(uploaded.Id).SupportsAllDrives(true).Context(ctx).Do()
		}
	}()
	metadata, err := c.drive.Files.Get(uploaded.Id).Fields("webContentLink").SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		return uploadedFile{}, err
	}
	if strings.TrimSpace(metadata.WebContentLink) == "" {
		return uploadedFile{}, fmt.Errorf("webContentLink is empty for %s", uploaded.Id)
	}
	keep = true
	return uploadedFile{ID: uploaded.Id, URL: metadata.WebContentLink}, nil
}

func imageRequest(pageID, imageURL string, x, y, width, height float64) *slides.Request {
	return &slides.Request{CreateImage: &slides.CreateImageRequest{
		ObjectId: "gslidev-image-" + uuid.NewString(),
		Url:      imageURL,
		ElementProperties: &slides.PageElementProperties{
			PageObjectId: pageID,
			Size: &slides.Size{
				Width:  &slides.Dimension{Magnitude: width, Unit: "PT"},
				Height: &slides.Dimension{Magnitude: height, Unit: "PT"},
			},
			Transform: &slides.AffineTransform{ScaleX: 1, ScaleY: 1, TranslateX: x, TranslateY: y, Unit: "PT"},
		},
	}}
}

func speakerNotesID(page *slides.Page) string {
	if page.SlideProperties == nil || page.SlideProperties.NotesPage == nil {
		return ""
	}
	for _, element := range page.SlideProperties.NotesPage.PageElements {
		if element.Shape != nil && element.Shape.Placeholder != nil && element.Shape.Placeholder.Type == "BODY" {
			return element.ObjectId
		}
	}
	return ""
}

func pageSize(presentation *slides.Presentation) (float64, float64) {
	if presentation.PageSize == nil || presentation.PageSize.Width == nil || presentation.PageSize.Height == nil {
		return defaultSlideWidthPT, defaultSlideHeightPT
	}
	return presentation.PageSize.Width.Magnitude, presentation.PageSize.Height.Magnitude
}

func nonEmptySlice(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}
