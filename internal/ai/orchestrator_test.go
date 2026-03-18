package ai

import (
	"testing"
)

func TestExtractSQL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Here is the query:\n```sql\nSELECT * FROM users;\n```",
			expected: "SELECT * FROM users;",
		},
		{
			input:    "```sql SELECT 1 ```",
			expected: "SELECT 1",
		},
		{
			input:    "No SQL here",
			expected: "",
		},
		{
			input:    "Multiple blocks:\n```sql\nSELECT 1;\n```\nAnd then:\n```sql\nSELECT 2;\n```",
			expected: "SELECT 1;", // Should return first match
		},
	}

	for _, tt := range tests {
		got := extractSQL(tt.input)
		if got != tt.expected {
			t.Errorf("extractSQL(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	input := "Results in JSON:\n```json\n[{\"id\": 1}]\n```"
	expected := "[{\"id\": 1}]"
	got := extractJSON(input)
	if got != expected {
		t.Errorf("extractJSON() = %q; want %q", got, expected)
	}
}

func TestBasicSecurityCheck(t *testing.T) {
	tests := []struct {
		query   string
		isSafe  bool
	}{
		{"SELECT * FROM users", true},
		{"SELECT 1", true},
		{"DROP TABLE users", false},
		{"INSERT INTO users VALUES (1)", false},
		{"UPDATE users SET name='foo'", false},
		{"DELETE FROM users", false},
		{"CREATE TABLE foo (id int)", false},
		{"alter table users add column bar int", false},
		{"GRANT ALL ON SCHEMA public TO guest", false},
	}

	for _, tt := range tests {
		err := basicSecurityCheck(tt.query)
		if tt.isSafe && err != nil {
			t.Errorf("basicSecurityCheck(%q) returned error for safe query: %v", tt.query, err)
		}
		if !tt.isSafe && err == nil {
			t.Errorf("basicSecurityCheck(%q) passed for unsafe query", tt.query)
		}
	}
}
