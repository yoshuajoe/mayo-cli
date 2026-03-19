package knowledge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

type VectorClient interface {
	GetEmbedding(ctx context.Context, text string) ([]float32, error)
}

// PrivacyFilter masks PII before content is sent externally or stored.
type PrivacyFilter interface {
	Apply(text string) string
}

func IndexDocument(ctx context.Context, db *sql.DB, ai VectorClient, pf PrivacyFilter, doc *Document, tableName string) error {
	// 1. Create table with embedding support (using BLOB for vector storage)
	// We keep FTS5 for hybrid search potential later
	createStmt := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT,
			content TEXT,
			embedding BLOB
		)
	`, tableName)

	_, err := db.Exec(createStmt)
	if err != nil {
		return fmt.Errorf("failed to create knowledge table: %v", err)
	}

	// Also create FTS5 table as a sidecar for keyword search
	ftsTableName := tableName + "_fts"
	_, _ = db.Exec(fmt.Sprintf("CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(content, content='%s', content_rowid='id')", ftsTableName, tableName))

	// 2. Chunking (Improved: smaller chunks for better granularity)
	chunks := strings.Split(doc.Content, "\n")
	var refinedChunks []string
	var currentChunk strings.Builder

	const maxChunkSize = 500 // Smaller chunks = more granular search results
	for _, line := range chunks {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if currentChunk.Len()+len(trimmed) > maxChunkSize {
			refinedChunks = append(refinedChunks, currentChunk.String())
			currentChunk.Reset()
		}
		currentChunk.WriteString(trimmed + " ")
	}
	if currentChunk.Len() > 0 {
		refinedChunks = append(refinedChunks, currentChunk.String())
	}

	// Filter out very small chunks
	var validChunks []string
	for _, chunk := range refinedChunks {
		if len(strings.TrimSpace(chunk)) >= 20 {
			validChunks = append(validChunks, chunk)
		}
	}

	fmt.Printf("  📄 Source: %s (%d chars, %d pages)\n", doc.Source, len(doc.Content), strings.Count(doc.Content, "\n"))
	fmt.Printf("  🧩 Split into %d chunks (max %d chars each)\n", len(validChunks), maxChunkSize)

	if pf != nil {
		fmt.Println("  🛡️  PII Masking: ACTIVE")
	} else {
		fmt.Println("  🛡️  PII Masking: OFF")
	}

	if ai != nil {
		fmt.Println("  🧠 Embedding: ACTIVE (generating vectors...)")
	} else {
		fmt.Println("  ⚠️  Embedding: SKIPPED (no AI client — keyword search only)")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 3. Clear existing source data if we are re-indexing same file
	_, _ = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE source = ?", tableName), doc.Source)

	stmt, err := tx.Prepare(fmt.Sprintf("INSERT INTO %s (source, content, embedding) VALUES (?, ?, ?)", tableName))
	if err != nil {
		return err
	}
	defer stmt.Close()

	maskedCount := 0
	embeddedCount := 0

	for i, chunk := range validChunks {
		// Apply PII masking before sending externally or storing
		safeChunk := chunk
		if pf != nil {
			masked := pf.Apply(chunk)
			if masked != chunk {
				maskedCount++
			}
			safeChunk = masked
		}

		var embBLOB []byte
		if ai != nil {
			emb, err := ai.GetEmbedding(ctx, safeChunk)
			if err == nil {
				embBLOB = serializeEmbedding(emb)
				embeddedCount++
			}
		}

		_, err = stmt.Exec(doc.Source, safeChunk, embBLOB)
		if err != nil {
			return err
		}

		// Progress indicator every 10 chunks
		if (i+1)%10 == 0 || i == len(validChunks)-1 {
			fmt.Printf("  ⏳ Processed %d/%d chunks\r", i+1, len(validChunks))
		}
	}
	fmt.Println() // New line after progress

	if maskedCount > 0 {
		fmt.Printf("  🛡️  PII masked in %d chunks\n", maskedCount)
	}
	if embeddedCount > 0 {
		fmt.Printf("  🧠 Generated %d embeddings\n", embeddedCount)
	}

	// Refresh FTS index
	_, _ = tx.Exec(fmt.Sprintf("INSERT INTO %s(%s) VALUES('rebuild')", ftsTableName, ftsTableName))

	return tx.Commit()
}

func SearchKnowledge(ctx context.Context, db *sql.DB, ai VectorClient, tableName string, query string, limit int) ([]string, error) {
	if ai != nil {
		queryEmb, err := ai.GetEmbedding(ctx, query)
		if err == nil {
			// SEMANTIC SEARCH (Vector RAG)
			rows, err := db.Query(fmt.Sprintf("SELECT content, embedding FROM %s", tableName))
			if err == nil {
				defer rows.Close()
				type scoredResult struct {
					content string
					score   float64
				}
				var results []scoredResult

				for rows.Next() {
					var content string
					var embBLOB []byte
					if err := rows.Scan(&content, &embBLOB); err == nil && len(embBLOB) > 0 {
						emb := deserializeEmbedding(embBLOB)
						score := cosineSimilarity(queryEmb, emb)
						results = append(results, scoredResult{content, score})
					}
				}

				// Sort by score descending
				for i := 0; i < len(results); i++ {
					for j := i + 1; j < len(results); j++ {
						if results[j].score > results[i].score {
							results[i], results[j] = results[j], results[i]
						}
					}
				}

				var topResults []string
				for i := 0; i < len(results) && i < limit; i++ {
					topResults = append(topResults, results[i].content)
				}

				if len(topResults) > 0 {
					return topResults, nil
				}
			}
		}
	}

	// KEYWORD SEARCH (FTS5 Fallback)
	ftsTableName := tableName + "_fts"
	searchQuery := fmt.Sprintf("SELECT content FROM %s WHERE %s MATCH ? ORDER BY rank LIMIT ?", ftsTableName, ftsTableName)
	rows, err := db.Query(searchQuery, query, limit)
	if err != nil {
		// Final fallback to LIKE
		searchQuery = fmt.Sprintf("SELECT content FROM %s WHERE content LIKE ? LIMIT ?", tableName)
		rows, err = db.Query(searchQuery, "%"+query+"%", limit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err == nil {
			results = append(results, content)
		}
	}
	return results, nil
}

func serializeEmbedding(emb []float32) []byte {
	buf := new(bytes.Buffer)
	for _, f := range emb {
		binary.Write(buf, binary.LittleEndian, f)
	}
	return buf.Bytes()
}

func deserializeEmbedding(data []byte) []float32 {
	res := make([]float32, len(data)/4)
	for i := 0; i < len(res); i++ {
		res[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4 : (i+1)*4]))
	}
	return res
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
