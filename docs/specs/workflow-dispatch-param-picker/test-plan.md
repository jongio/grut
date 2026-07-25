# Test Plan — Workflow-dispatch parameter picker and crash safety net

## Status: COVERED

## Planned Tests

| ID | AC | Description | Test | State |
|----|----|-------------|------|-------|
| T1 | AC1 | `choice` input → action picker of options, preselected on default | `TestHandleWorkflowInputsFetched_KnownInputsUsePicker` | automated |
| T2 | AC2 | `boolean` input → true/false picker, preselected on default | `TestHandleWorkflowInputsFetched_BooleanInputUsesPicker` | automated |
| T3 | AC3 | free-form input → text field pre-filled with default | `TestHandleWorkflowInputsFetched_FreeFormInputUsesTextField` | automated |
| T4 | AC1,AC3,AC6 | multi-input flow collects a picked choice + a custom text value and dispatches both | `TestWorkflowDispatch_MultiInput_CollectsChoiceAndText` | automated |
| T5 | AC4 | no declared inputs → dispatches directly, no prompt | `TestHandleWorkflowInputsFetched_NoInputsDispatchesDirectly` | automated |
| T6 | AC5 | unreadable inputs → free-form composer fallback (`opWorkflowDispatchRaw`) | `TestHandleWorkflowInputsFetched_UnknownInputsUseRawComposer`, `TestHandleWorkflowInputsFetched_NoInputs` | automated |
| T7 | AC6 | blank value omitted so workflow default applies | `TestWorkflowDispatch_EmptyValueKeepsDefault` | automated |
| T8 | AC7 | panic in a GitHub command goroutine → `ghCmdPanicMsg` (no crash) | `TestGuardedGitHubCmd_RecoversPanicToPanicMsg` | automated |
| T9 | AC7 | guarded command is transparent on the normal path | `TestGuardedGitHubCmd_PassesThroughNormalMsg` | automated |
| T10 | AC7 | `crashlog.CaptureCmdPanic` writes a report and records the last-crash path, quietly | `TestCaptureCmdPanic_WritesReportAndRecordsPath` | automated |
| T11 | AC8 | per-input dispatch path guards a nil client (no panic, error result) | `TestFireWorkflowDispatch_NilClientNoPanic` | automated |
| T12 | AC8 | free-form fallback path guards a nil client (no panic, error toast) | `TestHandleWorkflowDispatchRaw_NilClientNoPanic` | automated |
| T13 | AC1 | picker preselects the selected action's cursor, not always index 0 | `notify.TestShowActionPickerWithSelection_PreselectsCursor`, `notify.TestActionIndexByID` | automated |
| T14 | AC1..AC6 | full dispatch flow (D → ref → per-input → submit → result → toast) does not panic and dispatches once | `TestWorkflowDispatch_FullFlow_NoPanicSuccessToast` | automated |
| T15 | AC7 | post-dispatch render across sizes does not panic | `TestWorkflowDispatch_RenderAfterReload_NoPanic` | automated |
| T16 | AC7 | panic message clears stranded loading flags + shows error toast | `TestHandleGHCmdPanic_ClearsLoadingAndToasts` | automated |
| T17 | AC7 | concurrent command-goroutine panics each get a distinct crash file (no overwrite) | `TestCaptureCmdPanic_ConcurrentReportsAreDistinct` | automated |
| T18 | AC8 | representative guarded action (`cancelWorkflowRunCmd`) yields an error toast on nil client | `TestGuardedCommand_NilClientYieldsToast` | automated |
| T19 | AC1..AC3 | input prompt label building (description/name/required) | `TestWorkflowInputPrompt` | automated |

## Functionality Inventory (Phase 3 reconciliation)

Built from `git diff` of the changed source files. Every unit maps to a covering test.

| Unit | Covering test |
|------|---------------|
| `notify.ShowActionPickerWithSelection` (new constructor) | T13, and used by T1/T2/T4 |
| `notify.actionIndexByID` (helper) | T13 |
| `ShowModalMsg.SelectedID` + manager preselect (`actionCursor`) | T13 |
| `crashlog.CaptureCmdPanic` (quiet writer, records path) | T10 |
| `crashlog.Write` mutex + atomic-seq unique filename | T17 (distinct files, `-race` in CI) |
| `gitinfo.guardedGitHubCmd` (recover → `ghCmdPanicMsg`) | T8, T9 |
| `gitinfo.ghCmdPanicMsg` + `handleGHCmdPanic` (clear loading + toast) | T16 |
| `gitinfo.ghClientUnavailableCmd` (nil-client toast) | T11 (per-input), T12 (raw), T18 |
| `gitinfo.dispatchWorkflowCmd` (shared dispatch cmd + guards) | T4, T7 (fire path), T12 (raw path), T11 (nil client) |
| `handleWorkflowInputsFetched` (known/unknown/no-input branches) | T1, T3, T5, T6 |
| `promptNextWorkflowInput` (choice/boolean/text switch + fire) | T1, T2, T3, T4 |
| `fireWorkflowDispatch` (collect values, reset draft, delegate) | T4, T7, T11 |
| `handleWorkflowDispatchInputs` (record value, advance idx) | T4, T7, T14 |
| `handleWorkflowDispatchRaw` + `parseKeyValueInputs` | T6, T12 |
| `handleWorkflowDispatch` sets `inputsKnown` | T14 (`fetched.inputsKnown` asserted) |
| `workflowInputPrompt` / `optionsToActions` | T19; picker actions exercised by T1/T2/T4 |
| nil-client guards on 7 command funcs (cancel/merge/comment/reviewers/close/createPR/assignSelf) | T18 (representative) + shared `ghClientUnavailableCmd` (T11/T12); pre-existing per-command tests still pass |
| loader closures wrapped with `guardedGitHubCmd` (loadGitHubData/Meta/Issues/PRs/Actions/Workflows/Releases/Notifications) | T8/T9/T16 cover the wrapper + stranded-loading recovery; existing loader tests assert unchanged normal-path behavior |

**Gaps: 0.**

## Review findings triage (Phase 3)

- **code-review MEDIUM — crash-report filename collision + unsynchronized `Write`** (introduced by making `Write` concurrent/repeatable via `CaptureCmdPanic`): FIXED — `sync.Mutex` around `Write`/`pruneOld` + atomic-sequence unique filenames (T17).
- **rubber-duck #3 — panic recovery could strand a "Loading…" flag**: FIXED — `guardedGitHubCmd` now emits `ghCmdPanicMsg`, handled by `handleGHCmdPanic` which clears loading flags (T16).
- **rubber-duck #5** duplicates the crash-collision finding: FIXED (T17).
- **rubber-duck #1 (stale async / no request identity)**: NOT CHANGED — the dispatch flow is a strictly sequential, modal-gated flow; the async-fetch window and use of current-repo identity are pre-existing characteristics (the prior `pendingName`-based flow behaved identically), not a regression. A generation-ID system is over-engineering for a single-modal flow. Documented as out of scope.
- **rubber-duck #2 (`inputsKnown` cannot distinguish parse-failure from no-inputs)**: NOT CHANGED — `GetWorkflowInputs` returns `(nil,nil)` only for valid no-input cases and for YAML that fails to parse; a workflow listed by the API is GitHub-validated YAML, so the parse-failure branch is practically unreachable, and dispatching a genuinely-broken/no-trigger workflow fails fast at GitHub (422) rather than after a pointless composer. Acceptable.
- **rubber-duck #4 (blank/trim)**: NOT CHANGED — blank = "use the workflow default" is the intentional, documented design (AC6); choice/boolean pickers return non-empty IDs so omission never triggers for them; whitespace-padded options are pathological.
- **Self-finding (refactoring)**: duplicated dispatch-command construction extracted into `dispatchWorkflowCmd`.

