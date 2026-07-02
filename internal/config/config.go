// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Package config defines the per-requisition configuration format and loads it
// from disk. Each requisition lives in its own directory containing a
// requisition.yaml file (plus any source data and an optional transform script).
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultTTL is applied when a requisition sets no (positive) ttl, so caching
// is on by default rather than a live extraction on every poll.
const DefaultTTL = 5 * time.Minute

// Requisition is the parsed configuration for a single named requisition.
type Requisition struct {
	// Name is the directory name; it is the path segment in the HTTP route and
	// is filled in by Load (not read from YAML).
	Name string `yaml:"-"`
	// Dir is the absolute path to the requisition's config directory; filled in
	// by Load. It is the working directory for exec sources and the base for
	// relative file paths and transform scripts.
	Dir string `yaml:"-"`

	ForeignSource string        `yaml:"foreign-source"`
	TTL           time.Duration `yaml:"ttl"`
	Source        Source        `yaml:"source"`
	Transform     Transform     `yaml:"transform"`
}

// Source describes how to extract records. Type selects the extractor; the
// remaining fields are the union of options across the built-in extractors and
// the external subprocess contract.
type Source struct {
	Type string `yaml:"type"` // csv | sql | xls | exec

	// csv, xls
	File string `yaml:"file"`
	// xls
	Sheet     string `yaml:"sheet"`
	HeaderRow int    `yaml:"header-row"`
	// sql
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
	Query  string `yaml:"query"`
	// exec
	Command []string          `yaml:"command"`
	Timeout time.Duration     `yaml:"timeout"`
	Params  map[string]string `yaml:"params"`
}

// Transform declares how records map onto the requisition model.
type Transform struct {
	// ForeignID is a ${field} template. If empty (or it interpolates to empty),
	// a deterministic hash of the record's fields is used instead.
	ForeignID           string `yaml:"foreign-id"`
	NodeLabel           string `yaml:"node-label"`
	Location            string `yaml:"location"`
	City                string `yaml:"city"`
	Building            string `yaml:"building"`
	ParentForeignSource string `yaml:"parent-foreign-source"`
	ParentForeignID     string `yaml:"parent-foreign-id"`
	ParentNodeLabel     string `yaml:"parent-node-label"`

	Interfaces []Interface `yaml:"interfaces"`
	Categories []string    `yaml:"categories"`
	Assets     []Asset     `yaml:"assets"`
	MetaData   []MetaData  `yaml:"metadata"`

	// Script is an optional Starlark file (relative to the requisition dir) that
	// post-processes each node.
	Script string `yaml:"script"`
}

// Interface is a declarative interface mapping. String fields are ${field}
// templates.
type Interface struct {
	IPAddr      string   `yaml:"ip-addr"`
	Descr       string   `yaml:"descr"`
	SnmpPrimary string   `yaml:"snmp-primary"`
	Status      int      `yaml:"status"`
	Managed     *bool    `yaml:"managed"`
	Services    []string `yaml:"services"`
}

// Asset is a declarative asset mapping (name is literal, value is a template).
type Asset struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// MetaData is a declarative meta-data mapping.
type MetaData struct {
	Context string `yaml:"context"`
	Key     string `yaml:"key"`
	Value   string `yaml:"value"`
}

// Load reads and parses a single requisition.yaml from the given directory,
// applying secret interpolation to source credential fields.
func Load(dir string) (*Requisition, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(abs, "requisition.yaml"))
	if err != nil {
		return nil, err
	}
	var r Requisition
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("parse requisition.yaml in %s: %w", dir, err)
	}
	r.Name = filepath.Base(abs)
	r.Dir = abs
	if r.TTL <= 0 {
		// An unset (or non-positive) ttl would otherwise disable caching and
		// re-run a live extraction on every Provisiond poll; apply a sane
		// default. Set a small ttl explicitly for near-real-time requisitions.
		r.TTL = DefaultTTL
	}

	// Interpolate secrets into credential-bearing fields.
	r.Source.DSN, err = Interpolate(r.Source.DSN)
	if err != nil {
		return nil, fmt.Errorf("%s: source.dsn: %w", r.Name, err)
	}
	for k, v := range r.Source.Params {
		if r.Source.Params[k], err = Interpolate(v); err != nil {
			return nil, fmt.Errorf("%s: source.params.%s: %w", r.Name, k, err)
		}
	}
	return &r, nil
}

// LoadAll loads every requisition directory (one containing requisition.yaml)
// directly under root, keyed by name.
func LoadAll(root string) (map[string]*Requisition, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*Requisition)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "requisition.yaml")); err != nil {
			continue // not a requisition directory
		}
		r, err := Load(dir)
		if err != nil {
			return nil, err
		}
		out[r.Name] = r
	}
	return out, nil
}
