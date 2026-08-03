package slidev

import "testing"

func TestParseFrontmatterAndNotes(t *testing.T) {
	deck, err := Parse([]byte(`---
title: Demo
gslidev:
  presentationID: pres
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
