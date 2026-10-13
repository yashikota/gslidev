package slidev

import "testing"

func TestParseFrontmatterAndNotes(t *testing.T) {
	deck, err := Parse([]byte(`---
title: Demo
presentationID: pres
gslidev:
  folderID: folder
---
# One

<!-- first note -->

---
# Two

<!--
second note
-->`))
	if err != nil {
		t.Fatal(err)
	}
	if deck.Title != "Demo" || deck.PresentationID != "pres" || deck.FolderID != "folder" {
		t.Fatalf("frontmatter = %#v", deck)
	}
	if len(deck.Slides) != 2 || deck.Slides[0].Notes != "first note" || deck.Slides[1].Notes != "second note" {
		t.Fatalf("slides = %#v", deck.Slides)
	}
}

func TestPresentationIDUsesTopLevelOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		headmatter string
		want       string
	}{
		{"top-level", "presentationID: target\n", "target"},
		{"conflicting nested ID", "presentationID: target\ngslidev:\n  presentationID: other\n", "target"},
		{"nested ID is ignored", "gslidev:\n  presentationID: old\n", ""},
		{"no ID", "title: Demo\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deck, err := Parse([]byte("---\n" + tc.headmatter + "---\n# Slide\n"))
			if err != nil {
				t.Fatal(err)
			}
			if deck.PresentationID != tc.want {
				t.Fatalf("presentation ID = %q, want %q", deck.PresentationID, tc.want)
			}
		})
	}
}

func TestSplitSlidesIgnoresFencedSeparators(t *testing.T) {
	parts := splitSlides("# One\n\n```md\n---\n```\n\n---\n# Two")
	if len(parts) != 2 {
		t.Fatalf("got %d parts", len(parts))
	}
}

func TestExtractNotesOnlyAtEnd(t *testing.T) {
	got := extractNotes("<!-- not a note -->\n\ntext\n\n<!-- note -->")
	if got != "note" {
		t.Fatalf("got %q", got)
	}
}
