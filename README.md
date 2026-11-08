# Revenant

Check every quotation in a draft against the source it claims to quote, and keep the evidence.

Paste a draft. Revenant finds each passage in double quotes, looks for it in the sources you
loaded, and says one of four things: **verbatim**, **drifted** (with a word-by-word diff),
**unsupported** (not in any loaded source), or **no-source** (nothing loaded to check against).
A change that can flip the meaning, like "shall not" to "shall never", is marked **material**
no matter how close the rest of the sentence is.

Built for the Kiroween 2026 hackathon, category **Resurrection**.

## The dead ideas

**Project Xanadu** (Ted Nelson, 1960) wanted quotes to be references to the source, links that work
in both directions, and versions that never disappear. The web kept none of that. In Revenant a
citation points at a span of a source *version*, so when the source is edited the citation says
what happened to its words (`intact`, `moved`, `drifted`, `orphaned`), and you can ask any
passage who quotes it.

**Memex trails** (Vannevar Bush, 1945) were saved paths through documents. A Revenant trail is a
path from each claim to the exact source version behind it. Replaying a trail re-checks every
step and shows where the sources no longer say what they said.

## Run it

Needs Go 1.24 or newer. No other dependencies.

```
go run ./cmd/revenant serve
```

Open http://127.0.0.1:8080, press **Load sample**. That loads the Gettysburg Address and the
Declaration of Independence (public domain) and checks a draft essay with six quotes in it:
three exact, two altered, one invented.

Command line:

```
revenant add gettysburg examples/sources/gettysburg.txt
revenant check examples/report.md           # exit 1 if any quote is not verbatim
revenant receipt -doc gettysburg "Four score and seven years ago" > receipt.json
revenant verify-receipt -source examples/sources/gettysburg.txt receipt.json
revenant audit                              # verify the hash chain and every stored blob
```

Data lives in `./data` (override with `-data` or `REVENANT_DATA`). It holds the log, the source
texts and your signing key. The key is private; `data/` is in `.gitignore`.

## Try it without installing Go

Prebuilt binaries for Windows, macOS and Linux are in the release zip (`RUN-ME.txt` inside).
`revenant serve -demo` loads the samples, so the first page already has something to check.

For a public URL, see [docs/DEPLOY.md](docs/DEPLOY.md). A public instance runs with `-readonly`:
visitors can check text and verify receipts, but cannot add sources, cite, seal or build trails.

## Receipts

`revenant receipt` signs a statement of one check with Ed25519. Anyone can verify it later with
`revenant verify-receipt`, with no server and no copy of your data: it checks the signature and
re-grades the quote against the excerpts inside the receipt. If you also give it the source file,
it checks that too.

What a receipt proves: this key saw that version of that source contain (or not contain) the
quote. What it does not prove: that the source is the genuine document, or that the time on it
is true. Every verification prints what it did not check. To tie a receipt to a person, pin
their public key (`-pin`) that you got from them some other way.

## Use it from a coding agent

`revenant mcp` serves six tools over MCP (`check_text`, `check_quote`, `who_haunts`,
`issue_receipt`, `list_sources`, `add_source`). `.kiro/settings/mcp.json` registers it for Kiro,
and a hook there checks quotes in a markdown file whenever you save it.

## How it works

- `internal/store`: append-only log, each entry carries the hash of the one before it; source
  texts are stored by SHA-256. `revenant audit` walks the chain and rehashes every blob.
- `internal/text`, `internal/align`, `internal/verify`: fold case, quote marks, dashes and spacing,
  align the quote against the source word by word, grade it, and decide severity from *which*
  words changed rather than how many.
- `internal/engine`: documents with immutable versions, citations that are re-located after
  edits, trails.
- `internal/receipt`, `internal/canon`: canonical JSON and signatures.
- `internal/api`, `internal/mcp`, `internal/web`: HTTP, MCP and the browser UI, all in one binary.

Go standard library only. Offsets are Unicode code points everywhere. Tests include property
tests (a seeded generator in `internal/prop`): any span of a source verifies as verbatim; changing
a word never does; inserting a negation is always material; any change to a receipt breaks it.

```
go vet ./... && go test -race ./...
```

## Limits

- It finds **quotations**. It does not catch paraphrases or claims with no quote marks.
- `unsupported` means the passage is not in the sources you loaded. The real source may simply
  not be loaded. Nothing here proves a quote is false.
- The list of words that make a change "material" is English only, and a changed word outside the
  list (a noun swapped for another noun) is graded `minor`.
- The server has no accounts. By default it listens on loopback only and refuses other Host
  names. If you bind another address it warns you: there is no login.
- Receipts are not timestamped by a third party. The `issued` time is the signer's own clock.

## Docs

- [docs/BUILDLOG.md](docs/BUILDLOG.md): what was built, in order, and why
- [docs/DEMO.md](docs/DEMO.md): the three-minute demo script
- [docs/KIRO-LOG.md](docs/KIRO-LOG.md): the record of real Kiro sessions (to be filled in by the people who ran them)
- [.kiro/](.kiro/): specs, steering, hooks, MCP config

## License

MIT, see [LICENSE](LICENSE). The two sample sources are public domain.
