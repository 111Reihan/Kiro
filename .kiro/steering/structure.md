---
inclusion: always
---
# Structure steering

```
cmd/revenant     CLI entry; thin, calls the packages below
internal/text    normalise, tokenise, extract quotes (no deps on other internal packages)
internal/align   word alignment and diff
internal/verify  grades a quote against text: verdict, severity, spans
internal/canon   canonical JSON for signing
internal/store   hash-chained log + blobs
internal/receipt sign and verify receipts (offline: needs nothing but the receipt)
internal/engine  documents, citations, trails; ties the above together
internal/api     HTTP JSON + UI; internal/mcp stdio MCP; internal/web embeds the UI
examples         public-domain sources and a sample draft
```

Imports point downward only: `api`, `mcp` and `cmd` may import `engine`; `engine` may import
`store`, `verify`, `receipt`; `verify` imports `align` and `text`. `receipt` must never import
`engine` or `store`, so verification stays possible without the original install.
