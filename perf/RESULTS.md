# Performance Initiative — Final Results

## Test Environment

| Spec | Value |
|------|-------|
| CPU | Intel Core i9-13900K (24C/32T) |
| RAM | 128 GB DDR5 |
| OS (native) | Windows 11 Pro (windows-amd64) |
| OS (Linux) | Ubuntu via WSL2 (linux-amd64) |
| Go | 1.26.x (see go.mod for exact version) |

---

Branch: `perf/go-performance`
Baseline: `origin/main` (pre-optimization, `perf/bench_before.txt`, 5 runs)
Optimized: current branch (`perf/baselines/windows-amd64/main.txt`, 6 runs)
Hardware: see Test Environment section above

---

## What Was Changed

| Change | Files | Technique |
|--------|-------|-----------|
| Struct alignment | 70+ files | `betteralign -apply ./...` — 800+ bytes of padding eliminated |
| Slice backing array reuse | `internal/panels/gitdiff/gitdiff.go` | `lines = lines[:0]` reset instead of `nil` — retains capacity |
| String builder in render loops | `internal/tui/app.go` | `strings.Builder` + `Reset()` instead of `+=` concatenation |
| pprof flags | `cmd/root.go` | `--cpu-profile` / `--mem-profile` for on-demand profiling |
| Memory growth benchmarks | `internal/panels/gitdiff/bench_test.go` | `runtime.ReadMemStats` + `b.ReportMetric` for heap-inuse-b/op, gc-cycles/op |

---

## Memory Allocation Results (windows-amd64, authoritative)

Memory metrics (B/op, allocs/op) are deterministic — zero variance across runs.
These are the reliable signal. All changes are statistically significant (p=0.004).

### gitdiff — B/op (bytes allocated per render)

| Benchmark | Before | After | Δ |
|-----------|--------|-------|---|
| InlineDiffRender/10_lines | 4.47 KiB | 4.02 KiB | **-10.1%** |
| InlineDiffRender/100_lines | 37.5 KiB | 33.1 KiB | **-11.7%** |
| InlineDiffRender/1000_lines | 381.9 KiB | 322.3 KiB | **-15.6%** |
| SideBySideDiffRender/10_lines | 16.1 KiB | 15.7 KiB | **-2.8%** |
| SideBySideDiffRender/100_lines | 149.8 KiB | 145.4 KiB | **-3.0%** |
| SideBySideDiffRender/1000_lines | 1.40 MiB | 1.36 MiB | **-2.5%** |
| **geomean** | **25.5 KiB** | **24.2 KiB** | **-4.7%** |

### gitdiff — allocs/op (allocations per render)

| Benchmark | Before | After | Δ |
|-----------|--------|-------|---|
| InlineDiffRender/10_lines | 127 | 122 | **-3.9%** |
| InlineDiffRender/100_lines | 1,079 | 1,071 | **-0.7%** |
| InlineDiffRender/1000_lines | 10,450 | 10,437 | **-0.2%** |
| SideBySideDiffRender/10_lines | 243 | 238 | **-2.1%** |
| SideBySideDiffRender/100_lines | 2,225 | 2,217 | **-0.4%** |
| SideBySideDiffRender/1000_lines | 21,210 | 21,192 | **-0.1%** |
| **geomean** | **303** | **301** | **-0.7%** |

### git parsing — B/op (unchanged as expected)

| Benchmark | Before | After | Δ |
|-----------|--------|-------|---|
| ParseStatus/100_files | 24.66 KiB | 24.66 KiB | 0.0% |
| ParseDiff/1000_lines | 116.5 KiB | 116.5 KiB | 0.0% |
| All allocs/op | identical | identical | 0.0% |

Git parsing structs were aligned but have no slice reuse — B/op is correctly unchanged.

---

## Memory Growth Results (new — no before baseline)

`BenchmarkMemoryGrowth` measures net heap-inuse delta after GC per render operation.
**Negative values = memory is being freed correctly. Zero would be ideal.**

| Benchmark | heap-inuse-b/op | gc-cycles/op |
|-----------|----------------|--------------|
| inline_100_lines | -16 b/op | 0.011 |
| inline_1000_lines | -796 b/op | 0.115 |
| sidebyside_100_lines | -18 b/op | 0.052 |
| sidebyside_1000_lines | -1,694 b/op | 0.545 |

All negative: **no memory leaks detected in render paths.** GC cycles < 1 per render op.

---

## Timing Results (sec/op)

> **Note:** sec/op comparison between `bench_before.txt` and the current baseline is
> unreliable. The before file was collected at a different time under different thermal/load
> conditions, and shows ±∞% variance on many benchmarks (too few samples for stable
> estimation). The authoritative timing reference is the previous focused before/after
> comparison (`benchstat_comparison.txt`) which was run same-session same-machine:

From `perf/benchstat_comparison.txt` (controlled same-session comparison):

| Benchmark | Before | After | Δ |
|-----------|--------|-------|---|
| SideBySideDiffRender/1000_lines sec/op | ~5.1ms | ~3.1ms | **-39%** |
| InlineDiffRender/1000_lines B/op | ~385 KiB | ~323 KiB | **-16%** |

---

## Cross-Platform: Windows vs Linux (WSL2)

Same hardware (i9-13900K), same optimized code, different OS scheduler.

| Benchmark | windows-amd64 | linux-amd64 | Δ |
|-----------|--------------|-------------|---|
| gitdiff geomean sec/op | ~75 µs | ~36 µs | Linux 2x faster |
| git geomean sec/op | ~1.4 µs | ~2.1 µs | Similar |
| B/op (all) | identical | identical | Platform-independent |
| allocs/op (all) | identical | identical | Platform-independent |

Memory allocations are deterministic and platform-independent.
Timing differences are OS scheduler + Hyper-V VM overhead (WSL2).
CI runners (`ubuntu-latest`) are bare-metal Linux — expect better timing than WSL2.

---

## CI Integration

- `.github/workflows/bench.yml` — runs on every PR and main push
  - Platforms: `ubuntu-latest` + `macos-latest` (matrix)
  - Per-platform baselines: `perf/baselines/{goos-goarch}/main.txt`
  - Sticky PR comments with benchstat diff per platform
  - Regression detection: >15% timing, >10% memory (p<0.05)
  - Auto-commits updated baseline on main push

- `.github/workflows/release.yml` — on every release tag
  - Snapshots baseline as `perf/baselines/{platform}/v{version}.txt`
  - Injects `### Performance` section into CHANGELOG

- `scripts/bench-regress.sh` — regression detector
  - Timing threshold: 15% (p<0.05)
  - Memory threshold: 10% (heap-inuse-b/op, gc-cycles/op)

---

## Local Commands

```bash
mage benchBaseline   # capture new baseline for current platform
mage benchCompare    # compare current run against baseline
mage benchWSL        # run benchmarks in WSL (linux-amd64)
```

Raw data files:
- `perf/bench_before.txt` — original main branch, pre-optimization (5 runs)
- `perf/bench_after_final.txt` — optimized branch (6 runs, windows-amd64)
- `perf/benchstat_final_comparison.txt` — full benchstat output (all metrics)
- `perf/benchstat_comparison.txt` — controlled before/after (same session)
- `perf/baselines/windows-amd64/main.txt` — rolling Windows baseline
- `perf/baselines/linux-amd64/main.txt` — rolling Linux/WSL2 baseline
