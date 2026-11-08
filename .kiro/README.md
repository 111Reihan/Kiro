# .kiro

Specs, steering, hooks and MCP config for Revenant, in the layout Kiro reads.

**Provenance, stated plainly.** The Go code in this repo was written with Claude Code in
the first days of the Kiroween 2026 submission period. These `.kiro/` files were drafted in
that same session so the project has a spec to build against, a steering baseline, and
working hooks. They were not produced by a Kiro session. Anything Kiro itself did (its
generated task runs, hook triggers, MCP calls) belongs in `docs/KIRO-LOG.md`, written by
whoever ran it. Do not read this folder as a record of Kiro usage; read that file.

## What is here

| path | what it is |
|---|---|
| `steering/product.md` | what Revenant is for, the vocabulary, what it refuses to claim |
| `steering/tech.md` | stack, commands, hard constraints (stdlib only, no innerHTML, CSP) |
| `steering/structure.md` | package map and who may import whom |
| `specs/quote-verification/` | requirements (EARS), design, tasks for grading quotes |
| `specs/ghost-citations/` | citations that survive source edits |
| `specs/receipts/` | signed receipts and the offline verifier |
| `hooks/*.kiro.hook` | agent hooks (see below) |
| `settings/mcp.json` | registers the `revenant` MCP server for Kiro |

## Hooks

- `check-quotes-on-save`: when a markdown file is saved, run the quote checker and report anything that is not verbatim.
- `tests-after-go-edit`: when Go source is saved, run `go vet` and the tests for that package.
- `ui-guardrails`: when the UI files change, check for `innerHTML` and remote URLs, which the CSP would block.
- `spec-drift-check`: manual button; compare each spec's requirements to the tests that claim to cover them.
