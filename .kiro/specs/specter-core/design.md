# Design: Specter core

```
requirements.md ─┐                      ┌─ specter check   (CI gate, exit 1 on drift)
tasks.md ────────┼─ internal/spec ─┐    ├─ specter serve   (read-only HTTP + UI)
*_test.go ───────┤  (scan, no I/O  ├─ internal/ledger ─ internal/store (hash chain + blobs)
go test -json ───┘   beyond reads) │   (status, seal,     internal/engine (versioned docs, key)
                                    │    audit, history)
                       internal/verify.Compare (word diff + severity)
```

- **Anchors are words, not line numbers.** A requirement is identified by `spec/local-id` and
  fingerprinted by its folded words (`WordHash`), so reflowing or recasing does not matter.
- **Status precedence:** failing > stale > orphaned > uncovered > unsealed > verified.
- **A seal is an Ed25519 signature** over canonical JSON (domain prefix `specter-attest-v1`)
  of: requirement text, linked tests with body hashes and results, requirement-file version,
  and the log head at signing time (the anchor).
- **What a seal does not prove:** that the tests are good, that the signer is who they say
  (pin the key), or that the clock is right.
- **The server never runs commands.** Only the CLI runs tests (`specter run`).
