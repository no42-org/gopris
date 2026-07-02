<!--
Copyright 2026 The OpenNMS Group, Inc.
SPDX-License-Identifier: MIT
Created by Ronny Trommer <ronny@opennms.com>
-->

# gopris subprocess data-source contract

**Contract version: `1`**

The subprocess contract is the language-agnostic way to add a data source to
gopris. A source is any executable that follows this contract — write it in
shell, Python, Go, or anything else that can read stdin and write stdout.

gopris invokes the program once per extraction pass and turns each line of its
output into one canonical record, which then flows through the same transform
and serialization path as the built-in sources.

## The contract

| Channel | Direction | Meaning |
|---|---|---|
| **stdin** | gopris → program | A single JSON object: the `source.params` map from the requisition config (`{}` if none). |
| **stdout** | program → gopris | **NDJSON** — one JSON object per line. Each object becomes one record; its keys are field names. |
| **stderr** | program → gopris | Free-form log lines, surfaced in the gopris log. |
| **exit code** | program → gopris | `0` = success. Any non-zero exit (or a timeout) = extraction failed; gopris serves the last-good rendering instead. |
| **working directory** | — | The requisition's config directory, so relative paths resolve predictably. |
| **timeout** | — | `source.timeout` (default 60s). Exceeding it fails the extraction. |

### Field value coercion

Records are string-keyed, string-valued. JSON scalars are coerced:

| JSON | Record value |
|---|---|
| `"text"` | `text` |
| `42` | `42` |
| `true` / `false` | `true` / `false` |
| `null` | `""` (empty string) |
| object / array | re-encoded as compact JSON |

## Example

Configuration:

```yaml
source:
  type: exec
  command: ["python3", "inventory.py"]
  timeout: 30s
  params:
    region: eu-central
```

`inventory.py`:

```python
import json, sys
params = json.load(sys.stdin)          # {"region": "eu-central"}
print("querying " + params["region"], file=sys.stderr)
for host in fetch(params["region"]):   # your data source
    print(json.dumps({"name": host.name, "ip": host.ip}))
```

Each printed line becomes a record; the requisition's `transform` block maps
those fields onto nodes, interfaces, services, and so on.

## Versioning & upgrade path

This contract is a public API. Breaking changes bump the contract version.

When one-way NDJSON is not enough — you need streaming, progress reporting, or
the plugin to call back into gopris for configuration or secrets — the intended
upgrade path is a **gRPC plugin** (HashiCorp `go-plugin` style), which is the
same subprocess model plus a typed, bidirectional schema. See `design.md`
(decision D8) for the full escape-hatch axis.
