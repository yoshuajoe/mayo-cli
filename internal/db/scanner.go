package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	IsNullable bool   `json:"is_nullable"`
}

type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
	PK      []string `json:"primary_keys"`
}

type Schema struct {
	Tables []Table `json:"tables"`
}

func Connect(driver, dsn string) (*sql.DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ConnectWithContext(ctx, driver, dsn)
}

func ConnectWithContext(ctx context.Context, driver, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func ScanSchema(ctx context.Context, db *sql.DB, driver string) (*Schema, error) {
	if driver == "postgres" {
		return scanPostgres(ctx, db)
	} else if driver == "mysql" {
		return scanMySQL(ctx, db)
	} else if driver == "sqlite" {
		return scanSQLite(ctx, db)
	}
	return nil, fmt.Errorf("unsupported driver: %s", driver)
}

func scanPostgres(ctx context.Context, db *sql.DB) (*Schema, error) {
	query := `
		SELECT table_name, column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()


	schema := &Schema{}
	tableMap := make(map[string]*Table)

	for rows.Next() {
		var tableName, colName, dataType, isNullable string
		if err := rows.Scan(&tableName, &colName, &dataType, &isNullable); err != nil {
			return nil, err
		}

		if _, ok := tableMap[tableName]; !ok {
			table := &Table{Name: tableName}
			schema.Tables = append(schema.Tables, *table)
			tableMap[tableName] = &schema.Tables[len(schema.Tables)-1]
		}

		tableMap[tableName].Columns = append(tableMap[tableName].Columns, Column{
			Name:       colName,
			Type:       dataType,
			IsNullable: isNullable == "YES",
		})
	}

	return schema, nil
}

func scanMySQL(ctx context.Context, db *sql.DB) (*Schema, error) {
	// Similar logic for MySQL
	query := `
		SELECT table_name, column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		ORDER BY table_name, ordinal_position;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schema := &Schema{}
	tableMap := make(map[string]*Table)

	for rows.Next() {
		var tableName, colName, dataType, isNullable string
		if err := rows.Scan(&tableName, &colName, &dataType, &isNullable); err != nil {
			return nil, err
		}

		if _, ok := tableMap[tableName]; !ok {
			table := &Table{Name: tableName}
			schema.Tables = append(schema.Tables, *table)
			tableMap[tableName] = &schema.Tables[len(schema.Tables)-1]
		}

		tableMap[tableName].Columns = append(tableMap[tableName].Columns, Column{
			Name:       colName,
			Type:       dataType,
			IsNullable: isNullable == "YES",
		})
	}

	return schema, nil
}

func scanSQLite(ctx context.Context, db *sql.DB) (*Schema, error) {
	query := `
		SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schema := &Schema{}
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}

		table := Table{Name: tableName}
		
		// Get columns for this table
		colRows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			continue
		}
		for colRows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt interface{}
			if err := colRows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				continue
			}
			table.Columns = append(table.Columns, Column{
				Name:       name,
				Type:       ctype,
				IsNullable: notnull == 0,
			})
		}
		colRows.Close()
		schema.Tables = append(schema.Tables, table)
	}

	return schema, nil
}
