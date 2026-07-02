# Copyright 2026 The OpenNMS Group, Inc.
# SPDX-License-Identifier: MIT
# Created by Ronny Trommer <ronny@opennms.com>

BINARY := gopris
PKG    := github.com/opennms/gopris

.PHONY: build test vet verify run tidy clean

build: ## Build the gopris binary
	go build -o bin/$(BINARY) ./cmd/gopris

test: ## Run the test suite
	go test ./...

vet: ## Run go vet
	go vet ./...

verify: vet test build ## Vet, test, and build (used by CI)

run: build ## Build and run against ./examples
	./bin/$(BINARY) --config examples/requisitions --state ./_state

tidy: ## Sync go.mod/go.sum
	go mod tidy

clean:
	rm -rf bin _state
