# Ghost citations: tasks

| # | task | covers | test |
|---|---|---|---|
| 1 | immutable versions | G1, G2 | `engine.TestVersions` |
| 2 | record a citation with context | G3, G4 | `engine.TestCiteRejectsUnsupportedQuote`, `engine.TestFollowCiteLifecycle` |
| 3 | follow-mode relocation | G5, G8 | `engine.TestFollowCiteLifecycle` |
| 4 | pinned mode | G6 | `engine.TestPinnedCiteStaysPut` |
| 5 | pick by context | G7 | `engine.TestRepeatedPassagePicksByContext` |
| 6 | who haunts | G9 | `engine.TestHaunts`, `api.TestCiteEditReceiptTrail` |
| 7 | survives unrelated edits | G10 | `engine.TestPropCiteSurvivesUnrelatedEdits` |
| 8 | trail replay | G11 | `engine.TestTrailReplayShowsChange` |
