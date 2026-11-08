# Three-minute demo

Record on the machine that runs it (a laptop browser is fine). Under 3:00, public on YouTube, no
music you do not own. Use the bundled public-domain sources so nothing on screen is copyrighted.

Setup: `go run ./cmd/revenant serve` with an empty `data/` folder. Browser at 1280 wide.

| time | on screen | say |
|---|---|---|
| 0:00 | the draft essay (examples/report.md) in an editor | "This essay was written by an AI assistant. Every quote looks right. Are they?" |
| 0:15 | Séance, press **Load sample** | "Revenant checks each quotation against the real source." |
| 0:25 | the sheet: three verbatim marks, one wavy, one struck through, one wavy | "Three are exact. One has been invented. Two have been changed." |
| 0:40 | click the struck quote, then the banner | "Unsupported means not in the sources I loaded. It does not claim the quote is false." |
| 0:55 | click the heavy wavy quote, show `never` / `not` diff | "The change from *not* to *never* is marked material. A similarity score alone would call this 94% fine." |
| 1:15 | Cite this passage on a verbatim quote | "A citation is a pointer to a span of one version of the source. Not a copy." |
| 1:30 | Archive, write a new version (change "fathers" to "forefathers") | "Now someone edits the source." |
| 1:45 | the citation turns Drifted, ghost diff shows | "The old wording is still there, with what changed. This is Xanadu's quote-by-reference, from 1960." |
| 2:00 | select text in the archive, press **Who haunts this passage?** | "And every passage knows who quotes it." |
| 2:10 | Séance, seal a receipt, then Ledger, paste it in, **Verify offline** | "Every check can be sealed. Anyone can verify it with no server and no copy of my data." |
| 2:25 | edit one word of the receipt, verify again, it fails | "Change one byte and it breaks." |
| 2:35 | Ledger: **Verify the whole chain** | "Underneath, everything is an append-only hash chain." |
| 2:42 | terminal: save a markdown file in Kiro, hook output | "In Kiro, a hook runs this check every time I save a draft." |
| 2:52 | title card: Revenant, what it is for | "Revenant: quotes that remember their source." |

Do not show the `.kiro` hook working unless it did work on your machine. If the hook did not
fire in Kiro when you tried it, cut that row and say nothing about it.
