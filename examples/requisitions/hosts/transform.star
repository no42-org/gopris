# Copyright 2026 The OpenNMS Group, Inc.
# SPDX-License-Identifier: MIT
# Created by Ronny Trommer <ronny@opennms.com>
#
# Starlark transform escape hatch. gopris calls transform(record, node) once per
# record, after applying the declarative mapping. `record` is a dict of the raw
# string fields; `node` is the declaratively-built node. Return the node dict.

def transform(record, node):
    # Add a category derived from the location prefix of the node label.
    site = record["name"].split("-")[1] if "-" in record["name"] else "unknown"
    node["categories"] = node["categories"] + ["site:" + site]

    # Tag routers so OpenNMS can group them.
    if record["name"].startswith("router"):
        node["categories"] = node["categories"] + ["Routers"]

    return node
