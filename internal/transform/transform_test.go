// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package transform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opennms/gopris/internal/config"
	"github.com/opennms/gopris/internal/record"
	"github.com/opennms/gopris/internal/requisition"
)

func rec() record.Record {
	return record.Record{Fields: map[string]string{
		"id": "n1", "name": "router-1", "ip": "192.0.2.1", "vendor": "Acme",
	}}
}

func TestDeclarativeMapping(t *testing.T) {
	cfg := config.Transform{
		ForeignID: "${id}",
		NodeLabel: "${name}",
		Interfaces: []config.Interface{{
			IPAddr: "${ip}", SnmpPrimary: "P", Status: 1, Services: []string{"ICMP", "SNMP"},
		}},
		Categories: []string{"Servers"},
		Assets:     []config.Asset{{Name: "vendor", Value: "${vendor}"}},
	}
	tr, err := New(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	n, err := tr.Node(rec())
	if err != nil {
		t.Fatal(err)
	}
	if n.ForeignID != "n1" || n.NodeLabel != "router-1" {
		t.Errorf("node identity wrong: %+v", n)
	}
	if len(n.Interfaces) != 1 || n.Interfaces[0].IPAddr != "192.0.2.1" || n.Interfaces[0].SnmpPrimary != requisition.SnmpPrimaryP {
		t.Errorf("interface mapping wrong: %+v", n.Interfaces)
	}
	if len(n.Interfaces[0].Services) != 2 {
		t.Errorf("want 2 services, got %d", len(n.Interfaces[0].Services))
	}
	if len(n.Assets) != 1 || n.Assets[0].Value != "Acme" {
		t.Errorf("asset mapping wrong: %+v", n.Assets)
	}
}

// Task 5.2: deterministic foreign-id. Same record -> same id across runs; the
// hash fallback is used when no template is configured.
func TestForeignIDStability(t *testing.T) {
	tr, _ := New(config.Transform{}, "") // no foreign-id template -> hash fallback
	a, _ := tr.Node(rec())
	b, _ := tr.Node(rec())
	if a.ForeignID == "" {
		t.Fatal("expected a hashed foreign-id, got empty")
	}
	if a.ForeignID != b.ForeignID {
		t.Errorf("foreign-id not stable: %q != %q", a.ForeignID, b.ForeignID)
	}
	// A different record must produce a different id.
	other := record.Record{Fields: map[string]string{"id": "n2"}}
	c, _ := tr.Node(other)
	if c.ForeignID == a.ForeignID {
		t.Error("different records produced the same foreign-id")
	}
}

// Task 5.3 / 5.5: a Starlark script producing/adjusting a node.
func TestStarlarkScript(t *testing.T) {
	dir := t.TempDir()
	script := `
def transform(record, node):
    node["node_label"] = record["name"].upper()
    node["categories"] = node["categories"] + ["Scripted"]
    return node
`
	if err := os.WriteFile(filepath.Join(dir, "t.star"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Transform{
		ForeignID:  "${id}",
		NodeLabel:  "${name}",
		Categories: []string{"Base"},
		Script:     "t.star",
	}
	tr, err := New(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	n, err := tr.Node(rec())
	if err != nil {
		t.Fatal(err)
	}
	if n.NodeLabel != "ROUTER-1" {
		t.Errorf("script did not transform node-label: %q", n.NodeLabel)
	}
	if len(n.Categories) != 2 || n.Categories[1].Name != "Scripted" {
		t.Errorf("script did not append category: %+v", n.Categories)
	}
}

// Finding #7: a script setting a wrong-typed field must error, not silently
// coerce to a zero value that produces a wrong node.
func TestStarlarkRejectsWrongType(t *testing.T) {
	dir := t.TempDir()
	// status set to a string instead of an int.
	script := "def transform(record, node):\n" +
		"    node[\"interfaces\"] = [{\"ip_addr\": record[\"ip\"], \"status\": \"3\"}]\n" +
		"    return node\n"
	if err := os.WriteFile(filepath.Join(dir, "t.star"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := New(config.Transform{ForeignID: "${id}", NodeLabel: "${name}", Script: "t.star"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Node(rec())
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("expected a type error mentioning status, got: %v", err)
	}
}

// Finding #14: ${env:NAME} in a transform value resolves instead of blanking.
func TestInterpolateEnvRef(t *testing.T) {
	t.Setenv("PRIS_DC", "eu-central")
	tr, _ := New(config.Transform{
		ForeignID: "${id}", NodeLabel: "${name}",
		Assets: []config.Asset{{Name: "datacenter", Value: "${env:PRIS_DC}"}},
	}, "")
	n, err := tr.Node(rec())
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Assets) != 1 || n.Assets[0].Value != "eu-central" {
		t.Errorf("env ref not resolved in transform value: %+v", n.Assets)
	}
}

// Finding #15: metadata whose value interpolates empty is omitted.
func TestMetadataEmptyValueSkipped(t *testing.T) {
	tr, _ := New(config.Transform{
		ForeignID: "${id}", NodeLabel: "${name}",
		MetaData: []config.MetaData{
			{Context: "gopris", Key: "rack", Value: "${missing}"}, // empty -> skipped
			{Context: "gopris", Key: "source", Value: "csv"},       // kept
		},
	}, "")
	n, err := tr.Node(rec())
	if err != nil {
		t.Fatal(err)
	}
	if len(n.MetaData) != 1 || n.MetaData[0].Key != "source" {
		t.Errorf("empty-value metadata not skipped: %+v", n.MetaData)
	}
}

func TestStarlarkScriptError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.star"),
		[]byte("def transform(record, node):\n    fail(\"nope\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := New(config.Transform{Script: "bad.star"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Node(rec()); err == nil {
		t.Fatal("expected script error, got nil")
	}
}
