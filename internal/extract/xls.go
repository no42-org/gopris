// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package extract

import (
	"context"
	"fmt"

	"github.com/opennms/gopris/internal/record"
	"github.com/xuri/excelize/v2"
)

// XLSExtractor reads a spreadsheet (.xls/.xlsx). It uses the configured sheet
// (default: the first sheet) and header row (1-based, default 1). Each data row
// below the header becomes one record with fields named by the header cells.
type XLSExtractor struct {
	Path      string
	Sheet     string
	HeaderRow int
}

func (e *XLSExtractor) Extract(ctx context.Context) ([]record.Record, error) {
	f, err := excelize.OpenFile(e.Path)
	if err != nil {
		return nil, fmt.Errorf("xls: %w", err)
	}
	defer f.Close()

	sheet := e.Sheet
	if sheet == "" {
		sheet = f.GetSheetName(0)
		if sheet == "" {
			return nil, fmt.Errorf("xls: no sheets in %s", e.Path)
		}
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("xls: read sheet %q: %w", sheet, err)
	}

	headerIdx := e.HeaderRow - 1
	if e.HeaderRow == 0 {
		headerIdx = 0
	}
	if headerIdx < 0 || headerIdx >= len(rows) {
		return nil, fmt.Errorf("xls: header-row %d out of range (sheet %q has %d rows)", e.HeaderRow, sheet, len(rows))
	}
	header := rows[headerIdx]

	var out []record.Record
	for _, row := range rows[headerIdx+1:] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if isBlankRow(row) {
			// excelize returns interior blank rows as empty slices; skip them
			// rather than emitting an all-empty record.
			continue
		}
		fields := make(map[string]string, len(header))
		for i, name := range header {
			if name == "" {
				continue
			}
			if i < len(row) {
				fields[name] = row[i]
			} else {
				fields[name] = ""
			}
		}
		out = append(out, record.Record{Fields: fields})
	}
	return out, nil
}

// isBlankRow reports whether every cell in the row is empty.
func isBlankRow(row []string) bool {
	for _, c := range row {
		if c != "" {
			return false
		}
	}
	return true
}
