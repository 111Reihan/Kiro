# .kiro

Specs, steering and hooks for Specter, in the layout Kiro reads.

**Provenance, stated plainly.** The code in this repo was written with Claude Code during the
Kiroween 2026 submission period. These `.kiro/` files were drafted in that same session so the
project has a spec to build against, steering, and hooks. They were not produced by a Kiro
session. Anything Kiro itself did belongs in `docs/KIRO-LOG.md`, written by whoever ran it.

## The spec is checked by the tool

`specs/specter-core/requirements.md` is the contract. Every requirement in it is named in a
`// covers:` comment above a test in this repo, and the committed ledger in `.specter/` holds
signed seals for them. Run `go run ./cmd/specter check` to see them verified, or edit a number
in the requirements and run it again.

| path | what it is |
|---|---|
| `steering/product.md` | what Specter claims and refuses to claim |
| `steering/tech.md` | stack, commands, hard constraints |
| `steering/structure.md` | package map and import direction |
| `specs/specter-core/` | requirements (EARS), design, tasks |
| `hooks/*.kiro.hook` | `requirements-saved`, `tests-after-go-edit`, `ui-guardrails`, `spec-drift-check` |

No MCP server is registered: the MCP server is a task not yet done (see `tasks.md`).
