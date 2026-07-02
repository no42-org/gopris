<!--
Copyright 2026 The OpenNMS Group, Inc.
SPDX-License-Identifier: MIT
Created by Ronny Trommer <ronny@opennms.com>
-->

# gopris — OpenNMS Provisioning Integration Server

`gopris` is a small, MIT-licensed daemon that turns inventory from external data
sources (SQL, CSV, spreadsheets, or anything you can script) into **OpenNMS
provisioning requisitions**, served as XML over HTTP for OpenNMS Provisiond to
import.

It is a modern Go rewrite of the original Java
[PRIS](https://github.com/opennms/opennms-provisioning-integration-server),
built for system and network operators: a single static binary, language-
agnostic data sources, and requisition output validated against the OpenNMS
schema.

## How it works

```
 OpenNMS Provisiond ──GET /requisitions/<name>──▶ gopris
                                                     │
                     ┌───────────────────────────────┼──────────────────────────┐
                     │  EXTRACT            TRANSFORM            SERIALIZE          │
                     │  csv│sql│xls│exec ─▶ declarative + ─▶ typed requisition ──▶ XML
                     │  (canonical Record)  Starlark          (XSD-valid)          │
                     └──────────────────────────────────────────────────────────┘
```

- **Pull delivery.** gopris is a passive server. Point a Provisiond
  requisition import URL at `http://<host>:8080/requisitions/<name>`.
- **On-demand + cache.** Each request renders on demand, caches the XML for the
  requisition's `ttl`, and serves the last successful rendering (persisted to
  disk) if a later extraction fails. Concurrent polls of one requisition share a
  single render.
- **Tolerant of messy data.** An individual record that maps to an invalid node
  (or a duplicate `foreign-id`) is skipped and logged; the rest of the
  requisition is still served, so one bad row never blocks every good host.
- **One pipeline for every source.** Built-in and external sources both produce
  the same canonical records, so the transform layer is identical regardless of
  source.

## Quick start

```sh
make build
./bin/gopris --config examples/requisitions --state ./_state
curl http://localhost:8080/requisitions/hosts
```

Flags: `--config` (dir of requisition dirs, default `config/requisitions`),
`--state` (last-good store, default `state`), `--addr` (default `:8080`).

## Configuration

One directory per requisition under the config root, each with a
`requisition.yaml`:

```
config/requisitions/
  hosts/
    requisition.yaml     # source + transform + ttl
    nodes.csv            # (source data, if file-based)
    transform.star       # (optional Starlark escape hatch)
```

```yaml
foreign-source: hosts          # OpenNMS foreign-source (defaults to dir name)
ttl: 5m                        # cache lifetime for rendered XML (default 5m if unset;
                               # set a small value like 10s for near-real-time)

source:
  type: csv                    # csv | sql | xls | exec
  file: nodes.csv              # csv/xls: path relative to this dir
  # sql:   driver, dsn, query
  # xls:   sheet, header-row
  # exec:  command, timeout, params

transform:
  foreign-id: "${id}"          # ${field} templates; hash of the record if empty
  node-label: "${name}"        # ${env:NAME}/${file:/path} also resolve in any value
  location: "${location}"
  interfaces:
    - ip-addr: "${ip}"
      snmp-primary: "P"        # P | S | N
      status: 1                # 1 (managed) | 3 (testing)
      services: [ICMP, SNMP]
  categories: ["${role}"]
  assets:
    - name: vendor
      value: "${vendor}"
  metadata:
    - {context: gopris, key: source, value: csv}
  script: transform.star       # optional; see escape hatch below
```

Credentials use secret references, resolved at load time:
`${env:NAME}` (environment variable) or `${file:/path}` (file contents).

### Source types

| Type | Reads | Key options |
|---|---|---|
| `csv` | a CSV file (first row = header) | `file` |
| `sql` | one record per result row | `driver`, `dsn`, `query` |
| `xls` | a spreadsheet (`.xls`/`.xlsx`) | `file`, `sheet`, `header-row` |
| `exec` | NDJSON from any executable | `command`, `timeout`, `params` |

SQLite is bundled (`driver: sqlite`). Other databases work by importing their
`database/sql` driver and setting `driver`/`dsn` accordingly.

## Extending gopris — the escape-hatch axis

gopris has two extension points, each with a simple default and a heavier
upgrade path for when you outgrow it (see `design.md`, decision D8):

```
  extract   → subprocess/NDJSON   (→ go-plugin/gRPC when you outgrow it)
  transform → Starlark            (→ WASM/wazero when it must be sandboxed)
```

- **New data source → subprocess contract.** Write an executable in any language
  that reads JSON params on stdin and emits NDJSON records on stdout. No Go, no
  rebuild. See [`docs/subprocess-contract.md`](docs/subprocess-contract.md) and
  [`examples/plugins/sample-source.py`](examples/plugins/sample-source.py). When
  you need streaming or callbacks, graduate to a **gRPC plugin** (`go-plugin`) —
  the same subprocess model with a typed, bidirectional schema.
- **Custom mapping logic → Starlark.** Add a `script:` that defines
  `transform(record, node)` and returns the node. When you need to run
  *untrusted* mapping logic, the upgrade path is a **WASM** sandbox (`wazero`).

## Development

```sh
make verify   # go vet + go test + build (what CI runs)
```

## License

MIT — see [LICENSE](LICENSE).
