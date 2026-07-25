---
title: Workflow-dispatch parameter picker and command-goroutine crash safety net
status: draft
issue: pending
scope: P1
---

## Problem

Dispatching a GitHub Actions workflow from the GitHub panel had two shortcomings:

1. **Parameter entry was a raw text blob.** Every `workflow_dispatch` input —
   including `choice` and `boolean` inputs that have a fixed set of valid
   values — was entered as free-form `key=value` lines in a single multi-line
   composer. Users had to know the valid options and type them exactly, with no
   guidance and no protection against invalid values.

2. **grut still crashed after a dispatch.** Even after PR #362 added
   `crashlog.GuardTUI`, dispatching could still kill the whole TUI with
   `program was killed: program experienced a panic`, leaving an empty crashes
   directory. `GuardTUI` only guards the model's `Init`/`Update`/`View` on the
   main goroutine; a panic inside a `tea.Cmd` goroutine (run by Bubble Tea's
   `execBatchMsg`) is converted by `recoverFromGoPanic` into a fatal
   `ErrProgramPanic` that no crash report captures. Historical
   `bubbletea-panic-*.log` stacks pinpointed nil-client dereferences in the
   GitHub data loaders (`loadGitHubMeta`, `loadIssuesPage`, `loadReleasesPage`)
   running in those goroutines.

## Approach

- **Per-input prompting.** After the ref is chosen, prompt once per declared
  input. `choice` and `boolean` inputs are shown as an action picker of their
  valid values, preselected on the workflow's declared default. Free-form
  inputs (`string`, `environment`, unspecified) use a text field pre-filled
  with the default, so the user can accept it or enter a custom value. A
  workflow with no inputs dispatches immediately. If the workflow YAML cannot
  be read, fall back to the free-form `key=value` composer.

- **Command-goroutine crash safety net.** Add `crashlog.CaptureCmdPanic`, a
  quiet crash-report writer (no stderr output, since the TUI still owns the
  terminal), and `gitinfo.guardedGitHubCmd`, which wraps a GitHub command
  closure so a panic becomes a crash report plus an error toast instead of
  killing the TUI. Wrap every GitHub data loader and the dispatch closures.

- **Close the remaining nil-client gap.** Seven command functions
  (`cancelWorkflowRunCmd`, `mergePRCmd`, `commentCmd`, `requestReviewersCmd`,
  `closeReopenIssueCmd`, `createPRCmd`, `assignSelfCmd`) still dereferenced a
  nil client in their goroutine. Guard each with an explicit failure toast,
  matching the loaders fixed in 659cc28.

## Trade-offs / decisions

- **Picker vs. custom value for `choice`.** GitHub rejects a `choice` value that
  is not one of the declared options, so `choice`/`boolean` inputs are
  picker-only (no custom entry); only free-form inputs allow a custom value.
  This matches GitHub's own dispatch semantics.
- **Recover-and-toast vs. re-panic in command goroutines.** `GuardTUI`
  re-panics so Bubble Tea can restore the terminal; a command goroutine must
  instead recover *without* re-panicking, because Bubble Tea's
  `recoverFromGoPanic` turns any re-panic into a fatal program kill. So
  `guardedGitHubCmd` recovers, writes a report, and returns an error toast.
- **Empty input value keeps the default.** A blank value is omitted from the
  dispatch payload so GitHub applies the workflow's declared default rather
  than an empty override.

## Out of scope

- `environment`-type inputs are not populated from the repository's configured
  environments (they use a free-form text field); fetching environments is a
  separate enhancement.

## Acceptance criteria

- AC1: A `choice` input is presented as a picker of its options, preselected on
  the declared default.
- AC2: A `boolean` input is presented as a true/false picker, preselected on
  the declared default.
- AC3: A free-form input is a text field pre-filled with the declared default
  and accepts a custom value.
- AC4: A workflow with no declared inputs dispatches without prompting.
- AC5: When the workflow inputs cannot be read, the free-form `key=value`
  composer is offered as a fallback.
- AC6: A blank input value is omitted so the workflow default is applied.
- AC7: A panic inside a GitHub command goroutine is captured to a crash report
  and surfaced as an error toast instead of killing the TUI.
- AC8: The seven previously-unguarded GitHub command functions surface a nil
  client as a failure toast rather than crashing.

<!-- Pipeline tracking (auto-managed, not part of product spec) -->
## Pipeline Status
Phase: SHIPPING (late entry via `go verify`)

Certification (Phase 4): `go build`, `go vet ./...`, `golangci-lint ./...` (0 issues),
`gofumpt -l .` (clean), `deadcode ./...` (0 new; all 210 findings pre-existing/allowlisted),
`govulncheck ./...` (no vulnerabilities), full `go test ./...` (exit 0). Race/WSL/benchmark
steps of `mage preflight` are CI-covered (need cgo/WSL unavailable locally). doc-check: astro
GitHub-workflows page + CHANGELOG updated; keybindings unchanged (up-to-date test passes).
Review: code-review (1 MEDIUM, fixed) + rubber-duck (crash-collision + stranded-loading fixed;
async-identity/inputsKnown/blank-trim assessed as pre-existing or intentional — see test-plan
triage). Test plan COVERED, 0 gaps.
