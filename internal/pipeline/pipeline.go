// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Package pipeline wires a requisition's configuration into the single
// extract -> transform -> serialize path used by every source, built-in or
// external.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/opennms/gopris/internal/config"
	"github.com/opennms/gopris/internal/extract"
	"github.com/opennms/gopris/internal/record"
	"github.com/opennms/gopris/internal/requisition"
	"github.com/opennms/gopris/internal/transform"
)

// Pipeline renders one configured requisition to XML on demand.
type Pipeline struct {
	cfg         *config.Requisition
	extractor   record.Extractor
	transformer *transform.Transformer
}

// New builds a Pipeline from a requisition config.
func New(cfg *config.Requisition) (*Pipeline, error) {
	ex, err := extract.Build(cfg.Source, cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("requisition %q: %w", cfg.Name, err)
	}
	tr, err := transform.New(cfg.Transform, cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("requisition %q: %w", cfg.Name, err)
	}
	return &Pipeline{cfg: cfg, extractor: ex, transformer: tr}, nil
}

// Render runs the full pipeline and returns validated, serialized XML.
func (p *Pipeline) Render(ctx context.Context) ([]byte, error) {
	records, err := p.extractor.Extract(ctx)
	if err != nil {
		return nil, fmt.Errorf("requisition %q: extract: %w", p.cfg.Name, err)
	}

	fs := p.cfg.ForeignSource
	if fs == "" {
		fs = p.cfg.Name
	}
	req := &requisition.Requisition{ForeignSource: fs}
	seen := make(map[string]struct{}, len(records))
	skipped := 0
	for i, rec := range records {
		node, err := p.transformer.Node(rec)
		if err != nil {
			// A transform/script error is systemic (a broken script or config
			// affects every record), so fail loudly rather than silently
			// dropping records.
			return nil, fmt.Errorf("requisition %q: transform record %d: %w", p.cfg.Name, i, err)
		}
		// A single malformed record must not void the whole requisition: skip
		// invalid nodes and duplicate identities, keeping every good node.
		if err := requisition.ValidateNode(&node); err != nil {
			slog.Warn("skipping invalid record", "requisition", p.cfg.Name, "record", i, "error", err)
			skipped++
			continue
		}
		if _, dup := seen[node.ForeignID]; dup {
			slog.Warn("skipping duplicate foreign-id", "requisition", p.cfg.Name, "record", i, "foreign-id", node.ForeignID)
			skipped++
			continue
		}
		seen[node.ForeignID] = struct{}{}
		req.Nodes = append(req.Nodes, node)
	}
	if skipped > 0 {
		slog.Warn("requisition rendered with skipped records", "requisition", p.cfg.Name, "kept", len(req.Nodes), "skipped", skipped)
	}
	// Final safety net: every node was validated and deduped above, so this
	// only guards requisition-level invariants (e.g. foreign-source).
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("requisition %q: invalid output: %w", p.cfg.Name, err)
	}
	return req.ToXML()
}
