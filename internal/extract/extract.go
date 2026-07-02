// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Package extract provides the built-in data-source extractors (CSV, SQL, XLS)
// and the external subprocess extractor. Every extractor normalizes its source
// to []record.Record so the transform layer is source-agnostic.
package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opennms/gopris/internal/config"
	"github.com/opennms/gopris/internal/record"
)

// Build constructs the extractor selected by src.Type, validating that the
// options required by that type are present. baseDir is the requisition's
// config directory: relative file paths resolve against it and it is the
// working directory for exec sources.
func Build(src config.Source, baseDir string) (record.Extractor, error) {
	switch src.Type {
	case "csv":
		if src.File == "" {
			return nil, fmt.Errorf("source.type csv requires 'file'")
		}
		return &CSVExtractor{Path: resolve(baseDir, src.File)}, nil
	case "sql":
		if src.Query == "" {
			return nil, fmt.Errorf("source.type sql requires 'query'")
		}
		return NewSQLExtractor(src.Driver, src.DSN, src.Query)
	case "xls":
		if src.File == "" {
			return nil, fmt.Errorf("source.type xls requires 'file'")
		}
		return &XLSExtractor{Path: resolve(baseDir, src.File), Sheet: src.Sheet, HeaderRow: src.HeaderRow}, nil
	case "exec":
		if len(src.Command) == 0 {
			return nil, fmt.Errorf("source.type exec requires 'command'")
		}
		return &SubprocessExtractor{Command: resolveCommand(baseDir, src.Command), Params: src.Params, Timeout: src.Timeout, Dir: baseDir}, nil
	case "":
		return nil, fmt.Errorf("source.type is required")
	default:
		return nil, fmt.Errorf("unknown source.type %q", src.Type)
	}
}

// resolve returns path unchanged if absolute, otherwise joins it onto baseDir.
func resolve(baseDir, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// resolveCommand fixes a common footgun: os/exec resolves a bare command name
// (no path separator) via $PATH, not against cmd.Dir. So a script sitting in
// the requisition directory referenced as just "source.sh" would not be found.
// If such a file exists in baseDir, rewrite argv[0] to its absolute path;
// otherwise leave it for a normal $PATH lookup (e.g. "python3").
func resolveCommand(baseDir string, command []string) []string {
	arg0 := command[0]
	if filepath.IsAbs(arg0) || strings.ContainsRune(arg0, filepath.Separator) {
		return command
	}
	cand := filepath.Join(baseDir, arg0)
	if info, err := os.Stat(cand); err == nil && !info.IsDir() {
		out := make([]string, len(command))
		copy(out, command)
		out[0] = cand
		return out
	}
	return command
}
