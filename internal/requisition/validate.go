// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package requisition

import (
	"errors"
	"fmt"
	"net"
)

// Validate checks the whole requisition against the constraints the OpenNMS
// model-import schema cannot express through Go's type system: foreign-source
// present, every node valid (see ValidateNode), and foreign-id unique across
// nodes (OpenNMS keys a node by (foreign-source, foreign-id), so duplicates are
// silently merged on import).
//
// encoding/xml is permissive, so these invariants are enforced explicitly
// before serialized output is trusted.
func (r *Requisition) Validate() error {
	var errs []error
	if r.ForeignSource == "" {
		errs = append(errs, errors.New("model-import: foreign-source is required"))
	}
	seen := make(map[string]struct{}, len(r.Nodes))
	for ni := range r.Nodes {
		n := &r.Nodes[ni]
		if err := ValidateNode(n); err != nil {
			errs = append(errs, fmt.Errorf("node[%d]: %w", ni, err))
		}
		if n.ForeignID != "" {
			if _, dup := seen[n.ForeignID]; dup {
				errs = append(errs, fmt.Errorf("node[%d]: duplicate foreign-id %q", ni, n.ForeignID))
			}
			seen[n.ForeignID] = struct{}{}
		}
	}
	return errors.Join(errs...)
}

// ValidateNode checks a single node's required attributes and interface
// constraints (node-label, foreign-id, ip-addr validity, snmp-primary and
// status enumerations). It does not check requisition-level invariants such as
// foreign-source or cross-node foreign-id uniqueness.
func ValidateNode(n *Node) error {
	var errs []error
	where := fmt.Sprintf("(foreign-id=%q)", n.ForeignID)
	if n.NodeLabel == "" {
		errs = append(errs, fmt.Errorf("%s: node-label is required", where))
	}
	if n.ForeignID == "" {
		errs = append(errs, fmt.Errorf("%s: foreign-id is required", where))
	}
	for ii := range n.Interfaces {
		iface := &n.Interfaces[ii]
		iwhere := fmt.Sprintf("%s interface[%d]", where, ii)
		if iface.IPAddr == "" {
			errs = append(errs, fmt.Errorf("%s: ip-addr is required", iwhere))
		} else if net.ParseIP(iface.IPAddr) == nil {
			errs = append(errs, fmt.Errorf("%s: ip-addr %q is not a valid IP address", iwhere, iface.IPAddr))
		}
		switch iface.SnmpPrimary {
		case "", SnmpPrimaryP, SnmpPrimaryS, SnmpPrimaryC, SnmpPrimaryN:
		default:
			errs = append(errs, fmt.Errorf("%s: snmp-primary %q must be one of P, S, C, N", iwhere, iface.SnmpPrimary))
		}
		switch iface.Status {
		case 0, 1, 3:
		default:
			errs = append(errs, fmt.Errorf("%s: status %d must be 1 (managed) or 3 (testing)", iwhere, iface.Status))
		}
		for si := range iface.Services {
			if iface.Services[si].ServiceName == "" {
				errs = append(errs, fmt.Errorf("%s service[%d]: service-name is required", iwhere, si))
			}
		}
	}
	return errors.Join(errs...)
}
