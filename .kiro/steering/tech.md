---
inclusion: always
---
# Tech steering

- Go 1.24+, standard library only. Do not add a dependency; if you think you need one, say why first.
- One binary: `revenant serve | add | check | receipt | verify-receipt | audit | key | mcp`.
- Storage is an append-only JSONL log with a SHA-256 hash chain plus content-addressed blobs.
  Never rewrite the log. Redaction writes a tombstone event and deletes the blob.
- Offsets are Unicode code points (runes), never bytes or UTF-16 units. Text is NFC-folded the
  same way everywhere (`internal/text`).
- Signing is Ed25519 over canonical JSON (`internal/canon`) with a domain prefix.

## Commands

```
go vet ./... && go test -race ./...
go run ./cmd/revenant serve            # http://127.0.0.1:8080
go run ./cmd/revenant check examples/report.md
```

## UI rules (the CSP enforces these)

- No inline `<script>`, no inline `style=`, no remote fonts, images or scripts.
- Never use `innerHTML`. Build nodes with the `h()` helper and `textContent`.
- Verdicts are never colour alone: mark + icon + text label.
- Respect `prefers-reduced-motion`. Targets are at least 44px. It must work at 375px wide.

## Tests

Every grading rule gets a property test (`internal/prop`) as well as examples. A new verdict
rule without a property test is not done.
