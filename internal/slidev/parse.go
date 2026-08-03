package slidev

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type frontmatter struct {
	Title   string `yaml:"title"`
	GSlidev struct {
		PresentationID string `yaml:"presentationID"`
		FolderID       string `yaml:"folderID"`
	} `yaml:"gslidev"`
}

func ParseFile(path string) (*Deck, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

func Parse(b []byte) (*Deck, error) {
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var fm frontmatter
	body := text
	if strings.HasPrefix(body, "---\n") {
		end := strings.Index(body[4:], "\n---\n")
		if end < 0 {
			return nil, fmt.Errorf("unterminated frontmatter")
		}
		end += 4
		if err := yaml.Unmarshal([]byte(body[4:end]), &fm); err != nil {
			return nil, fmt.Errorf("invalid frontmatter: %w", err)
		}
		body = body[end+5:]
	}

	parts := splitSlides(body)
	if len(parts) == 0 || (len(parts) == 1 && strings.TrimSpace(parts[0]) == "") {
		return nil, fmt.Errorf("no slides found")
	}
	deck := &Deck{
		Title:          fm.Title,
		PresentationID: fm.GSlidev.PresentationID,
		FolderID:       fm.GSlidev.FolderID,
		Slides:         make([]Slide, len(parts)),
	}
	for i, part := range parts {
		deck.Slides[i].Notes = extractNotes(part)
	}
	return deck, nil
}

func splitSlides(body string) []string {
	var parts []string
	start := 0
	lineStart := 0
	inFence := false
	for lineStart <= len(body) {
		lineEnd := strings.IndexByte(body[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(body)
		} else {
			lineEnd += lineStart
		}
		line := strings.TrimSpace(body[lineStart:lineEnd])
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
		}
		if !inFence && line == "---" {
			parts = append(parts, body[start:lineStart])
			start = lineEnd
			if start < len(body) && body[start] == '\n' {
				start++
			}
		}
		if lineEnd == len(body) {
			break
		}
		lineStart = lineEnd + 1
	}
	return append(parts, body[start:])
}

func extractNotes(slide string) string {
	rest := strings.TrimSpace(slide)
	var notes []string
	for {
		if !strings.HasSuffix(rest, "-->") {
			break
		}
		end := len(rest) - len("-->")
		start := strings.LastIndex(rest[:end], "<!--")
		if start < 0 {
			break
		}
		note := strings.TrimSpace(rest[start+len("<!--") : end])
		notes = append([]string{note}, notes...)
		rest = strings.TrimSpace(rest[:start])
	}
	return strings.Join(notes, "\n\n")
}
