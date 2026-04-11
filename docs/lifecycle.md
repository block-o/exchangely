# Task Lifecycle

This document explains how Exchangely's planner, workers, and coordinator interact to schedule, execute, and track tasks. It covers the full lifecycle from task creation through completion, the deduplication strategy, and how each task type flows through the system.

## Overview

Exchangely uses an event-driven task engine built on three cooperating components:

| Component | Role | Key files |
|-----------|------|-----------|
| **Planner** | Acquires leadership, reads sync state, generates tasks, writes them to PostgreSQL, publishes references to Kafka | `planner/runtime.go`, `planner/scheduler.go` |
| **Worker** | Claims tasks from the DB, acquires per-pair advisory locks, executes task-specific logic, writes results | `worker/processor.go`, `worker/*_executor.go` |
| **Coordinator** | DB-backed lease manager for planner leadership election | `coordinator/lease_manager.go`, `storage/postgres/lease_repository.go` |

PostgreSQL is the single source of truth for task state, sync progress, coverage tracking, and leadership. Kafka is a notification channel that accelerates task pickup but is not required for correctness.

## Planner Tick

The planner runs a loop every `BACKEND_PLANNER_TICK` (default 10s). Each tick:

```
1. Acquire or renew the leadership lease (DB-backed, 15s TTL)
   └─ If not leader → skip this tick

2. Load current state
   ├─ List all tracked pairs from the catalog
   ├─ Load sync_status for each pair (hourly/daily progress, realtime cutover)
   ├─ Load data_coverage (which days have verified gap-free data)
   ├─ Load integrity_coverage (which days have passed cross-source checks)
   └─ Query active backfill pairs (pairs with pending/running backfill tasks)

3. Generate tasks (two batches)
   ├─ Batch 1 (priority): Realtime tasks — enqueued first
   └─ Batch 2 (follow-up): Everything else
       ├─ Backfill tasks (round-robin across pairs, budget-capped)
       ├─ Backfill probe tasks (one per pair per day)
       ├─ Gap validation sweeps (one per pair with uncovered days)
       ├─ Integrity check sweeps (one per pair with unverified days)
       ├─ Consolidation tasks (daily candle rebuild for caught-up pairs)
       ├─ Cleanup task (one per calendar day)
       └─ News fetch tasks (one per RSS source)

4. Enqueue tasks to PostgreSQL (idempotent via ON CONFLICT)
5. Publish task references to Kafka (best-effort, degrades gracefully)
```

Realtime tasks are enqueued in a separate batch before everything else so they reach workers immediately, even when the follow-up batch is large.

## Task Deduplication

Every task has a deterministic ID. The `Enqueue` method uses `INSERT ... ON CONFLICT (id) DO UPDATE ... WHERE status IN ('completed', 'failed')`. This means:

- A task that is `pending` or `running` is never overwritten — the insert is silently skipped.
- A task that is `completed` or `failed` gets reset to `pending` so it can run again.
- Stale `live_ticker` tasks (claimed >30s ago but still `running`) are also re-queued.

This is the core mechanism that prevents task duplication. The ID format determines the dedup granularity:

| Task type | ID format | Dedup behavior |
|-----------|-----------|----------------|
| `live_ticker` | `live_ticker:{PAIR}:realtime` | One per pair, ever. Re-queued after completion. |
| `news_fetch` | `news_fetch:{SOURCE}:periodic` | One per RSS source, ever. Re-queued after completion. |
| `integrity_check` | `integrity_check:{PAIR}:sweep` | One per pair, ever. Re-queued after completion. |
| `gap_validation` | `gap_validation:{PAIR}:sweep` | One per pair, ever. Re-queued after completion. |
| `historical_backfill` | `historical_backfill:{PAIR}:{INTERVAL}:{START}:{END}` | One per specific time window. |
| `consolidation` | `consolidation:{PAIR}:1d:{START}:{END}` | One per pair per day. |
| `task_cleanup` | `task_cleanup:daily` | One ever. Re-queued after completion. |
| Backfill probe | `historical_backfill:{PAIR}:probe:{DAY_UNIX}` | One per pair per calendar day. |

Tasks with stable IDs (no timestamp component) naturally cycle: the planner emits them every tick, but the insert is a no-op while the task is pending or running. Once a worker completes it, the next planner tick re-queues it.

## Worker Processing

Workers run a polling loop every `BACKEND_WORKER_POLL_INTERVAL` (default 5s). Each poll:

```
1. Fetch pending tasks from PostgreSQL (3-phase priority fetch)
   ├─ Phase 1: live_ticker tasks (guaranteed slots)
   ├─ Phase 2: Other non-backfill tasks (integrity, consolidation, gap, news, cleanup)
   └─ Phase 3: Backfill tasks (up to the backfill budget cap)

2. Process the batch concurrently (up to BACKEND_WORKER_CONCURRENCY goroutines)
   └─ For each task (in parallel):
       ├─ Claim the task (atomic UPDATE status='running' WHERE status IN ('pending','failed'))
       ├─ Acquire a per-pair advisory lock (pg_advisory_lock)
       ├─ Route to the type-specific executor
       ├─ On success: mark 'completed'
       └─ On failure: increment retry_count, set retry_at with jitter
           ├─ 1h tasks: up to 24 retries
           └─ 1d tasks: up to 7 retries
           └─ After max retries: mark 'failed' permanently

3. Wait for all goroutines to finish before starting the next poll
```

The 3-phase fetch ensures live_ticker tasks always get worker slots regardless of how many other tasks are queued. This prevents backfill or maintenance tasks from starving realtime price updates.

### Parallel batch execution

`BACKEND_WORKER_CONCURRENCY` (default 4) controls how many tasks within a single batch are processed simultaneously. A semaphore limits the number of active goroutines, and a `sync.WaitGroup` ensures the batch completes before the next poll starts.

Safety is preserved by two mechanisms:
- **Atomic claiming**: Each task is claimed via an atomic `UPDATE ... WHERE status='pending'`. If two workers race for the same task, only one succeeds.
- **Per-pair advisory locks**: Even with parallel execution, `pg_advisory_lock` prevents two goroutines from mutating the same trading pair's data concurrently. A goroutine processing BTCEUR blocks until any other goroutine holding the BTCEUR lock finishes.

Setting `BACKEND_WORKER_CONCURRENCY=1` restores the original sequential behavior. The sequential path avoids goroutine overhead entirely — it uses a plain `for` loop instead of the semaphore/WaitGroup machinery.

Task failures within a parallel batch do not block or cancel other goroutines. Each task logs its own failure independently and the remaining tasks continue to completion.

Workers also receive task notifications via Kafka for faster pickup, but the polling loop is the primary mechanism.

## Task State Machine

```
                    ┌──────────────────────────────┐
                    │                              │
                    ▼                              │
  ┌─────────┐   claim   ┌─────────┐   fail    ┌──┴────┐
  │ pending │──────────▶│ running │──────────▶│ failed │
  └─────────┘           └─────────┘           └────────┘
       ▲                     │                     │
       │                     │ complete            │ (retry < max)
       │                     ▼                     │
       │              ┌───────────┐                │
       │              │ completed │                │
       │              └───────────┘                │
       │                                           │
       └───────────────────────────────────────────┘
              (re-enqueue resets to pending)
```

- `pending` → `running`: Worker claims the task (atomic).
- `running` → `completed`: Executor succeeds.
- `running` → `failed`/`pending`: Executor fails. If retries remain, status goes back to `pending` with a `retry_at` delay (1h ± 5min jitter). If retries are exhausted, status stays `failed`.
- `completed`/`failed` → `pending`: Planner re-enqueues via ON CONFLICT (for recurring tasks with stable IDs).

## Task Types in Detail

### live_ticker

Polls external providers for the latest price snapshot of a trading pair.

- **Scheduled by**: `BuildRealtimeTasks` — one per pair with sync state.
- **ID**: `live_ticker:{PAIR}:realtime` (stable, no timestamp).
- **Executor**: `RealtimeExecutor` — fetches from providers with `CapRealtime`, publishes to Kafka or falls back to direct DB write.
- **Cycle**: Completes → next planner tick re-enqueues → worker picks up again.
- **Priority**: Always fetched first in the worker batch (Phase 1).

### historical_backfill

Fetches historical OHLCV candles walking backwards from yesterday into the past.

- **Scheduled by**: `BuildInitialBackfillTasksLimited` — round-robin across pairs, budget-capped per tick.
- **ID**: `historical_backfill:{PAIR}:{INTERVAL}:{START_UNIX}:{END_UNIX}` (unique per window).
- **Executor**: `BackfillExecutor` — fetches from providers with `CapHistorical`, consolidates raw candles, writes to both `raw_candles` and `candles_1h`/`candles_1d`.
- **Dedup**: Pairs with active (pending/running) backfill tasks are skipped by the planner so the budget goes to other pairs.
- **Daily probe**: `BuildBackfillProbeTasks` emits one extra task per pair per day targeting the hour before the oldest synced point, discovering newly available upstream data.

### integrity_check

Cross-source validation that compares candle data from multiple providers.

- **Scheduled by**: `BuildIntegrityCheckTasks` — one stable sweep per pair with coverage.
- **ID**: `integrity_check:{PAIR}:sweep` (stable, no timestamp).
- **Executor**: `ValidatorExecutor` — walks unverified days (up to 7 per run), fetches from all validator sources, compares close prices. Days that pass are marked in `integrity_coverage`. Days with divergence above `BACKEND_INTEGRITY_MAX_DIVERGENCE_PCT` fail.
- **Progressive**: Each run picks up where the last left off. Already-verified days are skipped.
- **Cycle**: Completes → planner checks for remaining unverified days → re-enqueues if any exist.

### gap_validation

Verifies that stored data has no missing hours or days.

- **Scheduled by**: `BuildGapValidationTasks` — one stable sweep per pair with uncovered days.
- **ID**: `gap_validation:{PAIR}:sweep` (stable, no timestamp).
- **Executor**: `GapValidatorExecutor` — walks uncovered days (up to 30 per run), checks for 24 hourly candles + 1 daily candle per day. Complete days are marked in `data_coverage`.
- **Progressive**: Each run picks up where the last left off. Already-covered days are skipped.
- **Cycle**: Completes → planner checks for remaining uncovered days → re-enqueues if any exist.

### consolidation

Rebuilds daily candles from hourly candles for the previous UTC day.

- **Scheduled by**: `BuildConsolidationTasks` — one per fully caught-up pair (hourly + daily backfill complete).
- **ID**: `consolidation:{PAIR}:1d:{START_UNIX}:{END_UNIX}` (unique per day).
- **Executor**: `BackfillExecutor` (daily path) — reads hourly candles from DB, aggregates into daily.

### news_fetch

Fetches RSS news from a specific source.

- **Scheduled by**: `BuildNewsFetchTasks` — one per RSS source (coindesk, cointelegraph, theblock).
- **ID**: `news_fetch:{SOURCE}:periodic` (stable, no timestamp).
- **Executor**: `NewsExecutor` → `NewsService.FetchSource` — fetches RSS, parses XML, upserts to DB.
- **Cycle**: Completes → next planner tick re-enqueues → worker picks up again.
- **Independence**: Each source fails/retries independently. A CoinDesk outage doesn't block Cointelegraph fetches.

### task_cleanup

Prunes old completed/failed tasks from the task table.

- **Scheduled by**: `BuildCleanupTask` — single stable task.
- **ID**: `task_cleanup:daily` (stable, no timestamp).
- **Executor**: `CleanupExecutor` — deletes tasks older than `BACKEND_TASK_RETENTION_PERIOD` (default 24h) and caps the log at `BACKEND_TASK_MAX_LOG_COUNT` (default 1000).
- **Cycle**: Completes → next planner tick re-enqueues → runs again.

## Leadership and Coordination

Only one planner instance can schedule tasks at a time. Leadership is managed via a DB-backed lease:

- The `service_leases` table stores the current leader's ID and expiry.
- Each planner tick attempts `AcquireOrRenew`. The query uses `ON CONFLICT ... WHERE expires_at < NOW() OR holder_id = holder` so only the current holder or an expired lease can be claimed.
- Lease TTL is 15s (configurable via `BACKEND_PLANNER_LEASE_TTL`), renewed every tick (10s). If a planner crashes, the lease expires and another instance takes over within 15s.

Workers don't need leadership — they independently poll for pending tasks and use atomic claims to avoid double-processing.

## Pair-Level Locking

Workers acquire PostgreSQL advisory locks (`pg_advisory_lock`) keyed by pair symbol before executing any task. This prevents two workers from mutating the same pair's data concurrently, even across different task types. The lock is held for the duration of task execution and released immediately after.

## Coverage Tracking

Two tables track progressive validation state:

| Table | Purpose | Written by |
|-------|---------|------------|
| `data_coverage` | Tracks which (pair, day) combinations have complete hourly + daily candle data | `GapValidatorExecutor` |
| `integrity_coverage` | Tracks which (pair, day) combinations have passed cross-source integrity checks | `ValidatorExecutor` |

Both tables use the same schema: `(pair_symbol, day) → is_complete/verified`. The planner reads these tables each tick to decide whether to emit new sweep tasks. Once all days are covered/verified for a pair, no new tasks are generated until new data extends the range.

## Sequence Diagram: Typical Planner Tick

```
Planner                    PostgreSQL                  Kafka              Worker
   │                          │                          │                  │
   │── AcquireOrRenew ───────▶│                          │                  │
   │◀── lease acquired ───────│                          │                  │
   │                          │                          │                  │
   │── ListPairs ────────────▶│                          │                  │
   │── States ───────────────▶│                          │                  │
   │── GetAllCompletedDays ──▶│                          │                  │
   │── GetAllVerifiedDays ───▶│                          │                  │
   │── ActiveBackfillPairs ──▶│                          │                  │
   │◀── state loaded ─────────│                          │                  │
   │                          │                          │                  │
   │   [generate tasks]       │                          │                  │
   │                          │                          │                  │
   │── Enqueue(realtime) ────▶│                          │                  │
   │── Publish(realtime) ─────┼─────────────────────────▶│                  │
   │── Enqueue(follow-up) ───▶│                          │                  │
   │── Publish(follow-up) ────┼─────────────────────────▶│                  │
   │                          │                          │                  │
   │                          │                          │── notify ───────▶│
   │                          │                          │                  │
   │                          │◀── Pending(batch) ───────┼──────────────────│
   │                          │── tasks ────────────────▶│                  │
   │                          │                          │                  │
   │                          │                          │  [parallel exec] │
   │                          │◀── Claim(task_A) ────────┼──────────────────│
   │                          │◀── Claim(task_B) ────────┼──────────────────│
   │                          │◀── pg_advisory_lock(A) ──┼──────────────────│
   │                          │◀── pg_advisory_lock(B) ──┼──────────────────│
   │                          │                          │  [execute A & B] │
   │                          │◀── Complete(task_A) ─────┼──────────────────│
   │                          │◀── Complete(task_B) ─────┼──────────────────│
   │                          │◀── pg_advisory_unlock ───┼──────────────────│
```

## Key Source Files

| File | Purpose |
|------|---------|
| `planner/runtime.go` | Planner loop, lease renewal, state loading, batch enqueueing |
| `planner/scheduler.go` | Task generation logic for all 7 task types |
| `worker/processor.go` | Task claiming, pair locking, routing to executors, completion/failure |
| `worker/backfill_executor.go` | Historical candle fetch + consolidation |
| `worker/realtime_executor.go` | Live ticker polling + Kafka publish |
| `worker/validator_executor.go` | Cross-source integrity sweep with persistent tracking |
| `worker/gap_validator_executor.go` | Data completeness sweep with persistent tracking |
| `worker/news_executor.go` | Per-source RSS fetch |
| `worker/cleanup_executor.go` | Task log pruning |
| `storage/postgres/task_repository.go` | Enqueue, Claim, Complete, Fail, Pending (3-phase priority fetch) |
| `storage/postgres/lease_repository.go` | Leadership lease acquire/renew |
| `storage/postgres/pair_lock.go` | Per-pair advisory locking |
| `storage/postgres/coverage_repository.go` | Gap validation coverage tracking |
| `storage/postgres/integrity_repository.go` | Integrity check coverage tracking |
| `domain/task/task.go` | Task type constants and description builder |
