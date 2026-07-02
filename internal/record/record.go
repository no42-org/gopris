// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Package record defines the canonical record — the normalized shape every
// extractor emits — and the Extractor interface. Built-in and external sources
// alike produce []Record, so the transform layer is written once and is
// identical regardless of source.
package record

import "context"

// Record is a single extracted entity: a set of named string fields. Values are
// strings because CSV, spreadsheets, and the NDJSON subprocess contract are all
// inherently textual; type coercion (to int, IP, bool, ...) is the transform
// layer's responsibility.
type Record struct {
	Fields map[string]string
}

// Get returns the value of a field, or "" if absent.
func (r Record) Get(key string) string { return r.Fields[key] }

// Extractor fetches records from a data source. Implementations must respect
// ctx cancellation (used to enforce per-source timeouts).
type Extractor interface {
	// Extract returns all records for one extraction pass.
	Extract(ctx context.Context) ([]Record, error)
}
