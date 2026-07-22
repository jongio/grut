# Test Plan — Capture TUI panics & harden workflow-dispatch

## Status: COVERED

Issue: https://github.com/jongio/grut/issues/361 · Scope: P1

## Planned Tests

| ID | AC | Description | Location | Status |
|----|----|-------------|----------|--------|
| T1 | #1 | `GuardTUI` writes a crash report to `DataDir()/crashes` when a deferred call recovers an in-flight panic, and re-panics (original value + stack preserved, `LastCrashPath` set). | crashlog/guardtui_test.go | automated |
| T2 | #1 | `GuardTUI` is a no-op (no report) when there is no panic. | crashlog/guardtui_test.go | automated |
| T3 | #1 | Root model `Update` panic is captured (crash file written) and re-panics. | tui/panic_capture_test.go | automated |
| T4 | #1 | Root model `View` panic is captured likewise. | tui/panic_capture_test.go | automated |
| T10 | #1 | Root model `Init` panic is captured likewise. | tui/panic_capture_test.go | automated |
| T5 | #3 | `handleWorkflowDispatchInputs` with a nil GH client does not panic and yields an error result → failure toast. | gitinfo/gitinfo_dispatch_panic_test.go | automated |
| T6 | #4 | `handleWorkflowInputsFetched` produces a `ModalMultilineInput` pre-filled with joined `key=value` lines. | gitinfo/gitinfo_dispatch_panic_test.go | automated |
| T7 | #4 | `ShowMultilineInputWithValue` yields `ShowModalMsg{Kind: ModalMultilineInput, Value}`; Ctrl+D submits the composed value. | notify/notify_test.go | automated |
| T8 | #2,#5 | Full dispatch flow (D → ref → inputs → Ctrl+D) dispatches once and emits a success toast, no panic. | gitinfo/gitinfo_dispatch_panic_test.go | automated |
| T9 | #2 | Panel `View` across sizes after the post-dispatch data reload asserts no panic. | gitinfo/gitinfo_dispatch_panic_test.go | automated |

## Functionality Inventory (reconciled against the diff)

| Unit of functionality | Covering test |
|-----------------------|---------------|
| `crashlog.GuardTUI` recover→write→re-panic | T1, T3, T4, T10 |
| `crashlog.GuardTUI` no-op path | T2 |
| `crashlog.LastCrashPath` / `lastCrashPath` | T1 |
| `tui.Model.Update` guard wiring | T3 |
| `tui.Model.View` guard wiring | T4 |
| `tui.Model.Init` guard wiring | T10 |
| `cmd/root.go` surfacing crash path after Run error | display-only, gated on tested `LastCrashPath()`; exercised manually, no logic branch beyond the nil-path check |
| nil-client guard in `handleWorkflowDispatchInputs` | T5 |
| `errGitHubClientUnavailable` sentinel + failure toast | T5 |
| `notify.ShowMultilineInputWithValue` | T7 |
| inputs modal uses multi-line composer | T6, T8 |
| full dispatch flow (no panic, success toast, dispatch args) | T8 |
| post-dispatch reload render | T9 |

No functionality GAP rows remain.

