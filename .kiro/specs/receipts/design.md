# Receipts: design

`receipt.Envelope{body, alg, key, pub, sig}`. `sig` covers `domain || canon(body)`. `canon` is a
small canonical JSON writer (sorted keys, no whitespace, fixed number format), so the same body
always signs to the same bytes on any machine.

## What a receipt does and does not prove

Proves: a holder of this key saw version `blob` of document `doc` and found this quote graded this way.
Does not prove: that `blob` is the real document, that `issued` is the real time, or that the key
belongs to anyone in particular. Pinning a key you got from the signer by another route is how you
bind it to a person. Each verification report states this in `not_checked`.

## Re-grading

The receipt carries only the excerpts the grade depended on, not the whole source. `receipt.Verify`
runs the same `verify.Check` on the quote against those excerpts and demands the same verdict,
severity and spans. A forged "verbatim" with a doctored excerpt then fails against `-source`.

## Layering

`receipt` imports `verify`, `align`, `canon` and nothing from `store` or `engine`, so it can be
vendored into a tool that only checks receipts.
