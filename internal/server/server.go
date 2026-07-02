// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Package server exposes requisitions over HTTP for OpenNMS Provisiond to pull.
// It renders on demand, caches the rendered XML for each requisition's TTL, and
// serves the last successfully rendered XML (persisted to disk) when a fresh
// extraction fails.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/opennms/gopris/internal/config"
	"github.com/opennms/gopris/internal/pipeline"
	"golang.org/x/sync/singleflight"
)

// DefaultRenderTimeout bounds a single extract->transform->serialize pass so a
// hung source (e.g. an unresponsive database) cannot block indefinitely.
const DefaultRenderTimeout = 2 * time.Minute

// failureBackoff is how long, after a failed render, requests are served from
// last-good without re-running the (still likely failing) extraction.
const failureBackoff = 15 * time.Second

// Clock returns the current time; overridable in tests.
type Clock func() time.Time

// requisition bundles a pipeline with its cache state.
type requisition struct {
	cfg      *config.Requisition
	pipe     *pipeline.Pipeline
	mu       sync.Mutex
	cached   []byte
	rendered time.Time
	failedAt time.Time
}

// Server serves configured requisitions.
type Server struct {
	reqs          map[string]*requisition
	stateDir      string
	now           Clock
	renderTimeout time.Duration
	group         singleflight.Group
}

// New builds a Server from loaded requisition configs. stateDir is where
// last-good renderings are persisted (created if absent).
func New(cfgs map[string]*config.Requisition, stateDir string) (*Server, error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("state dir: %w", err)
	}
	s := &Server{reqs: make(map[string]*requisition), stateDir: stateDir, now: time.Now, renderTimeout: DefaultRenderTimeout}
	for name, cfg := range cfgs {
		p, err := pipeline.New(cfg)
		if err != nil {
			return nil, err
		}
		s.reqs[name] = &requisition{cfg: cfg, pipe: p}
	}
	return s, nil
}

// Handler returns the HTTP handler serving GET /requisitions/{name}.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /requisitions/{name}", s.handleRequisition)
	return mux
}

func (s *Server) handleRequisition(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	req, ok := s.reqs[name]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown requisition %q", name), http.StatusNotFound)
		return
	}
	xml, err := s.render(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write(xml)
}

// render returns the requisition XML, applying the cache / refresh /
// serve-last-good policy. The per-requisition lock is held only for the brief
// cache read/write; the extraction itself runs outside the lock and is
// deduplicated across concurrent requests via singleflight, so a slow source
// no longer serializes every poller or blocks cache hits.
func (s *Server) render(req *requisition) ([]byte, error) {
	name := req.cfg.Name
	now := s.now()

	req.mu.Lock()
	cached, rendered, failedAt := req.cached, req.rendered, req.failedAt
	req.mu.Unlock()

	// Serve from the in-memory cache while within TTL.
	if cached != nil && req.cfg.TTL > 0 && now.Sub(rendered) < req.cfg.TTL {
		return cached, nil
	}
	// Recent failure: serve last-good without re-hammering a failing source.
	if !failedAt.IsZero() && now.Sub(failedAt) < failureBackoff {
		if last := s.readLastGood(name); last != nil {
			return last, nil
		}
	}

	// Deduplicate concurrent renders of the same requisition into one pass. The
	// extraction uses a fresh, time-bounded context (not the HTTP request's), so
	// a client disconnect neither cancels a shared render nor is misreported as
	// a source failure.
	v, err, _ := s.group.Do(name, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), s.renderTimeout)
		defer cancel()
		return req.pipe.Render(ctx)
	})
	if err != nil {
		req.mu.Lock()
		req.failedAt = s.now()
		req.mu.Unlock()
		if last := s.readLastGood(name); last != nil {
			slog.Warn("serving last-good rendering after failure", "requisition", name, "error", err)
			return last, nil
		}
		slog.Error("render failed and no last-good available", "requisition", name, "error", err)
		return nil, fmt.Errorf("requisition %q unavailable: %w", name, err)
	}

	xml := v.([]byte)
	req.mu.Lock()
	req.cached = xml
	req.rendered = s.now()
	req.failedAt = time.Time{}
	req.mu.Unlock()
	s.writeLastGood(name, xml)
	return xml, nil
}

func (s *Server) lastGoodPath(name string) string {
	return filepath.Join(s.stateDir, name+".xml")
}

func (s *Server) readLastGood(name string) []byte {
	data, err := os.ReadFile(s.lastGoodPath(name))
	if err != nil {
		return nil
	}
	return data
}

func (s *Server) writeLastGood(name string, xml []byte) {
	tmp := s.lastGoodPath(name) + ".tmp"
	if err := os.WriteFile(tmp, xml, 0o644); err != nil {
		slog.Warn("persist last-good failed", "requisition", name, "error", err)
		return
	}
	if err := os.Rename(tmp, s.lastGoodPath(name)); err != nil {
		slog.Warn("persist last-good failed", "requisition", name, "error", err)
	}
}
