// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package extract

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/opennms/gopris/internal/record"
)

// DefaultSubprocessTimeout applies when a source sets no timeout.
const DefaultSubprocessTimeout = 60 * time.Second

// SubprocessExtractor runs an external program as a data source, implementing
// the gopris subprocess contract (see docs/subprocess-contract.md):
//
//   - stdin  <- JSON object of parameters (source.params),
//   - stdout -> NDJSON, one JSON object per line, each a record's fields,
//   - stderr -> log lines, surfaced in the gopris log,
//   - exit 0 = success; any non-zero exit (or timeout) = extraction failed,
//   - cwd    = the requisition's config directory.
//
// This is the language-agnostic extension point: a source can be written in any
// language that can read stdin and write stdout.
type SubprocessExtractor struct {
	Command []string
	Params  map[string]string
	Timeout time.Duration
	Dir     string
}

func (e *SubprocessExtractor) Extract(ctx context.Context) ([]record.Record, error) {
	if len(e.Command) == 0 {
		return nil, fmt.Errorf("exec: command is required")
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = DefaultSubprocessTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	params, err := json.Marshal(paramsOrEmpty(e.Params))
	if err != nil {
		return nil, fmt.Errorf("exec: marshal params: %w", err)
	}

	cmd := exec.CommandContext(ctx, e.Command[0], e.Command[1:]...)
	cmd.Dir = e.Dir
	cmd.Stdin = bytes.NewReader(params)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	// Surface the plugin's stderr as logs regardless of outcome.
	if s := strings.TrimRight(stderr.String(), "\n"); s != "" {
		for _, line := range strings.Split(s, "\n") {
			slog.Info("subprocess source", "command", e.Command[0], "stderr", line)
		}
	}
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("exec: %q timed out after %s", e.Command[0], timeout)
		}
		return nil, fmt.Errorf("exec: %q failed: %w", e.Command[0], runErr)
	}

	return parseNDJSON(&stdout)
}

func paramsOrEmpty(p map[string]string) map[string]string {
	if p == nil {
		return map[string]string{}
	}
	return p
}

// parseNDJSON reads newline-delimited JSON objects, each becoming one record.
// Numbers are decoded via json.Number so large 64-bit integers keep their exact
// value (a plain map[string]any would coerce them to float64 and lose
// precision). Blank lines and empty objects (including a bare null) are skipped
// rather than emitted as field-less records.
func parseNDJSON(r io.Reader) ([]record.Record, error) {
	var out []record.Record
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // allow long lines
	line := 0
	for sc.Scan() {
		line++
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			return nil, fmt.Errorf("exec: stdout line %d is not a JSON object: %w", line, err)
		}
		if len(obj) == 0 {
			// A `null` line decodes to a nil map, `{}` to an empty one; neither
			// is a usable record.
			continue
		}
		fields := make(map[string]string, len(obj))
		for k, v := range obj {
			fields[k] = anyToString(v)
		}
		out = append(out, record.Record{Fields: fields})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("exec: read stdout: %w", err)
	}
	return out, nil
}

// anyToString coerces a decoded JSON scalar to a string so records stay
// uniformly stringy. Numbers arrive as json.Number (see parseNDJSON), whose
// String() preserves the exact literal. Nested objects/arrays are re-encoded.
func anyToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}
