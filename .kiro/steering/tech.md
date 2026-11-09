---
inclusion: always
---
# Tech steering

- Go 1.24+, standard library only. Do not add a Go dependency without saying why first.
- UI source is `web/` (React, Vite, Tailwind, components from 21st.dev). `npm run build` writes
  `internal/web/dist`, which is committed and embedded in the binary. Do not hand-edit `dist`.
- One binary: `specter status | check | run | ingest | seal | explain | export | verify | audit | serve | key`.
- Storage is an append-only JSONL log with a SHA-256 hash chain plus content-addressed blobs.
  Never rewrite the log.
- Signing is Ed25519 over canonical JSON (`internal/canon`) with the domain prefix
  `specter-attest-v1`. The key file `.specter/key.ed25519` is never committed.
- The HTTP server must never execute commands (spec 4.1 has a test). Only the CLI runs tests.
- CSP: `script-src 'self'`; `style-src` allows inline because React and framer-motion set
  inline styles. No `innerHTML`, no remote assets, no remote fonts.

## Commands

```
go vet ./... && go test -race ./...
go run ./cmd/specter serve -demo          # http://127.0.0.1:8080, read-only, seeded drift
go run ./cmd/specter run -seal -- go test -json ./...
go run ./cmd/specter check                # exit 1 on drift
cd web && npm ci && npm run build
```

After editing a test or a requirement: run `specter run -seal -- go test -json ./...` so the
committed ledger matches, or CI's `specter check` fails (that is the feature working).
