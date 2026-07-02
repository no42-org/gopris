// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Package transform maps canonical records onto the OpenNMS requisition model
// using a declarative field mapping, with an optional Starlark script as an
// escape hatch for logic the declarative form cannot express. Built-in and
// external sources flow through this single path identically.
package transform

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/opennms/gopris/internal/config"
	"github.com/opennms/gopris/internal/record"
	"github.com/opennms/gopris/internal/requisition"
)

// fieldRef matches ${field} references in template strings.
var fieldRef = regexp.MustCompile(`\$\{([^}]+)\}`)

// Transformer converts records into requisition nodes for one requisition.
type Transformer struct {
	cfg    config.Transform
	script *starlarkScript // nil if no script configured
}

// New builds a Transformer. If cfg.Script is set, scriptDir is the directory the
// (relative) script path is resolved against.
func New(cfg config.Transform, scriptDir string) (*Transformer, error) {
	t := &Transformer{cfg: cfg}
	if cfg.Script != "" {
		s, err := loadScript(scriptDir, cfg.Script)
		if err != nil {
			return nil, err
		}
		t.script = s
	}
	return t, nil
}

// Node converts a single record into a requisition node.
func (t *Transformer) Node(r record.Record) (requisition.Node, error) {
	n := t.declarative(r)
	if t.script != nil {
		out, err := t.script.run(r, n)
		if err != nil {
			return requisition.Node{}, err
		}
		n = out
	}
	return n, nil
}

// declarative applies the configured field mapping to build a node.
func (t *Transformer) declarative(r record.Record) requisition.Node {
	n := requisition.Node{
		ForeignID:           t.foreignID(r),
		NodeLabel:           interpolate(t.cfg.NodeLabel, r),
		Location:            interpolate(t.cfg.Location, r),
		City:                interpolate(t.cfg.City, r),
		Building:            interpolate(t.cfg.Building, r),
		ParentForeignSource: interpolate(t.cfg.ParentForeignSource, r),
		ParentForeignID:     interpolate(t.cfg.ParentForeignID, r),
		ParentNodeLabel:     interpolate(t.cfg.ParentNodeLabel, r),
	}
	for _, ic := range t.cfg.Interfaces {
		ip := interpolate(ic.IPAddr, r)
		if ip == "" {
			// A record that lacks the address field yields no interface (rather
			// than an invalid empty one); the node itself can still be valid.
			continue
		}
		iface := requisition.Interface{
			IPAddr:      ip,
			Descr:       interpolate(ic.Descr, r),
			SnmpPrimary: requisition.SnmpPrimary(interpolate(ic.SnmpPrimary, r)),
			Status:      ic.Status,
			Managed:     ic.Managed,
		}
		for _, svc := range ic.Services {
			if name := interpolate(svc, r); name != "" {
				iface.Services = append(iface.Services, requisition.MonitoredService{ServiceName: name})
			}
		}
		n.Interfaces = append(n.Interfaces, iface)
	}
	for _, c := range t.cfg.Categories {
		if name := interpolate(c, r); name != "" {
			n.Categories = append(n.Categories, requisition.Category{Name: name})
		}
	}
	for _, a := range t.cfg.Assets {
		if v := interpolate(a.Value, r); v != "" {
			n.Assets = append(n.Assets, requisition.Asset{Name: a.Name, Value: v})
		}
	}
	for _, m := range t.cfg.MetaData {
		if v := interpolate(m.Value, r); v != "" {
			n.MetaData = append(n.MetaData, requisition.MetaData{Context: m.Context, Key: m.Key, Value: v})
		}
	}
	return n
}

// foreignID resolves the node's foreign-id: the configured template if it
// interpolates to a non-empty value, otherwise a deterministic hash of the
// record's fields so the same record always yields the same identity.
func (t *Transformer) foreignID(r record.Record) string {
	if id := interpolate(t.cfg.ForeignID, r); id != "" {
		return id
	}
	return hashRecord(r)
}

// hashRecord produces a stable ID from all fields, independent of map order.
func hashRecord(r record.Record) string {
	keys := make([]string, 0, len(r.Fields))
	for k := range r.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha1.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%s\x00", k, r.Fields[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// interpolate replaces ${...} references in tmpl. A ${env:NAME} or ${file:PATH}
// reference is resolved as a secret (so operators can inject constants from the
// environment); any other reference is a record field. Unknown record fields
// interpolate to an empty string; a secret reference that fails to resolve is
// left literal so the failure is visible rather than silently blank.
func interpolate(tmpl string, r record.Record) string {
	if !strings.Contains(tmpl, "${") {
		return tmpl
	}
	return fieldRef.ReplaceAllStringFunc(tmpl, func(m string) string {
		key := fieldRef.FindStringSubmatch(m)[1]
		if strings.HasPrefix(key, "env:") || strings.HasPrefix(key, "file:") {
			if resolved, err := config.Interpolate(m); err == nil {
				return resolved
			}
			return m
		}
		return r.Fields[key]
	})
}
