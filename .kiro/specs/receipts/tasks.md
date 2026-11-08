# Receipts: tasks

| # | task | covers | test |
|---|---|---|---|
| 1 | canonical JSON | C2 | `receipt.TestRoundTrip` |
| 2 | issue and verify | C1, C3 | `engine.TestReceiptEndToEnd`, `receipt.TestRoundTrip` |
| 3 | grade must reproduce | C3 | `receipt.TestGradingMustReproduce` |
| 4 | source check | C4 | `receipt.TestSourceCheck` |
| 5 | pinning | C5 | `api.TestCiteEditReceiptTrail`, `revenant.TestCommandLineFlow` |
| 6 | not-checked list | C6 | `receipt.TestReportSaysWhatIsNotProved` |
| 7 | tamper property | C7 | `receipt.TestPropTamperFails` |
| 8 | log integrity under the receipts | C1 | `store.TestChainDetectsEditDeleteSwap`, `store.TestBlobTamperDetected` |
