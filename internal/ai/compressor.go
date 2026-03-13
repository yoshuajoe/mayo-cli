package ai

import (
	"encoding/json"
	"mayo-cli/internal/db"
	"strings"
)

// Compressor optimizes prompts for token efficiency
type Compressor struct{}

// CompressSchema reduces the schema representation to essential parts
func (c *Compressor) CompressSchema(schema *db.Schema) string {
	if schema == nil {
		return "None"
	}

	// Simple compression: only table names and column names, omit types/nullable if too large
	// But let's start with a format that is more compact than standard JSON
	// Aggressive compression: table(c1|c2) without spaces
	var b strings.Builder
	for _, t := range schema.Tables {
		b.WriteString(t.Name + "(")
		var cols []string
		for _, col := range t.Columns {
			cols = append(cols, col.Name)
		}
		b.WriteString(strings.Join(cols, "|"))
		b.WriteString(")")
	}
	return b.String()
}

// CompressContext removes redundant whitespace and truncates to save tokens
func (c *Compressor) CompressContext(text string) string {
	if text == "" {
		return ""
	}

	// 1. Remove excessive whitespace and empty lines
	lines := strings.Split(text, "\n")
	var curated []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			// 2. Remove common prefixes that AI doesn't need repeatedly
			trimmed = strings.TrimPrefix(trimmed, "## ")
			curated = append(curated, trimmed)
		}
	}

	result := strings.Join(curated, " ") // Use space instead of newline to save tokens

	// 3. Strict truncation for cost control (keep only the most recent ~5000 chars)
	maxLen := 5000
	if len(result) > maxLen {
		result = "... " + result[len(result)-maxLen:]
	}

	return result
}

func MarshalCompressed(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}
