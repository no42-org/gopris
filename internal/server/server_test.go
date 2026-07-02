// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opennms/gopris/internal/config"
	"github.com/xuri/excelize/v2"

	_ "modernc.org/sqlite"
)

// writeReq creates a requisition directory with a requisition.yaml plus any
// extra files, then returns the parent config root.
func writeReq(t *testing.T, name, yaml string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requisition.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	for fn, body := range files {
		p := filepath.Join(dir, fn)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func serverFor(t *testing.T, root string) *Server {
	t.Helper()
	cfgs, err := config.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(cfgs, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func get(t *testing.T, srv *Server, name string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/requisitions/"+name, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func validateXSD(t *testing.T, xml []byte) {
	t.Helper()
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		return // best-effort; requisition package covers XSD validation unconditionally
	}
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "r.xml")
	if err := os.WriteFile(xmlPath, xml, 0o644); err != nil {
		t.Fatal(err)
	}
	xsd := filepath.Join("..", "requisition", "testdata", "model-import.xsd")
	if out, err := exec.Command(xmllint, "--noout", "--schema", xsd, xmlPath).CombinedOutput(); err != nil {
		t.Fatalf("served XML failed XSD validation: %v\n%s\n%s", err, out, xml)
	}
}

// Task 8.1: config -> extract -> transform -> serve valid XML for CSV, SQL, XLS,
// and subprocess sources.
func TestEndToEndAllSources(t *testing.T) {
	transform := `
transform:
  foreign-id: "${id}"
  node-label: "${name}"
  interfaces:
    - ip-addr: "${ip}"
      snmp-primary: "P"
      status: 1
`
	t.Run("csv", func(t *testing.T) {
		root := writeReq(t, "hosts", "foreign-source: hosts\nttl: 1h\nsource:\n  type: csv\n  file: nodes.csv\n"+transform,
			map[string]string{"nodes.csv": "id,name,ip\n1,router-1,192.0.2.1\n2,switch-1,192.0.2.2\n"})
		rec := get(t, serverFor(t, root), "hosts")
		assertServed(t, rec, "router-1")
	})

	t.Run("xls", func(t *testing.T) {
		root := writeReq(t, "hosts", "foreign-source: hosts\nttl: 1h\nsource:\n  type: xls\n  file: nodes.xlsx\n  header-row: 1\n"+transform, nil)
		f := excelize.NewFile()
		sheet := f.GetSheetName(0)
		_ = f.SetSheetRow(sheet, "A1", &[]any{"id", "name", "ip"})
		_ = f.SetSheetRow(sheet, "A2", &[]any{"1", "router-1", "192.0.2.1"})
		if err := f.SaveAs(filepath.Join(root, "hosts", "nodes.xlsx")); err != nil {
			t.Fatal(err)
		}
		f.Close()
		rec := get(t, serverFor(t, root), "hosts")
		assertServed(t, rec, "router-1")
	})

	t.Run("sql", func(t *testing.T) {
		root := t.TempDir()
		dbPath := filepath.Join(root, "hosts.db")
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE nodes(id TEXT,name TEXT,ip TEXT);INSERT INTO nodes VALUES('1','router-1','192.0.2.1');`); err != nil {
			t.Fatal(err)
		}
		db.Close()
		yaml := "foreign-source: hosts\nttl: 1h\nsource:\n  type: sql\n  driver: sqlite\n  dsn: " + dbPath + "\n  query: \"SELECT id,name,ip FROM nodes\"\n" + transform
		dir := filepath.Join(root, "hosts")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "requisition.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		rec := get(t, serverFor(t, root), "hosts")
		assertServed(t, rec, "router-1")
	})

	t.Run("subprocess", func(t *testing.T) {
		script := "#!/bin/sh\ncat >/dev/null\necho '{\"id\":\"1\",\"name\":\"router-1\",\"ip\":\"192.0.2.1\"}'\n"
		root := writeReq(t, "hosts", "foreign-source: hosts\nttl: 1h\nsource:\n  type: exec\n  command: [\"./source.sh\"]\n"+transform,
			map[string]string{"source.sh": script})
		rec := get(t, serverFor(t, root), "hosts")
		assertServed(t, rec, "router-1")
	})
}

func assertServed(t *testing.T, rec *httptest.ResponseRecorder, wantLabel string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()
	if !strings.Contains(string(body), `node-label="`+wantLabel+`"`) {
		t.Fatalf("served XML missing node-label %q:\n%s", wantLabel, body)
	}
	validateXSD(t, body)
}

func TestUnknownRequisition404(t *testing.T) {
	root := writeReq(t, "hosts", "foreign-source: hosts\nsource:\n  type: csv\n  file: n.csv\n", map[string]string{"n.csv": "id\n1\n"})
	rec := get(t, serverFor(t, root), "does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Task 6.3: cache serves within TTL even if the source disappears.
func TestCacheServesWithinTTL(t *testing.T) {
	root := writeReq(t, "hosts", "foreign-source: hosts\nttl: 1h\nsource:\n  type: csv\n  file: nodes.csv\n"+
		"transform:\n  foreign-id: \"${id}\"\n  node-label: \"${name}\"\n",
		map[string]string{"nodes.csv": "id,name\n1,router-1\n"})
	srv := serverFor(t, root)

	if rec := get(t, srv, "hosts"); rec.Code != 200 {
		t.Fatalf("first GET status %d", rec.Code)
	}
	// Remove the source; a cache hit should still succeed within TTL.
	os.Remove(filepath.Join(root, "hosts", "nodes.csv"))
	rec := get(t, srv, "hosts")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "router-1") {
		t.Fatalf("expected cached 200 with router-1, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Tasks 6.4 & 6.5: serve last-good on failure, and last-good survives a restart.
func TestServeLastGoodAcrossRestart(t *testing.T) {
	root := writeReq(t, "hosts", "foreign-source: hosts\nttl: 1ms\nsource:\n  type: csv\n  file: nodes.csv\n"+
		"transform:\n  foreign-id: \"${id}\"\n  node-label: \"${name}\"\n",
		map[string]string{"nodes.csv": "id,name\n1,router-1\n"})
	stateDir := filepath.Join(t.TempDir(), "state")

	cfgs, err := config.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	srv1, err := New(cfgs, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if rec := get(t, srv1, "hosts"); rec.Code != 200 { // renders + persists last-good
		t.Fatalf("initial render status %d", rec.Code)
	}

	// Break the source, let the 1ms cache expire, then force a re-render.
	os.Remove(filepath.Join(root, "hosts", "nodes.csv"))
	time.Sleep(5 * time.Millisecond)
	if rec := get(t, srv1, "hosts"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "router-1") {
		t.Fatalf("expected last-good served, got %d: %s", rec.Code, rec.Body.String())
	}

	// Simulate a restart: brand-new server (empty in-memory cache), same state dir.
	cfgs2, _ := config.LoadAll(root)
	srv2, err := New(cfgs2, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	rec := get(t, srv2, "hosts")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "router-1") {
		t.Fatalf("last-good did not survive restart, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Finding #5: concurrent requests for one requisition must not serialize behind
// a slow extraction. A ~300ms source hit by 8 concurrent requests should finish
// far under the ~2.4s a serialized implementation would take (singleflight
// coalesces them into one render, and the lock is not held across I/O).
func TestConcurrentRequestsNotSerialized(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	script := "#!/bin/sh\nsleep 0.3\necho '{\"id\":\"1\",\"name\":\"router-1\",\"ip\":\"192.0.2.1\"}'\n"
	root := writeReq(t, "hosts", "foreign-source: hosts\nttl: 1h\nsource:\n  type: exec\n  command: [\"./s.sh\"]\n"+
		"transform:\n  foreign-id: \"${id}\"\n  node-label: \"${name}\"\n",
		map[string]string{"s.sh": script})
	srv := serverFor(t, root)

	const n = 8
	var wg sync.WaitGroup
	codes := make([]int, n)
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = get(t, srv, "hosts").Code
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	for i, c := range codes {
		if c != 200 {
			t.Fatalf("request %d got status %d", i, c)
		}
	}
	if elapsed > 2*time.Second {
		t.Fatalf("requests serialized: %d concurrent took %v (serialized would be ~%v)", n, elapsed, n*300*time.Millisecond)
	}
}

// A failing source with no prior success yields 5xx.
func TestFailureWithoutLastGoodIs5xx(t *testing.T) {
	root := writeReq(t, "hosts", "foreign-source: hosts\nttl: 0s\nsource:\n  type: csv\n  file: missing.csv\n"+
		"transform:\n  node-label: \"${name}\"\n", nil)
	srv := serverFor(t, root)
	if rec := get(t, srv, "hosts"); rec.Code < 500 {
		t.Fatalf("expected 5xx, got %d", rec.Code)
	}
}
