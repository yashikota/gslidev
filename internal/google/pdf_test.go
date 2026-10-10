package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestUploadPDFCreatesOrUpdatesBesidePresentation(t *testing.T) {
	for _, mode := range []string{"create", "update", "move", "root", "ambiguous", "upload-failure"} {
		t.Run(mode, func(t *testing.T) {
			uploads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/files/deck"):
					if mode == "root" {
						fmt.Fprint(w, `{"id":"deck","name":"JA deck"}`)
					} else {
						fmt.Fprint(w, `{"id":"deck","name":"JA deck","parents":["folder"]}`)
					}
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/files/root"):
					fmt.Fprint(w, `{"id":"root-id"}`)
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/files"):
					if !strings.Contains(r.URL.Query().Get("q"), pdfSourceProperty) {
						t.Error("must search by source marker, not filename")
					}
					switch mode {
					case "update":
						fmt.Fprint(w, `{"files":[{"id":"pdf","parents":["folder"]}]}`)
					case "move":
						fmt.Fprint(w, `{"files":[{"id":"pdf","parents":["old-folder"]}]}`)
					case "ambiguous":
						fmt.Fprint(w, `{"files":[{"id":"pdf"},{"id":"other"}]}`)
					default:
						fmt.Fprint(w, `{"files":[]}`)
					}
				case r.Method == "POST" || r.Method == "PATCH":
					uploads++
					if mode == "upload-failure" {
						http.Error(w, `{"error":{"code":403,"message":"denied"}}`, 403)
						return
					}
					_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
					if err != nil {
						t.Error(err)
						return
					}
					reader := multipart.NewReader(r.Body, params["boundary"])
					part, err := reader.NextPart()
					if err != nil {
						t.Error(err)
						return
					}
					var metadata drive.File
					if err = json.NewDecoder(part).Decode(&metadata); err != nil {
						t.Error(err)
						return
					}
					if metadata.Name != "JA deck.pdf" || metadata.Properties[pdfSourceProperty] != "deck" {
						t.Errorf("metadata %+v", metadata)
					}
					part, err = reader.NextPart()
					if err != nil {
						t.Error(err)
						return
					}
					data, _ := io.ReadAll(part)
					if string(data) != "%PDF-test" {
						t.Error("PDF bytes changed")
					}
					if mode == "update" || mode == "move" {
						if r.Method != "PATCH" || !strings.HasSuffix(r.URL.Path, "/files/pdf") {
							t.Errorf("expected update: %s %s", r.Method, r.URL.Path)
						}
						if len(metadata.Parents) > 0 {
							t.Error("parents must be updated through query parameters")
						}
						if mode == "move" && (r.URL.Query().Get("addParents") != "folder" || r.URL.Query().Get("removeParents") != "old-folder") {
							t.Error("PDF must follow presentation's folder")
						}
					} else {
						parent := "folder"
						if mode == "root" {
							parent = "root-id"
						}
						if r.Method != "POST" || len(metadata.Parents) != 1 || metadata.Parents[0] != parent {
							t.Errorf("wrong creation folder: %+v", metadata.Parents)
						}
					}
					fmt.Fprint(w, `{"id":"pdf"}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 400)
				}
			}))
			defer server.Close()
			endpoint, _ := url.Parse(server.URL)
			hc := &http.Client{Transport: rewriteTransport{endpoint: endpoint, base: http.DefaultTransport}}
			svc, err := drive.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(hc))
			if err != nil {
				t.Fatal(err)
			}
			file, err := (&Client{drive: svc}).UploadPDF(context.Background(), "deck", []byte("%PDF-test"))
			if mode == "ambiguous" || mode == "upload-failure" {
				if err == nil {
					t.Fatal("expected error")
				}
				if mode == "ambiguous" && uploads != 0 {
					t.Fatal("ambiguous PDFs must not be overwritten")
				}
			} else if err != nil || file.Id != "pdf" || uploads != 1 {
				t.Fatalf("file=%+v error=%v uploads=%d", file, err, uploads)
			}
		})
	}
}
