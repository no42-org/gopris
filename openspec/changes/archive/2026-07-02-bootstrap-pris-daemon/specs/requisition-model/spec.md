## ADDED Requirements

### Requirement: Typed requisition model
The system SHALL represent an OpenNMS requisition as a typed model comprising a foreign-source and a set of nodes, where each node has a foreign-id and node-label and may carry interfaces, monitored-services, categories, assets, meta-data, and parent-topology references.

#### Scenario: Model holds a full node
- **WHEN** a node is constructed with a foreign-id, node-label, one interface, one monitored-service, one category, and one asset
- **THEN** the model retains all of those elements associated with that node

### Requirement: Interface fidelity
The requisition model SHALL represent an interface with its IP address and SHALL support the OpenNMS interface attributes `descr`, `snmp-primary` (one of `P`, `S`, `N`), `status`, and `managed`.

#### Scenario: Primary SNMP interface
- **WHEN** an interface is set with `snmp-primary` = `P` and a status value
- **THEN** the serialized interface carries `snmp-primary="P"` and the given status

### Requirement: Metadata contexts
The requisition model SHALL support `meta-data` entries (context, key, value) at node, interface, and monitored-service scope.

#### Scenario: Node-scoped metadata
- **WHEN** a node is given a meta-data entry with a context, key, and value
- **THEN** the serialized node includes that meta-data entry under the node

### Requirement: Parent topology
The requisition model SHALL support node parent references via `parent-foreign-source`, `parent-foreign-id`, and `parent-node-label` so that node hierarchy can be expressed.

#### Scenario: Node with a parent reference
- **WHEN** a node is given a parent-foreign-id (and optional parent-foreign-source)
- **THEN** the serialized node includes the parent reference attributes

### Requirement: Schema-valid XML serialization
The system SHALL serialize the requisition model to XML that conforms to the OpenNMS requisition import schema so that OpenNMS Provisiond accepts the import.

#### Scenario: Serialized requisition is schema-valid
- **WHEN** a populated requisition model is serialized to XML
- **THEN** the output validates against the OpenNMS requisition import schema

#### Scenario: Round-trip preserves elements
- **WHEN** a requisition model containing nodes, interfaces, services, categories, assets, and meta-data is serialized
- **THEN** every element present in the model appears in the serialized XML
