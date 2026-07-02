// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package transform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/opennms/gopris/internal/record"
	"github.com/opennms/gopris/internal/requisition"
	"go.starlark.net/starlark"
)

// starlarkScript wraps a compiled Starlark transform. The script must define a
// function `transform(record, node)` that returns the (possibly modified) node
// dict. `record` is a dict of the record's string fields; `node` is a dict
// representation of the declaratively-built node.
type starlarkScript struct {
	fn   starlark.Callable
	name string
}

func loadScript(dir, path string) (*starlarkScript, error) {
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(dir, path)
	}
	src, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("transform script: %w", err)
	}
	thread := &starlark.Thread{Name: "gopris-load"}
	globals, err := starlark.ExecFile(thread, full, src, nil)
	if err != nil {
		return nil, fmt.Errorf("transform script %s: %w", path, err)
	}
	v, ok := globals["transform"]
	if !ok {
		return nil, fmt.Errorf("transform script %s: must define a function transform(record, node)", path)
	}
	fn, ok := v.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("transform script %s: 'transform' is not a function", path)
	}
	return &starlarkScript{fn: fn, name: path}, nil
}

// run executes transform(record, node) and converts the result back to a node.
func (s *starlarkScript) run(r record.Record, n requisition.Node) (requisition.Node, error) {
	thread := &starlark.Thread{Name: "gopris-transform"}
	args := starlark.Tuple{recordToStarlark(r), nodeToStarlark(n)}
	res, err := starlark.Call(thread, s.fn, args, nil)
	if err != nil {
		return requisition.Node{}, fmt.Errorf("transform script %s: %w", s.name, err)
	}
	dict, ok := res.(*starlark.Dict)
	if !ok {
		return requisition.Node{}, fmt.Errorf("transform script %s: transform() must return a node dict, got %s", s.name, res.Type())
	}
	return nodeFromStarlark(dict)
}

func recordToStarlark(r record.Record) *starlark.Dict {
	d := starlark.NewDict(len(r.Fields))
	for k, v := range r.Fields {
		_ = d.SetKey(starlark.String(k), starlark.String(v))
	}
	return d
}

func nodeToStarlark(n requisition.Node) *starlark.Dict {
	d := starlark.NewDict(12)
	set := func(k, v string) { _ = d.SetKey(starlark.String(k), starlark.String(v)) }
	set("foreign_id", n.ForeignID)
	set("node_label", n.NodeLabel)
	set("location", n.Location)
	set("city", n.City)
	set("building", n.Building)
	set("parent_foreign_source", n.ParentForeignSource)
	set("parent_foreign_id", n.ParentForeignID)
	set("parent_node_label", n.ParentNodeLabel)

	ifaces := make([]starlark.Value, 0, len(n.Interfaces))
	for _, iface := range n.Interfaces {
		id := starlark.NewDict(6)
		_ = id.SetKey(starlark.String("ip_addr"), starlark.String(iface.IPAddr))
		_ = id.SetKey(starlark.String("descr"), starlark.String(iface.Descr))
		_ = id.SetKey(starlark.String("snmp_primary"), starlark.String(string(iface.SnmpPrimary)))
		_ = id.SetKey(starlark.String("status"), starlark.MakeInt(iface.Status))
		if iface.Managed != nil {
			_ = id.SetKey(starlark.String("managed"), starlark.Bool(*iface.Managed))
		} else {
			_ = id.SetKey(starlark.String("managed"), starlark.None)
		}
		svcs := make([]starlark.Value, 0, len(iface.Services))
		for _, s := range iface.Services {
			svcs = append(svcs, starlark.String(s.ServiceName))
		}
		_ = id.SetKey(starlark.String("services"), starlark.NewList(svcs))
		ifaces = append(ifaces, id)
	}
	_ = d.SetKey(starlark.String("interfaces"), starlark.NewList(ifaces))

	cats := make([]starlark.Value, 0, len(n.Categories))
	for _, c := range n.Categories {
		cats = append(cats, starlark.String(c.Name))
	}
	_ = d.SetKey(starlark.String("categories"), starlark.NewList(cats))

	assets := make([]starlark.Value, 0, len(n.Assets))
	for _, a := range n.Assets {
		ad := starlark.NewDict(2)
		_ = ad.SetKey(starlark.String("name"), starlark.String(a.Name))
		_ = ad.SetKey(starlark.String("value"), starlark.String(a.Value))
		assets = append(assets, ad)
	}
	_ = d.SetKey(starlark.String("assets"), starlark.NewList(assets))

	meta := make([]starlark.Value, 0, len(n.MetaData))
	for _, m := range n.MetaData {
		md := starlark.NewDict(3)
		_ = md.SetKey(starlark.String("context"), starlark.String(m.Context))
		_ = md.SetKey(starlark.String("key"), starlark.String(m.Key))
		_ = md.SetKey(starlark.String("value"), starlark.String(m.Value))
		meta = append(meta, md)
	}
	_ = d.SetKey(starlark.String("metadata"), starlark.NewList(meta))
	return d
}

func nodeFromStarlark(d *starlark.Dict) (requisition.Node, error) {
	var n requisition.Node
	var err error
	for _, f := range []struct {
		key string
		dst *string
	}{
		{"foreign_id", &n.ForeignID}, {"node_label", &n.NodeLabel}, {"location", &n.Location},
		{"city", &n.City}, {"building", &n.Building}, {"parent_foreign_source", &n.ParentForeignSource},
		{"parent_foreign_id", &n.ParentForeignID}, {"parent_node_label", &n.ParentNodeLabel},
	} {
		if *f.dst, err = optStr(d, f.key); err != nil {
			return n, err
		}
	}

	ifaces, err := optList(d, "interfaces")
	if err != nil {
		return n, err
	}
	for idx, v := range ifaces {
		id, ok := v.(*starlark.Dict)
		if !ok {
			return n, fmt.Errorf("interfaces[%d] must be a dict, got %s", idx, v.Type())
		}
		var iface requisition.Interface
		var sp string
		if iface.IPAddr, err = optStr(id, "ip_addr"); err != nil {
			return n, err
		}
		if iface.Descr, err = optStr(id, "descr"); err != nil {
			return n, err
		}
		if sp, err = optStr(id, "snmp_primary"); err != nil {
			return n, err
		}
		iface.SnmpPrimary = requisition.SnmpPrimary(sp)
		if iface.Status, err = optInt(id, "status"); err != nil {
			return n, err
		}
		if iface.Managed, err = optBoolPtr(id, "managed"); err != nil {
			return n, err
		}
		svcs, err := optList(id, "services")
		if err != nil {
			return n, err
		}
		for si, sv := range svcs {
			name, ok := starlark.AsString(sv)
			if !ok {
				return n, fmt.Errorf("interfaces[%d].services[%d] must be a string, got %s", idx, si, sv.Type())
			}
			iface.Services = append(iface.Services, requisition.MonitoredService{ServiceName: name})
		}
		n.Interfaces = append(n.Interfaces, iface)
	}

	cats, err := optList(d, "categories")
	if err != nil {
		return n, err
	}
	for i, v := range cats {
		name, ok := starlark.AsString(v)
		if !ok {
			return n, fmt.Errorf("categories[%d] must be a string, got %s", i, v.Type())
		}
		n.Categories = append(n.Categories, requisition.Category{Name: name})
	}

	assets, err := optList(d, "assets")
	if err != nil {
		return n, err
	}
	for i, v := range assets {
		ad, ok := v.(*starlark.Dict)
		if !ok {
			return n, fmt.Errorf("assets[%d] must be a dict, got %s", i, v.Type())
		}
		var a requisition.Asset
		if a.Name, err = optStr(ad, "name"); err != nil {
			return n, err
		}
		if a.Value, err = optStr(ad, "value"); err != nil {
			return n, err
		}
		n.Assets = append(n.Assets, a)
	}

	meta, err := optList(d, "metadata")
	if err != nil {
		return n, err
	}
	for i, v := range meta {
		md, ok := v.(*starlark.Dict)
		if !ok {
			return n, fmt.Errorf("metadata[%d] must be a dict, got %s", i, v.Type())
		}
		var m requisition.MetaData
		if m.Context, err = optStr(md, "context"); err != nil {
			return n, err
		}
		if m.Key, err = optStr(md, "key"); err != nil {
			return n, err
		}
		if m.Value, err = optStr(md, "value"); err != nil {
			return n, err
		}
		n.MetaData = append(n.MetaData, m)
	}
	return n, nil
}

// optStr returns the string value of key, "" if absent, or an error if the key
// is present but not a string. Silent coercion is deliberately avoided so a
// mistyped script field fails loudly instead of producing a wrong node.
func optStr(d *starlark.Dict, key string) (string, error) {
	v, ok, _ := d.Get(starlark.String(key))
	if !ok || v == starlark.None {
		return "", nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return "", fmt.Errorf("field %q must be a string, got %s", key, v.Type())
	}
	return s, nil
}

func optInt(d *starlark.Dict, key string) (int, error) {
	v, ok, _ := d.Get(starlark.String(key))
	if !ok || v == starlark.None {
		return 0, nil
	}
	i, ok := v.(starlark.Int)
	if !ok {
		return 0, fmt.Errorf("field %q must be an int, got %s", key, v.Type())
	}
	n, _ := i.Int64()
	return int(n), nil
}

func optBoolPtr(d *starlark.Dict, key string) (*bool, error) {
	v, ok, _ := d.Get(starlark.String(key))
	if !ok || v == starlark.None {
		return nil, nil
	}
	b, ok := v.(starlark.Bool)
	if !ok {
		return nil, fmt.Errorf("field %q must be a bool, got %s", key, v.Type())
	}
	return requisition.Managed(bool(b)), nil
}

// optList returns the elements of a list-valued key, nil if absent, or an error
// if the key is present but not a list.
func optList(d *starlark.Dict, key string) ([]starlark.Value, error) {
	v, ok, _ := d.Get(starlark.String(key))
	if !ok || v == starlark.None {
		return nil, nil
	}
	l, ok := v.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("field %q must be a list, got %s", key, v.Type())
	}
	out := make([]starlark.Value, 0, l.Len())
	iter := l.Iterate()
	defer iter.Done()
	var e starlark.Value
	for iter.Next(&e) {
		out = append(out, e)
	}
	return out, nil
}
