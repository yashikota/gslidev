package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yashikota/gslidev/internal/slidev"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"
)

func TestSyncKeepsOriginalUntilStagedSlidesAreComplete(t *testing.T) {
	for _, failure := range []string{"", "upload", "images", "commit"} {
		t.Run("failure="+failure, func(t *testing.T) {
			pages := []*slides.Page{testPage("original")}
			populated := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, ":batchUpdate"):
					var batch slides.BatchUpdatePresentationRequest
					if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
						t.Error(err)
					}
					for _, req := range batch.Requests {
						if req.CreateImage != nil && failure == "images" {
							http.Error(w, `{"error":{"code":400,"message":"image failure"}}`, 400)
							return
						}
						if req.CreateImage != nil && req.CreateImage.ElementProperties.PageObjectId == "original" && failure == "commit" {
							http.Error(w, `{"error":{"code":400,"message":"commit failure"}}`, 400)
							return
						}
					}
					for _, req := range batch.Requests {
						if req.CreateSlide != nil {
							pages = append(pages, testPage(req.CreateSlide.ObjectId))
						}
						if req.CreateImage != nil {
							populated = true
						}
						if req.DeleteObject != nil {
							if req.DeleteObject.ObjectId == "original" && !populated {
								t.Error("original deleted before images and notes were installed")
							}
							for i, page := range pages {
								if page.ObjectId == req.DeleteObject.ObjectId {
									pages = append(pages[:i], pages[i+1:]...)
									break
								}
							}
						}
					}
					fmt.Fprint(w, `{}`)
				case strings.Contains(r.URL.Path, "presentations/"):
					_ = json.NewEncoder(w).Encode(&slides.Presentation{PresentationId: "deck", Slides: pages})
				case r.Method == "POST" && !strings.Contains(r.URL.Path, "permissions"):
					if failure == "upload" {
						http.Error(w, `{"error":{"code":400,"message":"upload failure"}}`, 400)
						return
					}
					fmt.Fprint(w, `{"id":"image"}`)
				case r.Method == "GET":
					fmt.Fprint(w, `{"webContentLink":"https://example.com/image.png"}`)
				default:
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			endpoint, _ := url.Parse(server.URL)
			httpClient := &http.Client{Transport: rewriteTransport{endpoint: endpoint, base: http.DefaultTransport}}
			ctx := context.Background()
			ss, err := slides.NewService(ctx, option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(httpClient))
			if err != nil {
				t.Fatal(err)
			}
			ds, err := drive.NewService(ctx, option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(httpClient))
			if err != nil {
				t.Fatal(err)
			}
			client := &Client{slides: ss, drive: ds}
			_, err = client.Sync(ctx, &slidev.Deck{PresentationID: "deck", Slides: []slidev.Slide{{Screenshot: []byte("png"), Notes: "Narration"}}})
			if failure == "" && err != nil {
				t.Fatal(err)
			}
			if failure != "" && err == nil {
				t.Fatal("expected failure")
			}
			if len(pages) != 1 {
				t.Fatalf("unexpected pages after sync: %v", pages)
			}
			if failure != "" && pages[0].ObjectId != "original" {
				t.Fatal("original page was lost")
			}
			if failure == "" && (pages[0].ObjectId != "original" || !populated) {
				t.Fatal("existing page ID was not preserved or populated")
			}
		})
	}
}

type rewriteTransport struct {
	endpoint *url.URL
	base     http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = rt.endpoint.Scheme
	req.URL.Host = rt.endpoint.Host
	return rt.base.RoundTrip(req)
}

func testPage(id string) *slides.Page {
	return &slides.Page{ObjectId: id, SlideProperties: &slides.SlideProperties{NotesPage: &slides.Page{PageElements: []*slides.PageElement{{ObjectId: id + "-notes", Shape: &slides.Shape{Placeholder: &slides.Placeholder{Type: "BODY"}}}}}}}
}

func TestCommitPreservesPagesAndReplacesNotes(t *testing.T) {
	old := testPage("old")
	old.PageElements = []*slides.PageElement{{ObjectId: "old-image"}}
	staged := testPage("staged")
	image := imageRequest("staged", "https://example.com/image.png", 0, 0, 720, 405)
	requests, err := commitSlides([]*slides.Page{old, testPage("surplus")}, []*slides.Page{staged}, []*slides.Request{
		image,
		{DeleteText: &slides.DeleteTextRequest{ObjectId: "staged-notes", TextRange: &slides.Range{Type: "ALL"}}},
		{InsertText: &slides.InsertTextRequest{ObjectId: "staged-notes", Text: "Narration"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var replacedImage, replacedNotes, deletedStaged, deletedSurplus bool
	for _, req := range requests {
		if req.CreateImage != nil {
			replacedImage = req.CreateImage.ElementProperties.PageObjectId == "old" && req.CreateImage.ObjectId != image.CreateImage.ObjectId
		}
		if req.InsertText != nil {
			replacedNotes = req.InsertText.ObjectId == "old-notes" && req.InsertText.Text == "Narration"
		}
		if req.DeleteObject != nil {
			switch req.DeleteObject.ObjectId {
			case "old":
				t.Fatal("existing page deleted")
			case "staged":
				deletedStaged = true
			case "surplus":
				deletedSurplus = true
			}
		}
	}
	if !replacedImage || !replacedNotes || !deletedStaged || !deletedSurplus {
		t.Fatalf("incomplete commit: %#v", requests)
	}
	if image.CreateImage.ElementProperties.PageObjectId != "staged" {
		t.Fatal("staging request was mutated")
	}
}
