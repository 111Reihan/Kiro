# Specter

**Your tests are green. Do they still prove what the requirement says?**

Specter links each requirement to the tests that prove it, anchored to the requirement's exact
words. Change "within 2 seconds" to "within 5 seconds" and every proof of the old sentence goes
stale, with the change shown as a ghost diff and flagged **material**. When a requirement is
proved, Specter signs an attestation (Ed25519, on a hash-chained log) that anyone can verify
without trusting your machine.

Built for Kiroween 2026, category **Resurrection**. It works on Kiro's own `requirements.md` /
`tasks.md` layout, so spec-driven projects need no new format.

## The dead idea

Requirements traceability (DOORS, 1991 onward) and Xanadu's links that follow a passage when
the text around it moves. Both lost to "just use ticket ids", and spec-driven AI development
brought the problem back: an agent edits code and tests all day, and the spec file is the only
contract. Specter resurrects the idea with one change: the link is to the *words*, not the id.

**Honest prior art.** Doorstop, DOORS, Jama and Tracey trace requirements to tests by id.
Specter's additions are word-level drift detection with a severity, orphan recovery (a
requirement that moved is found by its words), and signed, offline-verifiable evidence. It does
not judge whether a test is any good.

## Try it

Needs Go 1.24+. No other Go dependencies.

```
go run ./cmd/specter serve -demo      # http://127.0.0.1:8080, read-only, seeded with drift
```

Open the **Requirements** tab, click `checkout/1.1`, and edit the "What if the words changed?"
box. The server answers what would go stale; nothing is saved.

On your own project:

```
specter status                         # where every requirement stands
specter run -seal -- go test -json ./...   # run tests, store the run, sign what passed
specter check                          # exit 1 on drift; use it in CI
specter explain checkout/1.1           # the ghost diff
specter export checkout/1.1 > a.json
specter verify a.json -key PUBKEY -log .specter/specter.log
specter serve                          # the UI, with Rescan and Seal
```

## How it links

- **Requirements**: Kiro `### Requirement N` + numbered items (`spec/N.k`), `**R1** text`, or `REQ-1: text`.
- **Tests**: a comment above (or just inside) a test names what it covers, in Go, Python, JS/TS, Java/Kotlin, Rust:

  ```go
  // covers: checkout/1.1, checkout/2.3
  func TestConfirmLatency(t *testing.T) { ... }
  ```
- **Results**: `go test -json` or JUnit XML.
- **Tasks**: Kiro's `_Requirements: 1.1_` lines show up in each requirement's detail.

Status, worst first: **failing** (a covering test failed), **stale** (words or proving tests
changed since the seal), **orphaned** (sealed, then gone; names where the words went),
**uncovered**, **unsealed**, **verified**.

## What a seal proves, and what it does not

It proves a key signed these exact words, these tests (by code hash) and these results at a log
position, and that the log has not been edited since. It does **not** prove the tests are good,
who holds the key (pin it with `verify -key`), or that the clock is right.

## Specter checks itself

`.kiro/specs/specter-core/requirements.md` has 15 requirements. Each is named in a `// covers:`
comment above a test here, and `.specter/` holds the committed, signed ledger. CI runs
`specter check`, so editing a requirement or its test without re-sealing fails the build.

## Safety

The HTTP server reads the project tree and the log; it never runs a command (a test enforces
that it cannot import `os/exec`). `-readonly` and `-demo` refuse writes. Bound to loopback it
rejects other Host headers. Bodies are capped at 1 MiB, JSON is strict.

## Layout

See [.kiro/steering/structure.md](.kiro/steering/structure.md). UI source is `web/` (React,
Tailwind, components from [21st.dev](https://21st.dev)); the built UI in `internal/web/dist` is
committed and embedded. Build log: [docs/BUILDLOG.md](docs/BUILDLOG.md). Kiro usage:
[docs/KIRO-LOG.md](docs/KIRO-LOG.md). License: MIT.
