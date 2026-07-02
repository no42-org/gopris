// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Command gopris is a passive HTTP daemon that renders OpenNMS provisioning
// requisitions from external data sources. OpenNMS Provisiond pulls each
// requisition as XML from GET /requisitions/<name>.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/opennms/gopris/internal/config"
	"github.com/opennms/gopris/internal/server"
)

func main() {
	var (
		configDir = flag.String("config", "config/requisitions", "directory of per-requisition config directories")
		stateDir  = flag.String("state", "state", "directory for persisted last-good renderings")
		addr      = flag.String("addr", ":8080", "HTTP listen address")
	)
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfgs, err := config.LoadAll(*configDir)
	if err != nil {
		slog.Error("load requisitions", "dir", *configDir, "error", err)
		os.Exit(1)
	}
	if len(cfgs) == 0 {
		slog.Warn("no requisitions found", "dir", *configDir)
	}

	srv, err := server.New(cfgs, *stateDir)
	if err != nil {
		slog.Error("init server", "error", err)
		os.Exit(1)
	}

	names := make([]string, 0, len(cfgs))
	for n := range cfgs {
		names = append(names, n)
	}
	slog.Info("gopris starting", "addr", *addr, "requisitions", names)

	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		slog.Error("http server", "error", err)
		os.Exit(1)
	}
}
