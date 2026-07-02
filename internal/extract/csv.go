// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package extract

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"

	"github.com/opennms/gopris/internal/record"
)

// CSVExtractor reads a CSV file whose first row is a header. Each subsequent row
// becomes one record with fields named by the header columns.
type CSVExtractor struct {
	Path string
}

func (e *CSVExtractor) Extract(ctx context.Context) ([]record.Record, error) {
	f, err := os.Open(e.Path)
	if err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate ragged rows
	header, err := r.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("csv: read header: %w", err)
	}

	var out []record.Record
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv: read row: %w", err)
		}
		fields := make(map[string]string, len(header))
		for i, name := range header {
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
