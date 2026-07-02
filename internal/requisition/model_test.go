// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

package requisition

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sample builds a fully populated requisition exercising every element type.
func sample() *Requisition {
	return &Requisition{
		DateStamp:     "2026-07-02T10:00:00.000-00:00",
		ForeignSource: "hosts",
		Nodes: []Node{{
			NodeLabel:           "router-1",
			ForeignID:           "n1",
			Location:            "Fulda",
			City:                "Fulda",
			Building:            "HQ",
			ParentForeignSource: "core",
			ParentForeignID:     "p1",
			ParentNodeLabel:     "spine-1",
			Interfaces: []Interface{{
				IPAddr:      "192.0.2.10",
				Descr:       "uplink",
				Status:      1,
				Managed:     Managed(true),
				SnmpPrimary: SnmpPrimaryP,
				Services: []MonitoredService{{
					ServiceName: "ICMP",
					MetaData:    []MetaData{{Context: "gopris", Key: "svc", Value: "ping"}},
				}},
				MetaData: []MetaData{{Context: "gopris", Key: "iface", Value: "eth0"}},
			}},
			Categories: []Category{{Name: "Servers"}},
			Assets:     []Asset{{Name: "vendor", Value: "Acme"}},
			MetaData:   []MetaData{{Context: "gopris", Key: "source", Value: "csv"}},
		}},
	}
}

// Task 2.9: round-trip test asserting every element appears in the XML.
func TestMarshalRoundTrip(t *testing.T) {
	r := sample()
	out, err := r.ToXML()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	xml := string(out)
	for _, want := range []string{
		`xmlns="http://xmlns.opennms.org/xsd/config/model-import"`,
		`foreign-source="hosts"`,
		`node-label="router-1"`, `foreign-id="n1"`,
		`parent-foreign-source="core"`, `parent-node-label="spine-1"`,
		`location="Fulda"`, `city="Fulda"`, `building="HQ"`,
		`ip-addr="192.0.2.10"`, `descr="uplink"`, `snmp-primary="P"`, `status="1"`, `managed="true"`,
		`service-name="ICMP"`,
		`<category name="Servers">`,
		`<asset name="vendor" value="Acme">`,
		`context="gopris"`, `key="source"`, `value="csv"`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("serialized XML missing %q\n---\n%s", want, xml)
		}
	}
}

// Task 2.8: validate serialized output against the real OpenNMS model-import XSD
// using xmllint. Skips (does not fail) when xmllint is unavailable.
func TestSerializedXMLIsSchemaValid(t *testing.T) {
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not available; skipping XSD validation")
	}
	out, err := sample().ToXML()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "requisition.xml")
	if err := os.WriteFile(xmlPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	xsd := filepath.Join("testdata", "model-import.xsd")
	cmd := exec.Command(xmllint, "--noout", "--schema", xsd, xmlPath)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xmllint schema validation failed: %v\n%s\n---\n%s", err, combined, out)
	}
}

func TestValidateCatchesBadValues(t *testing.T) {
	r := &Requisition{
		Nodes: []Node{{ // missing foreign-source, node-label, foreign-id
			Interfaces: []Interface{{IPAddr: "not-an-ip", SnmpPrimary: "X", Status: 7}},
		}},
	}
	err := r.Validate()
	if err == nil {
		t.Fatal("expected validation errors, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"foreign-source", "node-label", "not a valid IP", "snmp-primary", "status"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected validation error mentioning %q; got: %s", want, msg)
		}
	}
}

// Finding #12: snmp-primary "C" is valid per the XSD and must be accepted.
func TestValidateAcceptsSnmpPrimaryC(t *testing.T) {
	r := &Requisition{ForeignSource: "hosts", Nodes: []Node{{
		NodeLabel: "n", ForeignID: "1",
		Interfaces: []Interface{{IPAddr: "192.0.2.1", SnmpPrimary: SnmpPrimaryC}},
	}}}
	if err := r.Validate(); err != nil {
		t.Errorf("snmp-primary C should be accepted: %v", err)
	}
}

// Finding #3: duplicate foreign-ids across nodes are detected by full validation.
func TestValidateDetectsDuplicateForeignID(t *testing.T) {
	r := &Requisition{ForeignSource: "hosts", Nodes: []Node{
		{NodeLabel: "a", ForeignID: "dup"},
		{NodeLabel: "b", ForeignID: "dup"},
	}}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate foreign-id") {
		t.Errorf("expected duplicate foreign-id error, got: %v", err)
	}
}
