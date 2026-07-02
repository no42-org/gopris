// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package extract

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/opennms/gopris/internal/record"
	"github.com/xuri/excelize/v2"
)

// byName sorts records by their "name" field for stable assertions.
func byName(recs []record.Record) {
	sort.Slice(recs, func(i, j int) bool { return recs[i].Get("name") < recs[j].Get("name") })
}

func TestCSVExtractor(t *testing.T) {
	e := &CSVExtractor{Path: filepath.Join("testdata", "nodes.csv")}
	recs, err := e.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	byName(recs)
	if recs[0].Get("name") != "router-1" || recs[0].Get("ip") != "192.0.2.1" || recs[0].Get("vendor") != "Acme" {
		t.Errorf("unexpected first record: %v", recs[0].Fields)
	}
}

func TestSQLExtractor(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE nodes(id INTEGER, name TEXT, ip TEXT);
		INSERT INTO nodes VALUES (1,'router-1','192.0.2.1'),(2,'switch-1','192.0.2.2');`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	e, err := NewSQLExtractor("sqlite", dsn, "SELECT id, name, ip FROM nodes ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	recs, err := e.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].Get("name") != "router-1" || recs[0].Get("id") != "1" || recs[1].Get("ip") != "192.0.2.2" {
		t.Errorf("unexpected records: %v %v", recs[0].Fields, recs[1].Fields)
	}
}

func TestXLSExtractor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.xlsx")
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"name", "ip"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"router-1", "192.0.2.1"})
	_ = f.SetSheetRow(sheet, "A3", &[]any{"switch-1", "192.0.2.2"})
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	f.Close()

	e := &XLSExtractor{Path: path, HeaderRow: 1}
	recs, err := e.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	byName(recs)
	if recs[0].Get("name") != "router-1" || recs[0].Get("ip") != "192.0.2.1" {
		t.Errorf("unexpected first record: %v", recs[0].Fields)
	}
}

// Task 4.5: fixture-based integration test using a sample script that emits NDJSON.
func TestSubprocessExtractor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "source.sh")
	// Reads JSON params from stdin, echoes a log to stderr, emits two NDJSON records.
	body := `#!/bin/sh
cat > /dev/null            # consume params on stdin
echo "extracting" >&2
echo '{"name":"router-1","ip":"192.0.2.1","up":true,"ports":48}'
echo '{"name":"switch-1","ip":"192.0.2.2"}'
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &SubprocessExtractor{Command: []string{script}, Params: map[string]string{"k": "v"}, Dir: dir}
	recs, err := e.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	byName(recs)
	// JSON scalar coercion: bool -> "true", integer float64 -> "48".
	if recs[0].Get("up") != "true" || recs[0].Get("ports") != "48" {
		t.Errorf("scalar coercion wrong: %v", recs[0].Fields)
	}
}

// Finding #2: large 64-bit integers in NDJSON must not lose precision.
func TestSubprocessPreservesLargeInts(t *testing.T) {
	recs, err := parseNDJSON(strings.NewReader(`{"id":9007199254740993,"asn":4200000000}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if recs[0].Get("id") != "9007199254740993" {
		t.Errorf("large int corrupted: got %q, want 9007199254740993", recs[0].Get("id"))
	}
	if recs[0].Get("asn") != "4200000000" {
		t.Errorf("int corrupted: got %q", recs[0].Get("asn"))
	}
}

// Finding #6: null / {} / blank lines must not become empty records.
func TestSubprocessSkipsEmptyRecords(t *testing.T) {
	recs, err := parseNDJSON(strings.NewReader("null\n{}\n\n{\"name\":\"router-1\"}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Get("name") != "router-1" {
		t.Fatalf("want only the non-empty record, got %d: %+v", len(recs), recs)
	}
}

// Finding #4: interior blank spreadsheet rows must be skipped.
func TestXLSSkipsBlankRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.xlsx")
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"name", "ip"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"router-1", "192.0.2.1"})
	// row 3 left blank
	_ = f.SetSheetRow(sheet, "A4", &[]any{"switch-1", "192.0.2.2"})
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	f.Close()

	recs, err := (&XLSExtractor{Path: path, HeaderRow: 1}).Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("blank row not skipped: want 2 records, got %d: %+v", len(recs), recs)
	}
}

// Finding #9: a bare command naming a script in the config dir resolves to it.
func TestBuildResolvesBareCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "source.sh"), []byte("#!/bin/sh\necho '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveCommand(dir, []string{"source.sh", "--flag"})
	want := filepath.Join(dir, "source.sh")
	if got[0] != want {
		t.Errorf("bare command not resolved: got %q, want %q", got[0], want)
	}
	if got[1] != "--flag" {
		t.Errorf("args mangled: %v", got)
	}
	// A PATH command (not present in dir) is left untouched.
	if pc := resolveCommand(dir, []string{"python3"}); pc[0] != "python3" {
		t.Errorf("PATH command should be left alone, got %q", pc[0])
	}
}

func TestSubprocessExtractorFailsOnNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &SubprocessExtractor{Command: []string{script}, Dir: dir}
	if _, err := e.Extract(context.Background()); err == nil {
		t.Fatal("expected error on non-zero exit, got nil")
	}
}
