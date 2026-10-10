package google

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

const pdfSourceProperty = "gslidevPresentationID"

// UploadPDF stores a Slidev-exported PDF in the presentation's actual Drive
// folder. Only PDFs marked as generated for this presentation are updated.
func (c *Client) UploadPDF(ctx context.Context, presentationID string, data []byte) (*drive.File, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, fmt.Errorf("invalid PDF data")
	}
	source, err := c.drive.Files.Get(presentationID).Fields("id,name,parents").SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get presentation folder: %w", err)
	}
	parents := source.Parents
	if len(parents) == 0 {
		root, err := c.drive.Files.Get("root").Fields("id").Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("get My Drive root: %w", err)
		}
		parents = []string{root.Id}
	}
	escapedID := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(presentationID)
	matches, err := c.drive.Files.List().Q("trashed = false and mimeType = 'application/pdf' and properties has { key = '" + pdfSourceProperty + "' and value = '" + escapedID + "' }").PageSize(2).Fields("files(id,parents),nextPageToken").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("find generated PDF: %w", err)
	}
	if len(matches.Files) > 1 || matches.NextPageToken != "" {
		return nil, fmt.Errorf("multiple generated PDFs found for presentation %s; refusing to overwrite an arbitrary file", presentationID)
	}
	meta := &drive.File{Name: source.Name + ".pdf", MimeType: "application/pdf", Properties: map[string]string{pdfSourceProperty: presentationID}}
	if len(matches.Files) == 0 {
		meta.Parents = parents
		file, err := c.drive.Files.Create(meta).Media(bytes.NewReader(data), googleapi.ContentType("application/pdf")).Fields("id,name,parents,webViewLink").SupportsAllDrives(true).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("create PDF: %w", err)
		}
		return file, nil
	}
	existing := matches.Files[0]
	call := c.drive.Files.Update(existing.Id, meta).Media(bytes.NewReader(data), googleapi.ContentType("application/pdf")).Fields("id,name,parents,webViewLink").SupportsAllDrives(true).Context(ctx)
	add := parentDifference(parents, existing.Parents)
	remove := parentDifference(existing.Parents, parents)
	if len(add) > 0 {
		call = call.AddParents(strings.Join(add, ","))
	}
	if len(remove) > 0 {
		call = call.RemoveParents(strings.Join(remove, ","))
	}
	file, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("update PDF: %w", err)
	}
	return file, nil
}

func parentDifference(a, b []string) []string {
	var diff []string
	for _, value := range a {
		found := false
		for _, other := range b {
			if value == other {
				found = true
				break
			}
		}
		if !found {
			diff = append(diff, value)
		}
	}
	return diff
}
