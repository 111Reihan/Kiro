# Ghost citations: design

A `Cite` is `{doc, ver, start, end, exact, prefix, suffix, mode}`. It is an event in the log, so it
is itself tamper-evident.

## Re-locating a citation in a later version

1. Same range, same text: `intact`.
2. Search the new text for `exact`. If found, choose the match whose prefix and suffix overlap most
   with the stored context: `moved`.
3. Otherwise align `exact` against the new text. If similarity is above the drift threshold:
   `drifted`, with the diff. Below it: `orphaned`.

Order matters: the cheap exact checks first, so unchanged text is never reported as drifted
because a fuzzy score came out low.

## Trails

A trail is an ordered list of steps `{claim, quote, doc, ver, receipt}`. Adding a step seals a
receipt. Replay checks each signature, then re-grades the quote against the latest source and sets
`changed` when the verdict or the matched text differs from the sealed one. This is the Memex
trail made checkable: a path through documents that can tell you where it no longer holds.
