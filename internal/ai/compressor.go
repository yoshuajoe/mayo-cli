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
			cStr := col.Name
			if col.Type != "" {
				cStr += ":" + col.Type
			}
			if col.IsCategorical && len(col.Categories) > 0 {
				cStr += "[cats:" + strings.Join(col.Categories, ",") + "]"
			} else if len(col.SampleValues) > 0 {
				cStr += "[samples:" + strings.Join(col.SampleValues, ",") + "]"
			}
			cols = append(cols, cStr)
		}
		b.WriteString(strings.Join(cols, "|"))
		if t.Description != "" {
			b.WriteString(") -- " + t.Description + "\n")
		} else {
			b.WriteString(")\n")
		}
	}
	return b.String()
}

// CompressContext removes redundant whitespace and truncates to save tokens
func (c *Compressor) CompressContext(text string) string {
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	var curated []string
	var currentBlock []string
	var isPriority bool

	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "User:") || strings.HasPrefix(trimmed, "Mayo SQL:") || strings.HasPrefix(trimmed, "Analyst Insight:") {
			isPriority = true
			if len(currentBlock) > 0 {
				curated = append(curated, strings.Join(currentBlock, " "))
				currentBlock = nil
			}
			currentBlock = append(currentBlock, trimmed)
		} else if strings.HasPrefix(trimmed, "Mayo:") {
			isPriority = false
			if len(currentBlock) > 0 {
				curated = append(curated, strings.Join(currentBlock, " "))
				currentBlock = nil
			}
			// Truncate plain Mayo responses to keep them from dominating context
			if len(trimmed) > 150 {
				trimmed = trimmed[:147] + "..."
			}
			currentBlock = append(currentBlock, trimmed)
		} else {
			// Continuation of previous block
			if isPriority || len(currentBlock) < 3 { // Keep non-priority blocks short
				currentBlock = append(currentBlock, trimmed)
			}
		}
	}
	if len(currentBlock) > 0 {
		curated = append(curated, strings.Join(currentBlock, " "))
	}

	// Join with newlines for clarity but keep total tokens low
	result := strings.Join(curated, "\n")

	// Strict truncation for cost control (keep only the most recent ~8000 chars)
	maxLen := 8000
	if len(result) > maxLen {
		result = "... " + result[len(result)-maxLen:]
	}

	return result
}

func MarshalCompressed(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}
