// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opennms/gopris/internal/config"
)

func loadPipeline(t *testing.T, yaml string, files map[string]string) *Pipeline {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "requisition.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	for fn, body := range files {
		if err := os.WriteFile(filepath.Join(dir, fn), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Findings #1, #3: a single invalid record (and a duplicate foreign-id) must be
// skipped, not fail the whole requisition.
func TestRenderSkipsInvalidAndDuplicate(t *testing.T) {
	yaml := "foreign-source: hosts\nsource:\n  type: csv\n  file: nodes.csv\n" +
		"transform:\n  foreign-id: \"${id}\"\n  node-label: \"${name}\"\n  interfaces:\n    - ip-addr: \"${ip}\"\n"
	csv := "id,name,ip\n" +
		"1,router-1,192.0.2.1\n" + // valid
		"2,,192.0.2.2\n" + // invalid: empty node-label -> skipped
		"1,router-dup,192.0.2.3\n" // duplicate foreign-id 1 -> skipped
	p := loadPipeline(t, yaml, map[string]string{"nodes.csv": csv})

	xml, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("render should succeed despite bad rows: %v", err)
	}
	s := string(xml)
	if n := strings.Count(s, "<node "); n != 1 {
		t.Fatalf("want exactly 1 node, got %d\n%s", n, s)
	}
	if !strings.Contains(s, `node-label="router-1"`) {
		t.Errorf("valid node missing: %s", s)
	}
	if strings.Contains(s, "router-dup") {
		t.Errorf("duplicate foreign-id node should have been skipped: %s", s)
	}
}

// Finding #1 refinement: a record missing only the IP yields a node with no
// interface (still valid and served), not an invalid empty interface.
func TestRenderDropsEmptyInterface(t *testing.T) {
	yaml := "foreign-source: hosts\nsource:\n  type: csv\n  file: nodes.csv\n" +
		"transform:\n  foreign-id: \"${id}\"\n  node-label: \"${name}\"\n  interfaces:\n    - ip-addr: \"${ip}\"\n"
	csv := "id,name,ip\n1,router-1,\n"
	p := loadPipeline(t, yaml, map[string]string{"nodes.csv": csv})

	xml, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(xml)
	if !strings.Contains(s, `node-label="router-1"`) {
		t.Errorf("node should still be served: %s", s)
	}
	if strings.Contains(s, "<interface") {
		t.Errorf("empty-IP interface should be dropped: %s", s)
	}
}
