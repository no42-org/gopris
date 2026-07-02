// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Covers the data-extraction spec scenario: an environment variable referenced
// in config is resolved before the extractor uses it.
func TestLoadInterpolatesEnvSecret(t *testing.T) {
	t.Setenv("PRIS_DB_PASS", "s3cret")
	dir := t.TempDir()
	yaml := "foreign-source: hosts\n" +
		"source:\n  type: sql\n  driver: sqlite\n  dsn: \"user:${env:PRIS_DB_PASS}@/db\"\n  query: SELECT 1\n"
	if err := os.WriteFile(filepath.Join(dir, "requisition.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.DSN != "user:s3cret@/db" {
		t.Errorf("dsn not interpolated: %q", r.Source.DSN)
	}
	if r.Name != filepath.Base(dir) {
		t.Errorf("name = %q, want %q", r.Name, filepath.Base(dir))
	}
}

func TestInterpolateFileSecret(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "pw")
	if err := os.WriteFile(secret, []byte("filepass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Interpolate("pw=${file:" + secret + "}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "pw=filepass" {
		t.Errorf("got %q", got)
	}
}

func TestInterpolateMissingEnvErrors(t *testing.T) {
	if _, err := Interpolate("${env:DEFINITELY_NOT_SET_PRIS_VAR}"); err == nil {
		t.Fatal("expected error for missing env var")
	}
}
