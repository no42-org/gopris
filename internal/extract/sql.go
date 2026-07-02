// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package extract

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/opennms/gopris/internal/record"

	// modernc.org/sqlite is a pure-Go SQLite driver, registered as "sqlite". It
	// makes the SQL extractor usable out of the box (examples, tests). Other
	// databases work by importing their database/sql driver and setting
	// source.driver accordingly.
	_ "modernc.org/sqlite"
)

// SQLExtractor runs a query against a database/sql connection and maps each
// result row to one record whose fields are the selected columns. The *sql.DB
// is a long-lived connection pool opened once (see NewSQLExtractor) and reused
// across extractions, so refreshes do not reconnect on every poll.
type SQLExtractor struct {
	DB    *sql.DB
	Query string
}

// NewSQLExtractor opens the connection pool for the driver/DSN. sql.Open is
// lazy (no connection is made until first use), so this is cheap; the pool is
// then reused for the life of the process.
func NewSQLExtractor(driver, dsn, query string) (*SQLExtractor, error) {
	if driver == "" {
		return nil, fmt.Errorf("sql: driver is required")
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("sql: open: %w", err)
	}
	return &SQLExtractor{DB: db, Query: query}, nil
}

func (e *SQLExtractor) Extract(ctx context.Context) ([]record.Record, error) {
	rows, err := e.DB.QueryContext(ctx, e.Query)
	if err != nil {
		return nil, fmt.Errorf("sql: query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sql: columns: %w", err)
	}

	var out []record.Record
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("sql: scan: %w", err)
		}
		fields := make(map[string]string, len(cols))
		for i, name := range cols {
			fields[name] = cellToString(cells[i])
		}
		out = append(out, record.Record{Fields: fields})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sql: rows: %w", err)
	}
	return out, nil
}

// cellToString coerces a scanned column value to a string. NULL becomes "".
func cellToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}
