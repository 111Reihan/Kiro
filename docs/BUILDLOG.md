# Build log

Plain account of how this was made. Dates are the submission period (Oct 30 to Dec 4, 2026).

## Who wrote what

The code was written with Claude Code. The `.kiro/` files were drafted in that session, not in
Kiro. Kiro usage, if any, is recorded by its user in [KIRO-LOG.md](KIRO-LOG.md).

## History

1. **Revenant** (git history before the Specter commit): a quote-integrity checker. Kept the
   hash-chained store, canonical JSON, word-level alignment, Ed25519 signing and the severity
   rules. Dropped the product: it was a citation checker with prior art, not a new idea.
2. **Specter**: the pivot. Reused about 80% of the backend. New: `internal/spec` (scan),
   `internal/ledger` (status, seals, audit, history), CLI, API, demo project, and a new UI.

## Decisions

- Anchor to words, not ids or line numbers; fingerprint is the folded word list.
- `verify.Compare` treats case and punctuation as no change, so reflowing never goes stale.
- A seal requires every covering test to have passed in the latest stored run.
- The server cannot run commands. A what-if preview edits an in-memory copy and writes nothing,
  which is what makes a public read-only demo safe.
- The UI uses two 21st.dev components (animated stat card, card-frame table + badge). Its
  free retrieval quota (2 a day) ran out, so the side sheet is hand-written in the same style.

## Bugs found while building

- Go's `regexp` has no backreferences; the JS test-name regexp panicked at start-up.
- A `covers:` note under a one-line test attached to the wrong test. Fixed: notes inside a test
  must be indented deeper than its declaration.
- Fixture strings inside Go test files were read as real tests. Fixed: backtick strings skipped.
- The drawer was unusable: it sat inside `#root`, which is made inert while it is open. Found
  by screenshot test; fixed with a portal.
- A case-only edit made a requirement stale. Fixed in `verify.Compare`.

## Not done

- MCP server for Kiro (`tasks.md` item 7).
- Docker image and macOS/Windows binaries have not been built or run.
- Only Go test files are dogfooded; the other language scanners are covered by unit tests only.
- Scanning is a full walk per request; fine for repos in the thousands of files, not millions.
