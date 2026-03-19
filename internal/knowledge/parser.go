package knowledge

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

type Document struct {
	Source  string
	Content string
	Type    string // pdf, md
}

// ParsePDF extracts text from a PDF using GetPlainText (which handles
// character glue and spacing better than raw Content().Text).
// It also normalizes "spaced-out" text (e.g. "H e l l o") which some
// PDFs produce due to character-level positioning.
func ParsePDF(path string) (*Document, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Prefer GetPlainText: it handles character joining better
	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err == nil {
		_, _ = buf.ReadFrom(b)
	}

	// If GetPlainText produced nothing, try page-by-page extraction
	if buf.Len() == 0 {
		for i := 1; i <= r.NumPage(); i++ {
			p := r.Page(i)
			content := p.Content()
			var pageText strings.Builder
			for _, text := range content.Text {
				pageText.WriteString(text.S)
				pageText.WriteString(" ")
			}
			if pageText.Len() > 0 {
				buf.WriteString(pageText.String())
				buf.WriteString("\n")
			}
		}
	}

	// Normalize: fix spaced-out text patterns (e.g. "K O N E K S I" -> "KONEKSI")
	text := normalizeSpacedText(buf.String())

	return &Document{
		Source:  filepath.Base(path),
		Content: text,
		Type:    "pdf",
	}, nil
}

// normalizeSpacedText detects and fixes text where every character is
// separated by spaces (common PDF extraction artifact).
// e.g. "R E Q U E S T" -> "REQUEST"
func normalizeSpacedText(text string) string {
	lines := strings.Split(text, "\n")
	var result strings.Builder

	for _, line := range lines {
		normalized := normalizeSpacedLine(line)
		result.WriteString(normalized)
		result.WriteString("\n")
	}

	return result.String()
}

// normalizeSpacedLine checks if a line appears to be "spaced out"
// (majority of single chars separated by spaces) and collapses it.
func normalizeSpacedLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return line
	}

	// Split by spaces
	parts := strings.Fields(trimmed)
	if len(parts) < 3 {
		return line
	}

	// Count how many "parts" are single characters
	singleCount := 0
	for _, p := range parts {
		if len([]rune(p)) == 1 {
			singleCount++
		}
	}

	// If more than 60% of parts are single characters, this is spaced-out text
	ratio := float64(singleCount) / float64(len(parts))
	if ratio > 0.6 {
		// Collapse: join all parts without space for single chars,
		// but preserve space when a multi-char word appears
		var sb strings.Builder
		for i, p := range parts {
			sb.WriteString(p)
			// Add space after multi-char words, or between groups
			if len([]rune(p)) > 1 && i < len(parts)-1 {
				sb.WriteString(" ")
			}
		}
		return sb.String()
	}

	return line
}

func ParseMarkdown(path string) (*Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &Document{
		Source:  filepath.Base(path),
		Content: string(content),
		Type:    "md",
	}, nil
}

func LoadKnowledge(path string) (*Document, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".pdf" {
		return ParsePDF(path)
	} else if ext == ".md" || ext == ".markdown" || ext == ".txt" {
		return ParseMarkdown(path)
	}
	return nil, fmt.Errorf("unsupported knowledge format: %s", ext)
}
