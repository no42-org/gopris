## ADDED Requirements

### Requirement: Serve requisition XML by name
The system SHALL expose an HTTP endpoint `GET /requisitions/<name>` that returns the OpenNMS requisition XML for the named requisition, suitable for import by OpenNMS Provisiond.

#### Scenario: Known requisition requested
- **WHEN** a client requests `GET /requisitions/<name>` for a configured requisition
- **THEN** the system responds `200 OK` with `Content-Type: application/xml` and the rendered requisition XML in the body

#### Scenario: Unknown requisition requested
- **WHEN** a client requests `GET /requisitions/<name>` for a name that is not configured
- **THEN** the system responds `404 Not Found`

### Requirement: On-demand extraction with TTL cache
The system SHALL render a requisition by running its extract → transform pipeline on demand, and SHALL cache the rendered XML for a configurable time-to-live (TTL) to absorb repeated polls.

#### Scenario: Cache hit within TTL
- **WHEN** a requisition is requested and a cached rendering exists whose age is within the TTL
- **THEN** the system serves the cached XML without re-running extraction

#### Scenario: Cache miss or stale
- **WHEN** a requisition is requested and no cached rendering exists or the cached rendering is older than the TTL
- **THEN** the system runs the extract → transform pipeline, caches the resulting XML, and serves it

### Requirement: Serve last-good on extraction failure
The system SHALL persist the last successfully rendered XML for each requisition to disk and SHALL serve it when a subsequent extraction fails, so that a source outage does not break Provisiond imports.

#### Scenario: Extraction fails with a prior success available
- **WHEN** extraction or transformation fails for a requisition that has a previously persisted successful rendering
- **THEN** the system serves the last-good persisted XML and logs the failure

#### Scenario: Extraction fails with no prior success
- **WHEN** extraction or transformation fails for a requisition that has never rendered successfully
- **THEN** the system responds with a `5xx` error and logs the failure

#### Scenario: Last-good survives restart
- **WHEN** the daemon restarts after having persisted a successful rendering for a requisition
- **THEN** the persisted last-good XML is available to serve without re-running extraction
