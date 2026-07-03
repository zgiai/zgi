# Workflow Test Batch Timeout Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent workflow test batches from staying in `running` forever when a single workflow execution stalls or the async goroutine disappears.

**Architecture:** Keep the current lightweight goroutine-based execution for now, but make the status machine durable enough for production testing: only mark an item `running` when it actually starts, add per-item timeout, recover stale running batches, and persist a failure state on panic. This intentionally avoids a full queue migration in the first fix.

**Tech Stack:** Go service/repository tests with sqlmock, PostgreSQL via GORM, existing Next.js React Query polling.

---

## File Structure

- Modify `api/internal/modules/app/workflowtest/repository.go`
  - Add item-level status transition helpers.
  - Add stale running batch recovery helper.
  - Add batch heartbeat helper.
- Modify `api/internal/modules/app/workflowtest/service.go`
  - Stop marking all items as running in `StartBatch`.
  - Mark each item running immediately before execution.
  - Wrap each item execution with a timeout.
  - Update batch heartbeat after each item result.
  - Add stale batch recovery service method.
- Modify `api/internal/modules/app/workflowtest/handler.go`
  - Recover stale batches before creating/executing relevant batch actions.
  - Add panic recovery around async execution goroutine.
- Add or modify `api/internal/modules/app/workflowtest/batch_execution_test.go`
  - Verify `StartBatch` keeps items pending.
  - Verify item status changes to running only when execution starts.
  - Verify a timed-out item is failed and the batch continues.
  - Verify stale running batches are stopped and unfinished items are failed.

---

### Task 1: Add Tests For Correct Batch Progress Semantics

**Files:**
- Create: `api/internal/modules/app/workflowtest/batch_execution_test.go`

- [ ] **Step 1: Write failing test for `StartBatch` keeping items pending**

Add a test using sqlmock that expects only `workflow_test_batches.status` to change from `queued` to `running`. It must not expect an `UPDATE workflow_test_batch_items ... status='running'` during `StartBatch`.

- [ ] **Step 2: Run test and verify it fails**

Run:

```powershell
go test ./internal/modules/app/workflowtest -run TestStartBatchDoesNotMarkAllItemsRunning -count=1
```

Expected: FAIL because current `StartBatch` updates all pending items to `running`.

- [ ] **Step 3: Write failing test for per-item timeout**

Add a fake runner that blocks until context cancellation. Execute a batch with two items and assert the first item becomes `failed` with timeout error while execution proceeds to the second item.

- [ ] **Step 4: Run test and verify it fails**

Run:

```powershell
go test ./internal/modules/app/workflowtest -run TestExecuteBatchFailsTimedOutItemAndContinues -count=1
```

Expected: FAIL because current execution has no per-item timeout.

---

### Task 2: Implement Item-Level Running State And Timeout

**Files:**
- Modify: `api/internal/modules/app/workflowtest/repository.go`
- Modify: `api/internal/modules/app/workflowtest/service.go`

- [ ] **Step 1: Add repository helpers**

Add:

```go
func (r *Repository) UpdateBatchItemStatusIfCurrent(ctx context.Context, agentID, itemID, currentStatus, nextStatus string) (bool, error)
func (r *Repository) TouchBatch(ctx context.Context, agentID, batchID string) error
```

- [ ] **Step 2: Change `StartBatch`**

Remove the call that updates all pending items to running. `StartBatch` should only move the batch itself from `queued` to `running`.

- [ ] **Step 3: Add per-item timeout**

Use a package-level constant:

```go
const batchItemExecutionTimeout = 10 * time.Minute
```

Before each item execution:

1. Transition the item from `pending` to `running`.
2. Use `context.WithTimeout(ctx, batchItemExecutionTimeout)`.
3. If the context times out, mark the item as failed with `测试问题执行超时`.
4. Continue to the next item.

- [ ] **Step 4: Run focused tests**

Run:

```powershell
go test ./internal/modules/app/workflowtest -run "TestStartBatchDoesNotMarkAllItemsRunning|TestExecuteBatchFailsTimedOutItemAndContinues" -count=1
```

Expected: PASS.

---

### Task 3: Add Stale Batch Recovery And Panic Safety

**Files:**
- Modify: `api/internal/modules/app/workflowtest/repository.go`
- Modify: `api/internal/modules/app/workflowtest/service.go`
- Modify: `api/internal/modules/app/workflowtest/handler.go`
- Test: `api/internal/modules/app/workflowtest/batch_execution_test.go`

- [ ] **Step 1: Write failing stale recovery test**

Assert that a running batch older than the cutoff is marked `stopped`, and its `pending`/`running` items are marked `failed`.

- [ ] **Step 2: Implement stale recovery**

Add repository method:

```go
func (r *Repository) RecoverStaleRunningBatches(ctx context.Context, staleBefore time.Time, summary string, completedAt time.Time) (int64, error)
```

Behavior:

1. Select stale running batch IDs.
2. Update those batches to `stopped` with summary.
3. Update unfinished items for those batch IDs to `failed` with error text.

- [ ] **Step 3: Call recovery from handler**

Before executing a batch, call:

```go
_, err := h.service.RecoverStaleRunningBatches(c.Request.Context(), time.Now().Add(-60*time.Minute))
```

Log warning if recovery fails, but do not block the request.

- [ ] **Step 4: Add panic recovery**

Wrap async execution goroutine with `defer recover()` and call `MarkBatchExecutionFailed` with a panic-derived error.

- [ ] **Step 5: Run tests**

Run:

```powershell
go test ./internal/modules/app/workflowtest -count=1
```

Expected: PASS.

---

### Task 4: Validation

**Files:**
- No code files unless validation reveals failures.

- [ ] **Step 1: Format Go files**

Run:

```powershell
gofmt -w api/internal/modules/app/workflowtest/handler.go api/internal/modules/app/workflowtest/repository.go api/internal/modules/app/workflowtest/service.go api/internal/modules/app/workflowtest/batch_execution_test.go
```

- [ ] **Step 2: Run targeted tests**

Run:

```powershell
go test ./internal/modules/app/workflowtest -count=1
```

Expected: PASS.

- [ ] **Step 3: Confirm frontend does not need code changes**

React Query already polls running batches/items every 3 seconds. With item status now remaining `pending` until real execution, existing UI should show more accurate progress without frontend changes.

---

## Self-Review

- Spec coverage: Covers stale running batch, per-item blocking timeout, accurate item progress, and panic safety.
- Placeholder scan: No TODO/TBD placeholders.
- Type consistency: Uses existing `BatchStatusStopped`, `BatchItemStatusFailed`, repository/service patterns.
