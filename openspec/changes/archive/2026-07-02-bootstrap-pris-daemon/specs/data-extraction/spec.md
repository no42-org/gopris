## ADDED Requirements

### Requirement: Canonical record representation
Every extractor SHALL emit data as a stream of canonical records, where a record is a set of named fields. The transform layer SHALL consume records identically regardless of which extractor produced them.

#### Scenario: Built-in and external sources produce the same shape
- **WHEN** a CSV extractor, a SQL extractor, and an external subprocess extractor each run
- **THEN** each emits the same canonical record shape, and the transform layer processes them through one identical code path

### Requirement: Built-in SQL extractor
The system SHALL provide a built-in extractor that runs a configured SQL query against a configured database connection and maps each result row to one canonical record whose fields are the selected columns.

#### Scenario: Query returns rows
- **WHEN** the SQL extractor runs a configured query that returns rows
- **THEN** it emits one canonical record per row, with a field per selected column

### Requirement: Built-in CSV extractor
The system SHALL provide a built-in extractor that reads a configured CSV file and maps each data row to one canonical record whose fields are named by the CSV header.

#### Scenario: CSV with header row
- **WHEN** the CSV extractor reads a file with a header row and N data rows
- **THEN** it emits N canonical records, each field named by the corresponding header column

### Requirement: Built-in XLS extractor
The system SHALL provide a built-in extractor that reads a configured spreadsheet (`.xls`/`.xlsx`), using configuration to select the sheet and header row, and maps each data row to one canonical record. Cell values SHALL be coerced to record field values per the configured rules.

#### Scenario: Spreadsheet with selectable sheet and header row
- **WHEN** the XLS extractor is configured with a target sheet and header row and reads the file
- **THEN** it emits one canonical record per data row on that sheet, with fields named by the header row

### Requirement: External subprocess extractor contract
The system SHALL provide an extractor that runs an external executable as a subprocess and reads canonical records from it, enabling data sources written in any language without modifying the daemon. The contract SHALL be: JSON parameters on the subprocess stdin; newline-delimited JSON (NDJSON) canonical records on stdout; log output on stderr; exit code 0 for success and non-zero for failure; a configurable execution timeout; and the requisition's configuration directory as the working directory.

#### Scenario: Subprocess emits NDJSON records
- **WHEN** the configured executable exits 0 after writing NDJSON records to stdout
- **THEN** the system parses each line into a canonical record and passes the records to the transform layer

#### Scenario: Subprocess receives its parameters on stdin
- **WHEN** the external extractor runs
- **THEN** the executable receives the source's configuration block as JSON on stdin and is executed with the requisition's configuration directory as its working directory

#### Scenario: Subprocess fails
- **WHEN** the configured executable exits non-zero or exceeds the configured timeout
- **THEN** extraction is treated as failed and the daemon applies its serve-last-good behavior

### Requirement: Secret interpolation in source configuration
Source configuration SHALL support interpolation of secrets from environment variables or referenced secret files so that credentials are not stored as plaintext in shared configuration.

#### Scenario: Environment variable referenced in config
- **WHEN** a source configuration value references an environment variable placeholder and the daemon loads that source
- **THEN** the placeholder is replaced with the environment variable's value before the extractor uses it
