---
title: Capture TUI panics and harden the workflow-dispatch path
status: draft
issue: https://github.com/jongio/grut/issues/361
scope: P1
---

## Problem

Dispatching a GitHub Actions workflow from the GitHub panel (Workflows tab, `D`)
crashes the whole TUI with `run TUI: program was killed: program experienced a
panic`. The dispatch API call succeeds (the run appears on GitHub), but grut
exits.

Two problems compound:

1. **The panic is undiagnosable.** `cmd/root.go` creates the Bubble Tea program
   with panic-catching enabled and no forwarding, so Bubble Tea recovers the
   panic internally and returns a generic `ErrProgramKilled: ErrProgramPanic`.
   The top-level `recover()` in `main.go` never runs, `crashlog.WriteRecovery`
   is never called, and `DataDir()/crashes` stays empty. Because the observed
   error is the *wrapped* `ErrProgramKilled`, the panic is in the **main
   goroutine** — the model's `Update` or `View` — not a command goroutine.

2. **The dispatch path has latent defects.** `handleWorkflowDispatchInputs`
   calls `ghClient.DispatchWorkflow` with no `nil` guard (its sibling has one),
   and the inputs step feeds multi-line `key=value` content into a **single-line**
   `ModalInput`, which cannot even accept newlines (Enter submits), so a user
   can never enter more than the pre-filled lines.

## Approach

Fix (1) first — it is the durable, high-value fix and the fastest route to root
causing any residual crash.

- **Panic capture net.** Add `crashlog.GuardTUI(ctx)`: a deferred helper that
  `recover()`s an in-flight panic, writes a crash report (with the preserved
  stack and the package default log tail), then **re-panics** so Bubble Tea
  still performs its terminal restore. Defer it at the top of the root model's
  `Update`, `View`, and `Init`. `debug.Stack()` inside a deferred recover
  captures the original panic site (verified), so the report pinpoints the
  fault. This makes *every* future TUI panic diagnosable, not just this one.

- **Harden dispatch.** Guard `nil` GH client in `handleWorkflowDispatchInputs`
  (emit a failure toast rather than dereferencing nil). Switch the inputs modal
  to a multi-line composer (`ShowMultilineInputWithValue`) so its mode matches
  its multi-line content and users can actually add input lines.

## Trade-offs / decisions

- **Re-panic vs. `tea.WithoutCatchPanics()`.** Disabling Bubble Tea's catch
  would leave the terminal in raw mode after a crash (Bubble Tea's own docs warn
  this). Re-panicking from the model wrappers keeps Bubble Tea's terminal
  restore while still writing our crash report first. Chosen for correctness.
- **Inputs modal submit key changes** from Enter to Ctrl+D (standard multi-line
  composer semantics, shown in the modal hint). This is a UX *fix*: the old
  single-line modal could not accept newlines at all.

## Out of scope

- The exact real-data render fault (if any remains) is not independently
  reproducible without live API data; the capture net ensures it is now
  diagnosable, and the known nil/multi-line vectors are removed.

## Acceptance criteria

- TUI panics write a crash report under `DataDir()/crashes` with the stack.
- Dispatching a workflow does not crash the TUI.
- `handleWorkflowDispatchInputs` guards a nil client before `DispatchWorkflow`.
- The inputs modal uses a mode consistent with its multi-line content.
- A regression test drives the full dispatch flow and asserts no panic + toast.

<!-- Pipeline tracking (auto-managed, not part of product spec) -->
## Pipeline Status
Phase: CERTIFYING

- Phase 1 SCOPE/PLAN: done — P1, issue #361, test-plan written.
- Phase 2 BUILD: done — implementation + tests; build/vet/lint clean.
- Phase 3 VERIFY: done — full `go test ./...` green; test plan COVERED (0 gaps); code-review found no significant issues (validated re-panic safety, stack fidelity, no race, intact routing).
- Phase 4 CERTIFY: in progress — `mage preflight` (fmt/tidy/verify/vet/lint/build/test/race/vulncheck/gofumpt/deadcode/benchmarks). Individually verified clean: vet, full lint, gofumpt, deadcode, govulncheck, mod tidy.
- Known unrelated pre-existing issue (NOT this change): `cmd.TestRunStatus_NotARepo` fails when `GOTMPDIR` points inside the worktree (mage default `bin/.tmp`), because `t.TempDir()` then lands in a repo; reproduced identically on origin/main. Gates run with an external `GOTMPDIR` (a magefile-supported config).

