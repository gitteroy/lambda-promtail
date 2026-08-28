# Why this fork exists

The upstream `lambda-promtail` feeds label sets to Prometheus's
`relabel.Process` in **random order**, but that function requires **sorted input** —
so our relabel rules match non-deterministically and ~40–50% of log entries reach
Loki **mislabeled**. This fork adds the one missing line, `builder.Sort()`, to make
relabeling correct and deterministic.

---

## The bug in one picture

The label set is built from a **Go map** (random iteration order) and passed
straight to `relabel.Process`, which **requires sorted input**. Skipping the sort
step is the bug:

```
                    ┌────────────────────────────────┐
                    │  entry labels (a Go map)       │
                    │  → RANDOM iteration order      │
                    └───────────────┬────────────────┘
                                    │
                    UPSTREAM (bug)  │  THIS FORK (fixed)
                                    │
              ┌─────────────────────┴─────────────────────┐
              ▼                                             ▼
      (no sort step)                                 builder.Sort()
              │                                             │
              ▼                                             ▼
   relabel.Process gets                          relabel.Process gets
   labels in random order                        labels sorted by name
              │                                             │
              ▼                                             ▼
   matches by luck →                             matches every time →
   ~half of entries mislabeled                   correct & deterministic
```

`relabel.Process` matches each rule's `source_labels` against the entry's labels
and **assumes they are sorted by name**. Upstream never sorts them, so the match
succeeds or fails depending on the random order the map produced **on that
invocation** — same log line, same config, different result each time.

## The fix (the entire diff)

`pkg/main.go` → `applyRelabelConfigs()`:

```go
  builder := prommodel.NewScratchBuilder(len(labels))
  for name, value := range labels {   // map range = random order
      builder.Add(string(name), string(value))
  }

+ builder.Sort()                       // ← THE FIX: satisfy relabel.Process's contract
  promLabels := builder.Labels()

  processedLabels, keep := relabel.Process(promLabels, relabelConfigs...)
```

Upstream even has the comment `// Sort labels as required by Process` — but never
calls `Sort()`. This fork makes the code match the comment.

---

## Why it matters to us specifically

Our platform relabel rules (`observability` → `lambda-promtail/relabel-configs.json`)
are **entirely** `replace` / `labeldrop` actions — every one depends on the broken code path:

| Rule | Action | Effect | Broken upstream? |
|------|--------|--------|------------------|
| `__aws_log_type` → `log_type` | replace | rename format discriminator | **Yes** |
| `__aws_cloudwatch_log_group` → `log_group` | replace | rename CW log group | **Yes** |
| `__aws_s3_log_lb` → `loadbalancer` | replace | rename ALB name | **Yes** |
| `__aws_cloudwatch_log_stream` | labeldrop | drop high-cardinality label | **Yes** |

**Symptom on upstream:** `log_type`, `log_group`, and `loadbalancer` are randomly
missing on ~half of entries; `log_stream` sometimes fails to drop. Entries are **not**
dropped (we use no `keep`/`drop` actions) — they land in Loki **mislabeled**, which
makes any panel grouped by these labels show fluctuating or partial series.

## The proof (real data)

`pkg/relabel_apply_test.go` reproduces it with our actual config:

- Feeds a realistic CloudWatch label set through `applyRelabelConfigs` **1000 times**.
- **Without `Sort()`:** output varies between iterations — renamed labels randomly
  absent. Test **fails**.
- **With `Sort()`:** all 1000 iterations produce identical, correct output. Test **passes**.

Run it:

```bash
go test ./pkg -run TestApplyRelabelConfigs_DeterministicOutput -v
```

---

## When we can delete this fork

The moment upstream `grafana/lambda-promtail` ships a release containing the `Sort()`
call (or merges our PR), retire the fork: point consumers' `var.lambda_promtail_version`
at the plain upstream release and drop the `-sortfix` tagging. See the main
[README](../README.md) for the rebase/release workflow.