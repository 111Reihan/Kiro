# Requirements: Specter core

Specter links each requirement to the tests that prove it and tells you when either side drifts.
These requirements are checked by Specter itself: each one is named in a `// covers:` comment
above a test in this repo. Run `specter check` to see where they stand.

### Requirement 1: Reading a project

**User story:** As a developer, I want Specter to read the requirements and tests I already have, so that I do not rewrite them in a new format.

1. WHEN a project directory is scanned THEN the system SHALL read requirements from Kiro requirements.md files, bold-id lines such as **R1**, and REQ-1 lines.
2. WHEN a test carries a covers comment THEN the system SHALL link the test to the named requirements, and SHALL NOT resolve an id that matches more than one requirement.
3. WHEN test results are ingested THEN the system SHALL accept go test -json output and JUnit XML.

### Requirement 2: Finding drift

**User story:** As a reviewer, I want to know when a requirement and its proof stop agreeing, so that I do not trust a green test for a requirement that has changed.

1. WHEN the wording of a sealed requirement changes beyond case and punctuation THEN the system SHALL mark the requirement stale.
2. IF a wording change touches a number, a negation, a modal verb or a quantifier THEN the system SHALL classify the change as material.
3. WHEN a proving test is edited or loses its covers comment THEN the system SHALL mark the requirement stale.
4. WHEN a covering test fails in the latest run THEN the system SHALL mark the requirement failing, ahead of stale.
5. WHEN a sealed requirement no longer exists THEN the system SHALL report it as orphaned and name the requirement that now holds its words.

### Requirement 3: Evidence

**User story:** As an auditor, I want signed evidence that a requirement was proved, so that I can check it without trusting the machine that made it.

1. The system SHALL seal only requirements whose covering tests all passed in the latest run.
2. WHEN a requirement is sealed THEN the system SHALL sign its exact words, its tests and its log position with Ed25519.
3. IF a sealed attestation is altered THEN verification SHALL fail.
4. IF any line of the log is edited or removed THEN the audit SHALL fail.

### Requirement 4: A safe server

**User story:** As an operator of a public demo, I want the server unable to change or run anything, so that exposing it is low risk.

1. The server SHALL NOT execute any command.
2. WHILE the server is read-only it SHALL refuse writes and SHALL answer what-if previews without writing.
3. WHILE the server is bound to a loopback address it SHALL answer only localhost Host headers.
