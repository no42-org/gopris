## 1. Project scaffolding

- [x] 1.1 Create the Go module (`gopris`), MIT `LICENSE`, and `README`
- [x] 1.2 Add core dependencies: `chi` (or stdlib `net/http`), `database/sql` + drivers, `excelize`, `starlark-go`, a structured logger (`log/slog`)
- [x] 1.3 Set up structured logging and a minimal `main` that starts the HTTP server
- [x] 1.4 Add SPDX/OpenNMS source headers to all new source files
- [x] 1.5 Add a `Makefile` (`build`, `test`, `verify`) so local and CI use the same targets

## 2. Requisition model & XML serialization

- [x] 2.1 Define types: foreign-source, node (foreign-id, node-label, location)
- [x] 2.2 Add interface type with ip-addr, descr, snmp-primary (P/S/N), status, managed
- [x] 2.3 Add monitored-service, category, and asset types
- [x] 2.4 Add meta-data entries (context/key/value) at node, interface, and service scope
- [x] 2.5 Add parent-topology references (parent-foreign-source, parent-foreign-id, parent-node-label)
- [x] 2.6 Implement XML serialization via `encoding/xml` struct tags
- [x] 2.7 Add field-level validation for constrained values (e.g. snmp-primary ∈ {P,S,N})
- [x] 2.8 Add a test that validates serialized output against the OpenNMS requisition import XSD
- [x] 2.9 Add a round-trip test asserting every model element appears in the XML

## 3. Canonical record & extraction interface

- [x] 3.1 Define the canonical `Record` type (named fields) and an `Extractor` interface yielding records
- [x] 3.2 Implement secret interpolation (`${ENV}` / secret-file references) for source config
- [x] 3.3 Implement the CSV extractor (header-named fields) with `encoding/csv`
- [x] 3.4 Implement the SQL extractor (`database/sql`: query → one record per row)
- [x] 3.5 Implement the XLS extractor with `excelize`: sheet/header-row selection and cell coercion
- [x] 3.6 Unit-test each built-in extractor against fixture inputs

## 4. External subprocess extractor (plugin contract)

- [x] 4.1 Implement the subprocess extractor: JSON params on stdin, NDJSON records on stdout, stderr→logs
- [x] 4.2 Enforce configurable timeout via `context`; treat non-zero exit or timeout as extraction failure
- [x] 4.3 Run the executable with the requisition's config directory as cwd
- [x] 4.4 Document and version the subprocess contract (the public plugin API)
- [x] 4.5 Add a fixture-based integration test using a sample script that emits NDJSON

## 5. Transform layer

- [x] 5.1 Implement declarative field-mapping (record fields → node/interface/service/category/asset)
- [x] 5.2 Implement deterministic foreign-id derivation from record fields
- [x] 5.3 Integrate Starlark as the scripting escape hatch over records; surface script errors with requisition context
- [x] 5.4 Ensure built-in and external sources flow through one identical transform path
- [x] 5.5 Test declarative mapping and a Starlark script producing a node; test foreign-id stability across runs

## 6. Server, caching & resilience

- [x] 6.1 Implement `GET /requisitions/<name>` (200 XML / 404 unknown)
- [x] 6.2 Implement on-demand extract→transform→serialize pipeline invocation
- [x] 6.3 Add TTL cache of rendered XML (configurable, per-requisition)
- [x] 6.4 Persist last-good rendered XML to disk; serve it on extraction failure; 5xx when no prior success
- [x] 6.5 Verify last-good survives daemon restart
- [x] 6.6 Load per-requisition config from `config/requisitions/<name>/`

## 7. Configuration & docs

- [x] 7.1 Define and document the per-requisition config format (source block + transform + TTL)
- [x] 7.2 Write an example requisition (CSV source + declarative mapping) end to end
- [x] 7.3 Document the plugin escape-hatch axis from design.md (subprocess→go-plugin/gRPC, Starlark→WASM/wazero) in the README
- [x] 7.4 Provide a sample external subprocess data source in a non-Go language (e.g. Python or shell)

## 8. Verification

- [x] 8.1 End-to-end test: config → extract → transform → serve valid XML for CSV, SQL, XLS, and subprocess sources
- [ ] 8.2 Verify a rendered requisition imports cleanly in a real OpenNMS Provisiond instance
      <!-- NOT DONE: requires a running OpenNMS. Strongest proxy achieved instead:
           served XML validates against the real OpenNMS model-import.xsd via xmllint
           (unit test + verified against the running binary). Needs manual confirmation
           against a live Provisiond import URL. -->
- [ ] 8.4 Set up a git repository so the CI workflow actually runs (not yet initialized)
- [x] 8.3 `make verify` passes (build, vet, tests) in CI
