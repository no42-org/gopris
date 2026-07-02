## Why

The OpenNMS Provisioning Integration Server (PRIS) is an aging Java/Spring tool that extracts inventory from external sources and transforms it into OpenNMS requisitions. We want a modern, MIT-licensed rewrite named **gopris**, built in Go, that ships as a single static binary and — critically — lets people add their own data sources in *any* language without recompiling or knowing Go. Go is chosen deliberately: the audience is system and network operators, and the tools they already run and extend (Prometheus, Grafana, Telegraf, Kubernetes, Terraform) are overwhelmingly Go, giving the project the broadest possible contributor reach and database-driver coverage. This change bootstraps the daemon end to end: a working extract → transform → serve pipeline for the common sources.

## What Changes

- New `gopris` daemon: a passive HTTP server that renders an OpenNMS requisition as XML at `GET /requisitions/<name>`, which OpenNMS Provisiond pulls on its own schedule (classic PRIS delivery model — no push).
- **Extract** layer with built-in extractors for **SQL**, **CSV**, and **XLS**, plus an **external subprocess contract**: any executable in any language emits canonical records as NDJSON on stdout, so third parties add sources without touching Go.
- A **canonical `Record`** internal representation — every extractor (built-in or external) emits the same normalized shape, so the transform layer is identical regardless of source.
- **Transform** layer with a **declarative field-mapping** config for the common case plus a **Starlark script escape hatch** for cleanup/manipulation logic.
- A **typed, full-fidelity OpenNMS requisition model** (nodes, interfaces with `snmp-primary`/`status`/`managed`, monitored-services, categories, assets, `meta-data` contexts, and parent topology) that serializes to schema-valid XML.
- **Refresh model**: on-demand extraction with a short TTL cache; on extraction failure, serve the last-good XML (persisted to disk so it survives restarts).
- Per-requisition configuration layout (`config/requisitions/<name>/`) with secret interpolation for source credentials.

## Capabilities

### New Capabilities
- `requisition-server`: HTTP daemon that serves requisition XML per name, with TTL caching, on-demand refresh, and serve-last-good-on-failure semantics.
- `data-extraction`: built-in SQL/CSV/XLS extractors and the external subprocess (NDJSON) contract, all normalizing to a canonical `Record`.
- `requisition-transform`: declarative field-mapping from records to the requisition model, plus a Starlark scripting escape hatch.
- `requisition-model`: typed representation of the OpenNMS requisition (full fidelity incl. metadata and topology) and its XML serialization.

### Modified Capabilities
<!-- None — greenfield project, no existing specs. -->

## Impact

- New Go module (`gopris`), MIT licensed, single-binary daemon.
- Key dependencies: `net/http` (or `chi`) for HTTP, `database/sql` + drivers (SQL), `encoding/csv`, `excelize` (XLS), `encoding/xml` (XML), `starlark-go` (scripting); concurrency via goroutines (no async runtime).
- Establishes two public contracts that must be versioned carefully: the **subprocess/NDJSON extractor contract** (the plugin API) and the **requisition XML serialization** (must satisfy the OpenNMS import XSD or Provisiond rejects it).
- No changes to OpenNMS itself; integration is via the existing Provisiond requisition-import-URL mechanism.
