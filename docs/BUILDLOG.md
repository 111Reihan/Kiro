# Build log

Everything that went into this repo, in the order it was done, with the reason. Bugs that were
found and fixed along the way are listed too, because they changed the design.

Dates are 2026-11-07 and 2026-11-08, inside the Kiroween submission period (Oct 30 to Dec 4).
All work in this log was done with Claude Code. Kiro sessions, if any, are recorded separately
in [KIRO-LOG.md](KIRO-LOG.md).

## 0. The idea, and why it changed twice

First pitch: a notebook with live quotes (Xanadu transclusion). Reviewed honestly, it had no clear
user and looked like Obsidian block embeds. Second version: aim it at a real current problem,
quotes in AI-written text that are wrong or subtly altered. A design review of that version found
nine weaknesses; the ones that changed the build:

| weakness found | what was done about it |
|---|---|
| "fabricated" overclaims: a quote missing from your sources is not proven fake | verdicts are `verbatim`, `drifted`, `unsupported`, `no-source`; the word "fake" appears nowhere in the product |
| a similarity score hides meaning flips ("did" vs "did not" is 95% alike) | severity is decided by *which* words changed (negation, number, name, modal, quantifier), not by how many |
| a signed receipt sounds like it proves more than it does | every verification prints a "not checked" list; keys can be pinned |
| byte, UTF-16 and code point offsets disagree on accents and emoji | offsets are code points everywhere; a property test checks the folded-to-original mapping |
| anchors by position break when text moves | citations store context and are re-located: same place, then exact search by context, then fuzzy |
| "append-only" in a database is just a habit | hash-chained log, `audit` command, tamper tests |
| a server that fetches URLs is an SSRF risk | no URL fetching at all; sources are pasted or uploaded |
| "trails" were undefined | a trail is an ordered list of claims, each tied to a sealed receipt; replay re-checks them |

## 1. Core engine (commit "Add core engine")

Was already in the working directory, uncommitted, when this phase began. I read all of it,
built it, ran the tests with `-race`, and committed it without changes.

- `store`: JSONL log with a SHA-256 chain, content-addressed blobs, redaction tombstones, repair of a torn final line, a lock file so two processes cannot interleave writes.
- `text`: folding (case, quotes, dashes, spacing) with a map back to original offsets; quote extraction.
- `align`: word-level semi-global alignment and diff.
- `verify`: the four verdicts and three severities.
- `engine`: immutable document versions, citations that survive edits, "who haunts", trails.
- `receipt`, `canon`: Ed25519 over canonical JSON; offline verification that re-grades the quote.
- `prop`: a small seeded property-test driver, so there is no test dependency.

## 2. API, CLI, MCP, samples (commit "Add HTTP API, CLI, MCP server")

- `internal/api`: JSON endpoints for everything above. Guards: requests must carry a loopback `Host` (stops DNS-rebinding), POST bodies must be `application/json` (a cross-site form cannot send that), 4 MiB cap, unknown JSON fields rejected, CSP and `nosniff` on every response. Engine errors map to 400, 404, 409, 410, 422, and anything unexpected is logged and returned as a plain "internal error".
- `internal/mcp`: stdio JSON-RPC with six tools. stdout carries protocol messages only. A tool failure is returned as a result with `isError`, so the model can read it and recover; an unknown tool is a protocol error.
- `cmd/revenant`: `serve add check receipt verify-receipt audit key mcp version`. `check` exits 1 when any quote is not verbatim, so it works as a CI step or a git hook.
- `examples/`: public-domain Gettysburg Address and Declaration of Independence, and a draft essay with three exact, two altered and one invented quote.
- Two tiny accessors: `Store.Head`, `Engine.PubKey`.

## 3. Web UI (commit "Add candlelit web UI")

Design system chosen with the ui-ux-pro-max skill: editorial serif on a dark table, parchment sheet
for the draft, ember for actions. System fonts only (the CSP forbids remote ones).

- Verdicts are never colour alone: an underline style (solid, wavy, struck, dotted), an icon, and a text label, all agreeing.
- Keyboard: every quotation is a focusable button, Enter or Space selects it. Visible focus ring, 44px targets, skip link, `aria-live` regions, reduced-motion respected, usable at 375px.
- No `innerHTML` anywhere; all server text goes in as text nodes. A test loads a source containing `<script>` and an `<img onerror>` title and checks they stay inert.
- Added `GET /api/events` so the Ledger view can draw the chain without shipping payloads.

Checked in headless Chromium against the real binary: 14 end-to-end steps (cite, seal, trail, edit
the source, see the drift, who-haunts, replay, audit, verify, tamper, XSS, reduced motion,
keyboard), zero console or CSP errors, no horizontal scroll at 375px or 1360px.

### Bugs found while testing the UI

1. **Load sample showed nothing.** `run()` read the textarea, which was still empty, and overwrote the sample text. Fixed by filling the textarea first.
2. **The word diff printed `[object HTMLParagraphElement]`.** `diffView` returned an array, and DOM `append` turns arrays into text. It now returns a fragment; two other call sites that passed arrays to `replaceChildren` were spread.
3. **A toast about the last page lingered on the next.** Cleared on navigation.
4. **A test that proved nothing.** The new events test declared `Hash, Prev string \`json:"hash"\``. Two fields with one tag are both dropped by `encoding/json`, so every value was empty and the chain assertion passed vacuously. `go vet` caught it. Fixed, and the test now also requires at least 6 events.

## 4. `.kiro/`, docs, CI (this commit)

- `.kiro/steering`: product (what may and may not be claimed), tech (stdlib only, CSP rules), structure (who may import whom).
- `.kiro/specs`: three specs in EARS form with design and a task table. Every task row names a test; a script checked that all of them exist.
- `.kiro/hooks`: check quotes when a markdown file is saved; vet and test after Go edits; a UI guardrail (no `innerHTML`, no remote URLs); a manual spec-drift check.
- `.kiro/settings/mcp.json`: registers the MCP server.
- Spec wording was corrected against the code after review (the name rule is "one capitalised word swapped for another", not "any capital").
- `README.md`, `docs/DEMO.md` (3-minute script), `docs/KIRO-LOG.md` (empty template), `.github/workflows/ci.yml`.

## What was not done

- The `.kiro/` files were written outside Kiro and have not been run in Kiro. The hooks' `askAgent` prompts and the MCP registration are untested there.
- No third-party timestamping of receipts.
- No URL fetching, accounts, or multi-user mode (left out on purpose).
- Quote finding only: paraphrases and unquoted claims are not detected.
- English-only list of meaning-changing words.
- The demo video, the Devpost write-up, the blog post and the social post are not done and are the owner's to make.
