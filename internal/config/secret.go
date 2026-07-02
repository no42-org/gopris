// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// secretRef matches ${env:NAME} and ${file:/path} references.
var secretRef = regexp.MustCompile(`\$\{(env|file):([^}]+)\}`)

// Interpolate replaces secret references in s with their resolved values so that
// credentials need not be stored as plaintext in shared configuration:
//
//	${env:NAME}   -> value of environment variable NAME
//	${file:/path} -> trimmed contents of the file at /path
//
// A reference to a missing environment variable or unreadable file is an error.
func Interpolate(s string) (string, error) {
	var firstErr error
	out := secretRef.ReplaceAllStringFunc(s, func(match string) string {
		m := secretRef.FindStringSubmatch(match)
		kind, arg := m[1], m[2]
		switch kind {
		case "env":
			v, ok := os.LookupEnv(arg)
			if !ok {
				if firstErr == nil {
					firstErr = fmt.Errorf("environment variable %q is not set", arg)
				}
				return match
			}
			return v
		case "file":
			data, err := os.ReadFile(arg)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("read secret file %q: %w", arg, err)
				}
				return match
			}
			return strings.TrimSpace(string(data))
		}
		return match
	})
	return out, firstErr
}
