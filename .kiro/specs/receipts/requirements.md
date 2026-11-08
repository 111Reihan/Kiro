# Receipts: requirements

Introduction: a receipt is a signed statement of one check that anyone can verify later without
the server, the log or the author.

**C1** WHEN a receipt is issued, THE SYSTEM SHALL embed the quote, verdict, severity, matched excerpts, the source version's hash and the log event that recorded it.
**C2** THE SYSTEM SHALL sign the canonical JSON of the body with Ed25519 under a fixed domain prefix.
**C3** WHEN verifying, THE SYSTEM SHALL check the signature and SHALL re-grade the embedded quote against the embedded excerpts, and SHALL reject the receipt if the grade does not reproduce.
**C4** WHEN a source text is supplied, THE SYSTEM SHALL also check its hash and that each excerpt occurs in it.
**C5** WHEN a public key is pinned, THE SYSTEM SHALL reject a receipt signed by any other key.
**C6** THE verification report SHALL always list what was not checked: that the source is authentic, that the clock is right.
**C7** FOR ALL receipts and any change to any byte of the body, verification SHALL fail. (property)
**C8** Verification SHALL NOT need the store, the engine or network access.
