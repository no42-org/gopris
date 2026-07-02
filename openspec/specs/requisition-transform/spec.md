# requisition-transform Specification

## Purpose

Turn canonical records into requisition model elements through declarative field mapping, with a Starlark scripting escape hatch for logic the declarative mapping cannot express, deriving deterministic node identities.

## Requirements

### Requirement: Declarative field mapping
The system SHALL allow a requisition's transform to be expressed declaratively, mapping canonical record fields onto requisition model elements (node identity and label, interfaces, monitored services, categories, and assets) without writing code.

#### Scenario: Record mapped to a node via declarative config
- **WHEN** a canonical record is processed under a declarative mapping that assigns record fields to node label, foreign-id, and an interface address
- **THEN** the system produces a requisition node with the corresponding node-label, foreign-id, and interface

#### Scenario: One record maps to node with services and categories
- **WHEN** a declarative mapping assigns monitored services and categories from record fields
- **THEN** the produced node includes those monitored-services and categories

### Requirement: Starlark scripting escape hatch
The system SHALL support a Starlark script as an escape hatch for cleanup and derived logic that the declarative mapping cannot express, operating over canonical records to produce or adjust requisition model elements.

#### Scenario: Script derives a field the declarative mapping cannot
- **WHEN** a requisition is configured with a Starlark transform script and a record is processed
- **THEN** the script runs over the record and its output contributes to the resulting requisition node

#### Scenario: Script error is reported
- **WHEN** a Starlark transform script raises an error while processing a record
- **THEN** the transform fails for that requisition and the error is logged with context identifying the requisition

### Requirement: Deterministic node identity
The transform SHALL derive each node's `foreign-id` deterministically from record fields so that the same input record yields the same node identity across refreshes.

#### Scenario: Same record yields same foreign-id
- **WHEN** the same canonical record is transformed on two separate refreshes
- **THEN** the resulting node has the same foreign-id both times
