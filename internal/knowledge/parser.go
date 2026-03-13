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

func ParsePDF(path string) (*Document, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return nil, err
	}
	_, _ = buf.ReadFrom(b)

	return &Document{
		Source:  path,
		Content: buf.String(),
		Type:    "pdf",
	}, nil
}

func ParseMarkdown(path string) (*Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &Document{
		Source:  path,
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
