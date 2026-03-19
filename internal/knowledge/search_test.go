package knowledge

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

type mockAI struct{}

func (m *mockAI) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	// Simple deterministic "embedding" for testing
	if text == "apple" {
		return []float32{1.0, 0.0, 0.0}, nil
	}
	if text == "orange" {
		return []float32{1.0, 0.0, 0.1}, nil
	}
	if text == "car" {
		return []float32{0.0, 1.0, 0.0}, nil
	}
	return []float32{0.0, 0.0, 1.0}, nil
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	c := []float32{0.0, 1.0, 0.0}

	simAB := cosineSimilarity(a, b)
	if simAB < 0.99 {
		t.Errorf("Expected ~1.0, got %f", simAB)
	}

	simAC := cosineSimilarity(a, c)
	if simAC != 0.0 {
		t.Errorf("Expected 0.0, got %f", simAC)
	}
}

func TestSearchKnowledge(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	ai := &mockAI{}
	tableName := "test_knowledge"

	// Force multiple chunks by making maxChunkSize smaller in internal logic? 
	// Or just use one chunk and verify it returns.
	doc := &Document{
		Source:  "test.md",
		Content: "the quick brown fox jumps over the lazy dog and likes to eat an apple",
	}

	err = IndexDocument(ctx, db, ai, doc, tableName)
	if err != nil {
		t.Fatal(err)
	}

	// Search for something semantic
	results, err := SearchKnowledge(ctx, db, ai, tableName, "orange", 1)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Results: %v", results)

	if len(results) == 0 {
		t.Fatal("Expected results, got none")
	}

	if !strings.Contains(results[0], "apple") {
		t.Errorf("Expected result containing 'apple', got '%s'", results[0])
	}
}
