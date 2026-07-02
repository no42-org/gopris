// Copyright 2026 The OpenNMS Group, Inc.
// SPDX-License-Identifier: MIT
// Created by Ronny Trommer <ronny@opennms.com>

// Package requisition provides a typed model of the OpenNMS provisioning
// requisition (the "model-import" document) and its XML serialization.
//
// The element and attribute layout mirrors the OpenNMS model-import.xsd. Field
// order within each struct is significant: the schema uses xs:sequence, so the
// structs are ordered to emit interfaces, categories, assets and meta-data in
// the order the schema requires.
package requisition

import (
	"encoding/xml"
)

// Namespace is the XML namespace of the OpenNMS model-import document.
const Namespace = "http://xmlns.opennms.org/xsd/config/model-import"

// SnmpPrimary enumerates the allowed values of the interface snmp-primary
// attribute (P = primary, S = secondary, N = not collected).
type SnmpPrimary string

const (
	SnmpPrimaryP SnmpPrimary = "P"
	SnmpPrimaryS SnmpPrimary = "S"
	SnmpPrimaryN SnmpPrimary = "N"
	// SnmpPrimaryC is retained only for backwards compatibility with very old
	// requisitions; the model-import schema still enumerates it.
	SnmpPrimaryC SnmpPrimary = "C"
)

// Requisition is the root <model-import> element.
type Requisition struct {
	XMLName       xml.Name `xml:"http://xmlns.opennms.org/xsd/config/model-import model-import"`
	DateStamp     string   `xml:"date-stamp,attr,omitempty"`
	LastImport    string   `xml:"last-import,attr,omitempty"`
	ForeignSource string   `xml:"foreign-source,attr,omitempty"`
	Nodes         []Node   `xml:"node"`
}

// Node is a <node> element.
type Node struct {
	// Attributes.
	NodeLabel           string `xml:"node-label,attr"`
	ForeignID           string `xml:"foreign-id,attr"`
	ParentForeignSource string `xml:"parent-foreign-source,attr,omitempty"`
	ParentForeignID     string `xml:"parent-foreign-id,attr,omitempty"`
	ParentNodeLabel     string `xml:"parent-node-label,attr,omitempty"`
	Location            string `xml:"location,attr,omitempty"`
	City                string `xml:"city,attr,omitempty"`
	Building            string `xml:"building,attr,omitempty"`
	// Children, in schema sequence order.
	Interfaces []Interface `xml:"interface"`
	Categories []Category  `xml:"category"`
	Assets     []Asset     `xml:"asset"`
	MetaData   []MetaData  `xml:"meta-data"`
}

// Interface is an <interface> element.
type Interface struct {
	IPAddr      string      `xml:"ip-addr,attr"`
	Descr       string      `xml:"descr,attr,omitempty"`
	Status      int         `xml:"status,attr,omitempty"`
	Managed     *bool       `xml:"managed,attr,omitempty"`
	SnmpPrimary SnmpPrimary `xml:"snmp-primary,attr,omitempty"`

	Services   []MonitoredService `xml:"monitored-service"`
	Categories []Category         `xml:"category"`
	MetaData   []MetaData         `xml:"meta-data"`
}

// MonitoredService is a <monitored-service> element.
type MonitoredService struct {
	ServiceName string     `xml:"service-name,attr"`
	Categories  []Category `xml:"category"`
	MetaData    []MetaData `xml:"meta-data"`
}

// Category is a <category> element.
type Category struct {
	Name string `xml:"name,attr"`
}

// Asset is an <asset> element.
type Asset struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// MetaData is a <meta-data> element (a context/key/value triple).
type MetaData struct {
	Context string `xml:"context,attr"`
	Key     string `xml:"key,attr"`
	Value   string `xml:"value,attr"`
}

// Managed is a helper for setting the optional *bool Managed field.
func Managed(v bool) *bool { return &v }

// ToXML renders the requisition as an indented XML document, including the
// XML declaration, ready to be served to OpenNMS Provisiond.
func (r *Requisition) ToXML() ([]byte, error) {
	body, err := xml.MarshalIndent(r, "", "   ")
	if err != nil {
		return nil, err
	}
	out := append([]byte(xml.Header), body...)
	out = append(out, '\n')
	return out, nil
}
