---
inclusion: always
---
# Structure

```
cmd/specter        CLI and server wiring
internal/spec      scan requirements, tasks, tests, results (reads only)
internal/ledger    status model, seals, attestations, history, audit
internal/engine    versioned documents + signing key
internal/store     hash-chained log + blobs
internal/verify    Compare: word diff and severity (the "ghost diff")
internal/align     word-level edit distance      internal/text   folding, tokens
internal/canon     canonical JSON                internal/prop   tiny property-test driver
internal/api       HTTP handlers                 internal/web    embedded built UI
internal/demo      seeded project with drift for the public instance
web/               UI source
```

Imports flow downward: `api` -> `ledger` -> `engine` -> `store`; `spec` imports nothing local
except `text`. `spec` never writes. `api` never imports `os/exec`.
