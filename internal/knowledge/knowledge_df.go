package knowledge

import (
	"database/sql"
	"fmt"
)

// LoadAsDataframe loads all content from a knowledge table into a standard
// dataframe-compatible format (cols, rows) so the Orchestrator can use it
// as a regular dataset for cross-querying with SQL data.
func LoadAsDataframe(db *sql.DB, tableName string) ([]string, [][]string, error) {
	query := fmt.Sprintf("SELECT id, source, content FROM %s", tableName)
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load knowledge table '%s': %v", tableName, err)
	}
	defer rows.Close()

	cols := []string{"id", "source", "content"}
	var data [][]string

	for rows.Next() {
		var id int
		var source, content string
		if err := rows.Scan(&id, &source, &content); err != nil {
			continue
		}
		data = append(data, []string{fmt.Sprintf("%d", id), source, content})
	}

	if len(data) == 0 {
		return nil, nil, fmt.Errorf("no knowledge found in table '%s'", tableName)
	}

	return cols, data, nil
}
