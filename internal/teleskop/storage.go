package teleskop

import (
	"database/sql"
	"fmt"
	"mayo-cli/internal/config"
	"mayo-cli/internal/db"
	"path/filepath"
)

func GetScraperDBPath(id string) string {
	return filepath.Join(config.GetConfigDir(), "data", fmt.Sprintf("scraper_%s.db", id))
}

func ConnectScraperDB(id string) (*sql.DB, error) {
	path := GetScraperDBPath(id)
	return db.Connect("sqlite", path)
}

func InitializeScraperTable(id string) error {
	dbConn, err := ConnectScraperDB(id)
	if err != nil {
		return err
	}
	defer dbConn.Close()

	// Generic table for scraped data
	query := `
		CREATE TABLE IF NOT EXISTS scraped_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keyword TEXT,
			content TEXT,
			url TEXT,
			source TEXT,
			scraped_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`
	_, err = dbConn.Exec(query)
	return err
}

func GetHead(id string, n int) ([]string, [][]string, error) {
	dbConn, err := ConnectScraperDB(id)
	if err != nil {
		return nil, nil, err
	}
	defer dbConn.Close()

	query := fmt.Sprintf("SELECT * FROM scraped_data ORDER BY id ASC LIMIT %d", n)
	return executeQuery(dbConn, query)
}

func GetTail(id string, n int) ([]string, [][]string, error) {
	dbConn, err := ConnectScraperDB(id)
	if err != nil {
		return nil, nil, err
	}
	defer dbConn.Close()

	query := fmt.Sprintf("SELECT * FROM scraped_data ORDER BY id DESC LIMIT %d", n)
	return executeQuery(dbConn, query)
}

func GetSummary(id string) (string, error) {
	dbConn, err := ConnectScraperDB(id)
	if err != nil {
		return "", err
	}
	defer dbConn.Close()

	var count int
	err = dbConn.QueryRow("SELECT COUNT(*) FROM scraped_data").Scan(&count)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Scraper ID: %s\nTotal Records: %d", id, count), nil
}

func executeQuery(dbConn *sql.DB, query string) ([]string, [][]string, error) {
	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var data [][]string
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptr := make([]interface{}, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if err := rows.Scan(ptr...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			if v == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		data = append(data, row)
	}
	return cols, data, nil
}
