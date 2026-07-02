## Context

The original PRIS (Java/Spring, Maven) is a pull-based integration server: it renders an OpenNMS requisition as XML at a URL, and OpenNMS Provisiond is configured with that import URL and fetches it on a schedule. It supports XLS, JDBC, script, and OCS sources via a plugin architecture.

This design bootstraps a Go replacement (**gopris**, MIT) with the same delivery model but a cleaner internal pipeline and a language-agnostic extension story. The dominant constraint is a product one: **third parties must be able to add data sources very easily, without knowing Go or rebuilding the daemon.** That single requirement shapes the plugin boundary more than any language feature does — neither Go nor Rust has a robust native dynamic-plugin story (Go's `plugin` package is Linux-only and brittle).

Go was chosen over Rust deliberately. The workload is I/O-bound network glue (fetch rows → reshape → emit XML over HTTP) with no CPU-bound hot path, so Rust's performance advantages do not apply. The decisive factors were audience and reach: the target users are system and network operators, whose entire tooling ecosystem (Prometheus, Grafana, Telegraf, Kubernetes, Terraform) is Go, giving the broadest contributor pool and the widest database-driver coverage for the long tail of enterprise databases these users run. Go's goroutine concurrency is also a simpler fit than async Rust for orchestrating concurrent polls, background refresh, and extractor subprocesses.

## Goals / Non-Goals

**Goals:**
- Single static binary, passive HTTP daemon; Provisiond pulls requisitions (no push to OpenNMS).
- One internal pipeline — `extract → normalize(Record) → transform → requisition model → XML` — used identically by every source.
- Built-in SQL, CSV, XLS extractors; a language-agnostic subprocess contract for everything else.
- Declarative mapping for the common case, Starlark for the escape hatch.
- Full-fidelity requisition model that serializes to XSD-valid XML.
- Resilience: serve last-good XML when a source is down.

**Non-Goals:**
- Pushing requisitions via the OpenNMS ReST API (explicitly deferred; delivery is pull-only for now).
- In-process gRPC or WASM plugins in v1 (documented below as future upgrade paths, not built now).
- Porting the legacy OCS source or the legacy config/property format verbatim.
- A management UI. Configuration is files on disk.

## Decisions

### D1. Delivery: passive HTTP server, Provisiond pulls
`GET /requisitions/<name>` returns the rendered requisition XML. This matches the classic PRIS model and keeps gopris fully decoupled from OpenNMS timing and credentials.
*Alternative considered — push via ReST:* gopris POSTs to `/rest/requisitions` and triggers imports. Rejected for v1 because it makes gopris an active client needing OpenNMS credentials and its own scheduler, and couples release timing to OpenNMS. Left as a future capability.

### D2. Split "data source" into Extract and Transform
Extract needs I/O (DB drivers, network, files, credentials) and is hard to sandbox. Transform is pure record→model shaping and is easy to sandbox or script. The legacy tool blurs these; separating them is what makes the plugin story tractable — an extractor author only ever produces records and never touches requisition XML.

### D3. Canonical `Record` as the internal spine
Every extractor — built-in or external — emits the same normalized `Record` (named fields). The transform layer is written once against `Record`. This uniformity is the core simplification of the whole design.

### D4. Extract extensibility: subprocess/NDJSON contract (default)
An external source is any executable. Contract:
- **stdin** ← JSON params (the source's config block, so one program can serve many requisitions),
- **stdout** → NDJSON, one canonical record per line,
- **stderr** → logs (surfaced in gopris logs),
- **exit 0** = success; non-zero = extraction failed (→ serve last-good),
- plus a declared **timeout**, and **cwd** = the requisition's config directory.

This is the public plugin API and MUST be versioned. It matches the spirit of the legacy "script" source and lets authors use bash/Python/SQL/anything — a perfect fit for an ops audience that lives in shell and Python.
*Alternatives considered:* compile-in Go interface (robust/typed but requires Go + rebuild — fails the "very easy" bar for outsiders); native dynamic loading via Go's `plugin` package (Linux-only, brittle, version-lockstep with the host — unusable in practice).

### D5. Transform: declarative field-mapping + Starlark escape hatch
Declarative config expresses the 80% case (field→node/interface/service/category/asset). Starlark (`starlark-go`, pure Go, deterministic, sandboxable) handles cleanup and derived logic. It is already familiar to infra people as the configuration language of Bazel and Tilt. Same pipeline for built-in and external sources.
*Alternatives considered:* config-only (too limited for real-world cleanup); scripting-only (higher barrier for simple mappings); Lua/Tengo/`expr` (viable, but Starlark's determinism and ops familiarity win). The declarative+script hybrid is more to build but is the best UX.

### D6. Refresh: on-demand extraction + short TTL cache, serve last-good on failure
Provisiond polls on its own schedule. On a poll: if the cached XML is within TTL, serve it; otherwise extract+transform live, cache the result, and serve it. On extraction failure, serve the last successful XML. **The cache must persist to disk** (rendered XML per requisition) so serve-last-good survives a restart.
*Alternatives considered:* pure on-demand per GET (a slow DB stalls the import, hammers the source); scheduled background refresh only (adds a scheduler and a fixed staleness window). On-demand+TTL absorbs poll bursts while staying simple.

### D7. Full-fidelity requisition model
Model the OpenNMS requisition as Go types: `foreign-source`; nodes (`foreign-id`, `node-label`, `location`, parent topology via `parent-foreign-source`/`parent-foreign-id`); interfaces (`ip-addr`, `descr`, `snmp-primary` P/S/N, `status`, `managed`); monitored-services; categories; assets; and `meta-data` contexts at node/interface/service scope. Serialize with `encoding/xml` (struct tags) to satisfy the OpenNMS import XSD; validate constrained fields such as `snmp-primary` against their allowed values rather than relying on the type system.

### D8. Plugin escape-hatch axis (documented defaults + upgrade paths)

The two extension points sit on **different axes**, and each has a natural upgrade path. This is documented now so the v1 defaults are understood as points on a spectrum, not dead ends:

```
                 I/O freedom (DB/net) →
                 LOW                         HIGH
   sandbox  ┌──────────────────┬──────────────────┐
   STRONG   │  WASM/wazero      │  (needs host I/O │
            │  ← transform's    │   funcs → awkward)│
            │    sweet spot     │                   │
            ├──────────────────┼──────────────────┤
   WEAK/    │                   │  gRPC/go-plugin   │
   none     │                   │  ← extract's      │
            │                   │    sweet spot     │
            └──────────────────┴──────────────────┘
```

- **Extract** → default **subprocess/NDJSON** (D4). **Upgrade path: gRPC via HashiCorp `go-plugin`.** go-plugin *is* the subprocess contract plus a typed schema and bidirectional calls — it launches the plugin binary and they talk gRPC over a local socket. It is Go-native. Adopt it if/when NDJSON-over-stdout becomes limiting (streaming, progress reporting, or the plugin calling back into the host for config/secrets). Same trust model (full I/O, operator-trusted), more structure. It is an *evolution* of the v1 default, not a replacement.

- **Transform** → default **Starlark** (D5). **Upgrade path: WASM via `wazero`** (a pure-Go WASM runtime, no cgo). Transform is pure computation over records — precisely what WASM sandboxes well, and the one place sandboxing matters (running *untrusted* user mapping logic). Adopt WASM here if untrusted transforms or a single-file, cross-platform, no-subprocess plugin artifact becomes a requirement.

- **Anti-pattern to avoid:** WASM for *extract*. Extractors need DB/network I/O, which WASM only exposes via host-provided capability functions (WASI DB drivers are rare) — you'd rebuild I/O plumbing you get for free from a subprocess. WASM's home in this project is transform, not extract.

Rule of thumb baked into the design:
```
  extract   → subprocess/NDJSON   (→ go-plugin/gRPC when you outgrow it)   needs I/O, operator-trusted
  transform → Starlark            (→ WASM/wazero when it must be sandboxed) pure logic, sandboxable
```

Both upgrade paths are Go-native (`go-plugin`, `wazero`), which reinforces the language choice: the extensibility roadmap needs no foreign toolchain.

## Risks / Trade-offs

- **XLS is messy** (merged cells, typed vs string cells, multiple sheets, header-row ambiguity) → the XLS extractor needs an explicit config surface (which sheet, which header row, cell coercion rules). `excelize` is mature and handles the parsing; the risk is the config/coercion surface, so budget for it rather than assuming spreadsheets read cleanly.
- **Requisition XSD drift** → invalid XML makes Provisiond silently reject the import. Mitigation: validate serialized output against the OpenNMS import XSD in tests; treat the XML shape as a versioned contract. Note `encoding/xml` is more permissive than a typed model, so field-level validation (e.g. `snmp-primary` ∈ {P,S,N}) must be explicit.
- **Subprocess contract is a public API** → careless changes break third-party sources. Mitigation: version the contract (D4) and cover it with fixture-based integration tests.
- **Serve-last-good needs durable state** → an in-memory-only cache loses resilience across restarts. Mitigation: persist rendered XML per requisition to disk (D6).
- **Subprocess security** → external extractors run with the daemon's privileges and can be arbitrary programs. Mitigation: document the trust model (operator-controlled config), enforce timeouts, and keep the door open to the WASM sandbox for the transform stage where untrusted logic is more likely.
- **Secrets in config** → SQL credentials must not be plaintext in shared config. Mitigation: support `${ENV}` / secret-file interpolation in the source config before the format ossifies.

## Open Questions

- Exact typed-vs-string representation of `Record` field values (does the canonical record carry type hints, or is coercion entirely the transform layer's job?).
- Whether the declarative mapping is TOML/YAML tables or a small purpose-built DSL — and where the seam to Starlark sits.
- `foreign-id` stability and dedup semantics across refreshes (how is a node's identity derived deterministically from a record?).
- Cache TTL default and whether it is per-requisition configurable.
- HTTP layer: standard-library `net/http` alone, or a light router (`chi`)?
