#!/usr/bin/env python3
# Copyright 2026 The OpenNMS Group, Inc.
# SPDX-License-Identifier: MIT
# Created by Ronny Trommer <ronny@opennms.com>
#
# Sample external data source implementing the gopris subprocess contract
# (see docs/subprocess-contract.md). Use it from a requisition with:
#
#   source:
#     type: exec
#     command: ["python3", "sample-source.py"]
#     params:
#       count: "3"
#
# It reads params as JSON on stdin, logs to stderr, and emits NDJSON records on
# stdout. Replace the body of `fetch` with a real inventory query.

import json
import sys


def fetch(params):
    """Yield one dict per node. Here we synthesize `count` hosts."""
    count = int(params.get("count", "2"))
    for i in range(1, count + 1):
        yield {
            "id": str(i),
            "name": f"host-{i}",
            "ip": f"192.0.2.{i}",
            "vendor": "Acme",
        }


def main():
    params = json.load(sys.stdin) if not sys.stdin.isatty() else {}
    print(f"sample-source: extracting with params={params}", file=sys.stderr)
    for record in fetch(params):
        sys.stdout.write(json.dumps(record) + "\n")


if __name__ == "__main__":
    main()
