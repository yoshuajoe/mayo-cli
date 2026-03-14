package changelog

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed data/*.md
var changelogFiles embed.FS

// GetChangelogs returns all changelog contents sorted by version descending
func GetChangelogs() ([]string, error) {
	entries, err := fs.ReadDir(changelogFiles, "data")
	if err != nil {
		return nil, err
	}

	var filenames []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			filenames = append(filenames, entry.Name())
		}
	}

	// Sort by version descending (v1.2.0.md > v1.1.0.md)
	sort.Slice(filenames, func(i, j int) bool {
		return filenames[i] > filenames[j]
	})

	var results []string
	for _, fname := range filenames {
		content, err := fs.ReadFile(changelogFiles, "data/"+fname)
		if err == nil {
			results = append(results, string(content))
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no changelogs found")
	}

	return results, nil
}
