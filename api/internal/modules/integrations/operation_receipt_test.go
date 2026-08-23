package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	testConversationID = "55555555-5555-4555-8555-555555555555"
	testMessageID      = "66666666-6666-4666-8666-666666666666"
	secondMessageID    = "77777777-7777-4777-8777-777777777777"
)

func TestExecutorReplaysConfirmedGuardedSuccessWithoutCallingProviderAgain(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	action := guardedTestAction()
	registry := registerTestAction(t, action, adapter)
	receipts := newMemoryOperationReceiptRepository()
	quota := &testQuota{}
	executor := NewExecutor(registry, &testAudit{}, quota, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(receipts)
	req := guardedActionRequest(testMessageID, "recipient-a", "hello")

	first, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	second := guardedActionRequest(testMessageID, "recipient-a", "hello, rephrased")
	replayed, err := executor.Execute(context.Background(), second)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if adapter.calls != 1 || quota.calls != 1 {
		t.Fatalf("provider calls = %d, quota calls = %d; replay must not call or charge again", adapter.calls, quota.calls)
	}
	if first == nil || replayed == nil || !replayed.Replayed || replayed.Output["ok"] != true {
		t.Fatalf("first = %#v, replayed = %#v", first, replayed)
	}
}

func TestExecutorPersistsCanonicalTypedOutputBeforeReplayingGuardedSuccess(t *testing.T) {
	action := guardedTestAction()
	action.OutputSchema = map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"accepted_recipients": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		},
		"required": []string{"accepted_recipients"},
	}
	adapter := &testAdapter{driverID: "test", execute: func(context.Context, ActionRequest) (*ActionResult, error) {
		return &ActionResult{Output: map[string]interface{}{"accepted_recipients": []string{"recipient-a"}}}, nil
	}}
	receipts := newMemoryOperationReceiptRepository()
	executor := NewExecutor(registerTestAction(t, action, adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(receipts)
	req := guardedActionRequest(testMessageID, "recipient-a", "hello")

	first, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	replayed, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("replayed Execute() error = %v", err)
	}
	if adapter.calls != 1 || first == nil || replayed == nil || !replayed.Replayed {
		t.Fatalf("adapter calls=%d first=%#v replayed=%#v", adapter.calls, first, replayed)
	}
	if _, ok := first.Output["accepted_recipients"].([]interface{}); !ok {
		t.Fatalf("first output was not canonical JSON: %T %#v", first.Output["accepted_recipients"], first.Output)
	}
	if _, ok := replayed.Output["accepted_recipients"].([]interface{}); !ok {
		t.Fatalf("replayed output was not canonical JSON: %T %#v", replayed.Output["accepted_recipients"], replayed.Output)
	}
}

func TestExecutorGuardAllowsRetryAfterDefiniteFailure(t *testing.T) {
	calls := 0
	adapter := &testAdapter{driverID: "test", execute: func(context.Context, ActionRequest) (*ActionResult, error) {
		calls++
		if calls == 1 {
			return nil, NewError(ErrorCodeInvalidInput, "provider rejected input", nil)
		}
		return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
	}}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())
	req := guardedActionRequest(testMessageID, "recipient-a", "hello")

	if result, err := executor.Execute(context.Background(), req); result != nil || ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("first result = %#v, error = %v", result, err)
	}
	if _, err := executor.Execute(context.Background(), req); err != nil {
		t.Fatalf("retry Execute() error = %v", err)
	}
	if adapter.calls != 2 {
		t.Fatalf("provider calls = %d, want retry after definite failure", adapter.calls)
	}
}

func TestExecutorGuardBlocksRetryAfterUnknownOutcome(t *testing.T) {
	adapter := &testAdapter{driverID: "test", execute: func(context.Context, ActionRequest) (*ActionResult, error) {
		return nil, NewError(ErrorCodeTimeout, "provider timed out", context.DeadlineExceeded)
	}}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())
	req := guardedActionRequest(testMessageID, "recipient-a", "hello")

	if _, err := executor.Execute(context.Background(), req); ErrorCode(err) != ErrorCodeTimeout {
		t.Fatalf("first error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), req); ErrorCode(err) != ErrorCodeOperationOutcomeUnknown {
		t.Fatalf("second error = %v, code = %q", err, ErrorCode(err))
	}
	if adapter.calls != 1 {
		t.Fatalf("provider calls = %d, unknown outcome must not be retried", adapter.calls)
	}
}

func TestExecutorGuardAllowsNewUserMessageAndDifferentTarget(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())

	requests := []ActionRequest{
		guardedActionRequest(testMessageID, "recipient-a", "hello"),
		guardedActionRequest(secondMessageID, "recipient-a", "send again"),
		guardedActionRequest(testMessageID, "recipient-b", "hello"),
	}
	for index, req := range requests {
		if _, err := executor.Execute(context.Background(), req); err != nil {
			t.Fatalf("Execute(%d) error = %v", index, err)
		}
	}
	if adapter.calls != 3 {
		t.Fatalf("provider calls = %d, new message and different target must remain allowed", adapter.calls)
	}
}

func TestExecutorGuardSerializesConcurrentDuplicateSideEffects(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	adapter := &testAdapter{driverID: "test", execute: func(context.Context, ActionRequest) (*ActionResult, error) {
		close(started)
		<-release
		return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
	}}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())
	req := guardedActionRequest(testMessageID, "recipient-a", "hello")
	firstDone := make(chan error, 1)
	go func() {
		_, err := executor.Execute(context.Background(), req)
		firstDone <- err
	}()
	<-started
	if _, err := executor.Execute(context.Background(), req); ErrorCode(err) != ErrorCodeOperationInProgress {
		t.Fatalf("concurrent duplicate error = %v, code = %q", err, ErrorCode(err))
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if adapter.calls != 1 {
		t.Fatalf("provider calls = %d, concurrent duplicate escaped the atomic claim", adapter.calls)
	}
}

func TestExecutorGuardExecutesTenDistinctItemsForSameTargetExactlyOnce(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	receipts := newMemoryOperationReceiptRepository()
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(receipts)

	for pass := 0; pass < 2; pass++ {
		for index := 1; index <= 10; index++ {
			req := guardedBatchItemRequest(index)
			result, err := executor.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("pass %d item %d error = %v", pass, index, err)
			}
			if (pass == 1) != result.Replayed {
				t.Fatalf("pass %d item %d replayed = %v", pass, index, result.Replayed)
			}
		}
	}
	if adapter.calls != 10 {
		t.Fatalf("provider calls = %d, want exactly 10 distinct batch items", adapter.calls)
	}
}

func TestExecutorGuardExecutesDistinctProjectedPhaseItemsAndReplaysEachExactlyOnce(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())

	first := guardedActionRequest(testMessageID, "recipient-a", "first")
	first.OperationItemID = "phase:" + strings.Repeat("1", 64)
	second := guardedActionRequest(testMessageID, "recipient-a", "second")
	second.OperationItemID = "phase:" + strings.Repeat("2", 64)
	for pass := 0; pass < 2; pass++ {
		for index, request := range []ActionRequest{first, second} {
			result, err := executor.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("pass %d phase %d Execute() error = %v", pass, index+1, err)
			}
			if result == nil || result.Replayed != (pass == 1) {
				t.Fatalf("pass %d phase %d result = %#v", pass, index+1, result)
			}
		}
	}
	if adapter.calls != 2 {
		t.Fatalf("provider calls = %d, want one call for each phase and no replay calls", adapter.calls)
	}
}

func TestExecutorGuardTreatsOneMessageContainingTenEntriesAsOneOperation(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())
	req := guardedActionRequest(testMessageID, "recipient-a", "1. one\n2. two\n3. three\n4. four\n5. five\n6. six\n7. seven\n8. eight\n9. nine\n10. ten")
	if _, err := executor.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if adapter.calls != 1 {
		t.Fatalf("provider calls = %d, combined content must be one operation", adapter.calls)
	}
}

func TestExecutorGuardRetriesOnlyDefiniteFailedBatchItem(t *testing.T) {
	itemFourCalls := 0
	adapter := &testAdapter{driverID: "test", execute: func(_ context.Context, req ActionRequest) (*ActionResult, error) {
		if req.OperationItemID == operationItemID(4) {
			itemFourCalls++
			if itemFourCalls == 1 {
				return nil, NewError(ErrorCodeProviderRejected, "rejected", nil)
			}
		}
		return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
	}}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())

	for pass := 0; pass < 2; pass++ {
		for index := 1; index <= 10; index++ {
			_, err := executor.Execute(context.Background(), guardedBatchItemRequest(index))
			if pass == 0 && index == 4 {
				if ErrorCode(err) != ErrorCodeProviderRejected {
					t.Fatalf("first item 4 error = %v", err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("pass %d item %d error = %v", pass, index, err)
			}
		}
	}
	if adapter.calls != 11 || itemFourCalls != 2 {
		t.Fatalf("provider calls = %d, item 4 calls = %d; only definite failure may retry", adapter.calls, itemFourCalls)
	}
}

func TestExecutorGuardNeverRetriesUnknownBatchItem(t *testing.T) {
	adapter := &testAdapter{driverID: "test", execute: func(_ context.Context, req ActionRequest) (*ActionResult, error) {
		if req.OperationItemID == operationItemID(4) {
			return nil, NewError(ErrorCodeTimeout, "timeout", context.DeadlineExceeded)
		}
		return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
	}}
	executor := NewExecutor(registerTestAction(t, guardedTestAction(), adapter), &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
		WithOperationReceiptRepository(newMemoryOperationReceiptRepository())

	for pass := 0; pass < 2; pass++ {
		for index := 1; index <= 10; index++ {
			_, err := executor.Execute(context.Background(), guardedBatchItemRequest(index))
			if index == 4 {
				want := ErrorCodeTimeout
				if pass == 1 {
					want = ErrorCodeOperationOutcomeUnknown
				}
				if ErrorCode(err) != want {
					t.Fatalf("pass %d item 4 error = %v, want %s", pass, err, want)
				}
				continue
			}
			if err != nil {
				t.Fatalf("pass %d item %d error = %v", pass, index, err)
			}
		}
	}
	if adapter.calls != 10 {
		t.Fatalf("provider calls = %d, unknown item must never be auto-retried", adapter.calls)
	}
}

func TestExecutorGuardReplaysSucceededItemsAfterExecutorRestart(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, guardedTestAction(), adapter)
	receipts := newMemoryOperationReceiptRepository()
	newExecutor := func() *Executor {
		return NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("operation-test-key"), 0).
			WithOperationReceiptRepository(receipts)
	}
	first := newExecutor()
	for index := 1; index <= 7; index++ {
		if _, err := first.Execute(context.Background(), guardedBatchItemRequest(index)); err != nil {
			t.Fatalf("first executor item %d error = %v", index, err)
		}
	}
	second := newExecutor()
	for index := 1; index <= 10; index++ {
		if _, err := second.Execute(context.Background(), guardedBatchItemRequest(index)); err != nil {
			t.Fatalf("second executor item %d error = %v", index, err)
		}
	}
	if adapter.calls != 10 {
		t.Fatalf("provider calls = %d, restart must execute only the 3 unfinished items", adapter.calls)
	}
}

func TestExecutorGuardConcurrentBatchExecutesEachItemAtMostOnce(t *testing.T) {
	adapter := &concurrentBatchAdapter{calls: map[string]int{}}
	executor := NewExecutor(
		registerTestAction(t, guardedTestAction(), adapter), concurrentBatchAudit{}, concurrentBatchQuota{}, nil,
		[]byte("operation-test-key"), 0,
	).WithOperationReceiptRepository(newMemoryOperationReceiptRepository())
	start := make(chan struct{})
	errorsByCall := make(chan error, 20)
	var wait sync.WaitGroup
	for copyIndex := 0; copyIndex < 2; copyIndex++ {
		for itemIndex := 1; itemIndex <= 10; itemIndex++ {
			request := guardedBatchItemRequest(itemIndex)
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				_, err := executor.Execute(context.Background(), request)
				if err != nil && ErrorCode(err) != ErrorCodeOperationInProgress {
					errorsByCall <- err
				}
			}()
		}
	}
	close(start)
	wait.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		t.Fatalf("concurrent batch error = %v", err)
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.calls) != 10 {
		t.Fatalf("provider item calls = %#v", adapter.calls)
	}
	for itemID, calls := range adapter.calls {
		if calls != 1 {
			t.Fatalf("provider item %s calls = %d, want at most one", itemID, calls)
		}
	}
}

func guardedBatchItemRequest(index int) ActionRequest {
	req := guardedActionRequest(testMessageID, "recipient-a", "message")
	req.BatchID = "batch-test"
	req.OperationItemID = operationItemID(index)
	req.ItemIndex = index
	req.ItemCount = 10
	return req
}

func operationItemID(index int) string {
	return fmt.Sprintf("item-%03d-fixed", index)
}

type concurrentBatchAdapter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (*concurrentBatchAdapter) DriverID() string { return "test" }

func (adapter *concurrentBatchAdapter) Execute(_ context.Context, request ActionRequest) (*ActionResult, error) {
	adapter.mu.Lock()
	adapter.calls[request.OperationItemID]++
	adapter.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
}

type concurrentBatchAudit struct{}

func (concurrentBatchAudit) Create(context.Context, *ExecutionRecord) error { return nil }
func (concurrentBatchAudit) Complete(context.Context, uuid.UUID, ExecutionCompletion) error {
	return nil
}

type concurrentBatchQuota struct{}

func (concurrentBatchQuota) Acquire(context.Context, string) error { return nil }

func TestOperationTargetDigestScopesSessionApprovalByRecipient(t *testing.T) {
	guard := guardedTestAction().SuccessDeduplication
	first := operationTargetDigest(map[string]interface{}{"recipient_id": "a", "recipient_type": "open_id", "text": "one"}, guard)
	rephrased := operationTargetDigest(map[string]interface{}{"recipient_id": "a", "recipient_type": "open_id", "text": "two"}, guard)
	different := operationTargetDigest(map[string]interface{}{"recipient_id": "b", "recipient_type": "open_id", "text": "one"}, guard)
	if first == "" || first != rephrased || first == different {
		t.Fatalf("target digests first=%q rephrased=%q different=%q", first, rephrased, different)
	}
}

func TestOperationIdentityUsesMessageConnectionActionTargetAndItem(t *testing.T) {
	action := guardedTestAction()
	resolved := ResolvedAction{IntegrationID: IntegrationWebSearch, Definition: action}
	base := guardedBatchItemRequest(1)
	first, err := deriveOperationIdentity([]byte("operation-test-key"), base, resolved)
	if err != nil {
		t.Fatalf("deriveOperationIdentity() error = %v", err)
	}
	differentConversation := base
	differentConversation.ConversationID = "88888888-8888-4888-8888-888888888888"
	same, err := deriveOperationIdentity([]byte("operation-test-key"), differentConversation, resolved)
	if err != nil || same.OperationKey != first.OperationKey {
		t.Fatalf("conversation changed operation identity: first=%#v same=%#v err=%v", first, same, err)
	}
	differentItem := base
	differentItem.OperationItemID = operationItemID(2)
	second, _ := deriveOperationIdentity([]byte("operation-test-key"), differentItem, resolved)
	differentTarget := base
	differentTarget.Input = cloneJSONMap(base.Input)
	differentTarget.Input["recipient_id"] = "recipient-b"
	third, _ := deriveOperationIdentity([]byte("operation-test-key"), differentTarget, resolved)
	if first.OperationKey == second.OperationKey || first.OperationKey == third.OperationKey {
		t.Fatalf("operation identity did not distinguish item/target: first=%#v second=%#v third=%#v", first, second, third)
	}
}

func guardedTestAction() ActionDefinition {
	action := testAction("message.send", "send_message")
	action.Effect = toolgovernance.EffectExternalSend
	action.RiskLevel = toolgovernance.RiskLevelHigh
	action.Idempotent = false
	action.SupportedCallers = []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}
	action.InputSchema = map[string]interface{}{
		"type": "object", "additionalProperties": false,
		"properties": map[string]interface{}{
			"recipient_id": map[string]interface{}{
				"type": "string", "minLength": 1,
				"title_i18n": LocalizedText{LocaleEnglishUS: "Recipient ID", LocaleSimplifiedChinese: "接收者 ID"},
			},
			"recipient_type": map[string]interface{}{
				"type": "string", "minLength": 1,
				"title_i18n": LocalizedText{LocaleEnglishUS: "Recipient type", LocaleSimplifiedChinese: "接收者类型"},
			},
			"text": map[string]interface{}{
				"type": "string", "minLength": 1,
				"title_i18n": LocalizedText{LocaleEnglishUS: "Message", LocaleSimplifiedChinese: "消息"},
			},
		},
		"required": []string{"recipient_id", "recipient_type", "text"},
	}
	action.SuccessDeduplication = &SuccessDeduplicationDefinition{TargetArgumentPaths: []string{"recipient_type", "recipient_id"}}
	return action
}

func guardedActionRequest(messageID, recipientID, text string) ActionRequest {
	req := validActionRequest("placeholder")
	req.ConversationID = testConversationID
	req.MessageID = messageID
	req.ActionID = "message.send"
	req.Input = map[string]interface{}{"recipient_id": recipientID, "recipient_type": "open_id", "text": text}
	return req
}

type memoryOperationReceiptRepository struct {
	mu    sync.Mutex
	byKey map[string]*OperationReceipt
	byID  map[uuid.UUID]*OperationReceipt
}

func newMemoryOperationReceiptRepository() *memoryOperationReceiptRepository {
	return &memoryOperationReceiptRepository{byKey: map[string]*OperationReceipt{}, byID: map[uuid.UUID]*OperationReceipt{}}
}

func (r *memoryOperationReceiptRepository) Claim(_ context.Context, candidate *OperationReceipt) (OperationReceiptClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.byKey[candidate.OperationKey]; existing != nil {
		copy := *existing
		copy.ResultPayload = cloneJSONMap(existing.ResultPayload)
		return OperationReceiptClaim{Receipt: &copy}, nil
	}
	copy := *candidate
	r.byKey[candidate.OperationKey] = &copy
	r.byID[candidate.ID] = &copy
	return OperationReceiptClaim{Receipt: candidate, Claimed: true}, nil
}

func (r *memoryOperationReceiptRepository) MarkProviderStarted(_ context.Context, id, token, executionID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	receipt := r.byID[id]
	if receipt == nil || receipt.ClaimToken != token {
		return errors.New("receipt not found")
	}
	now := time.Now().UTC()
	receipt.ProviderStartedAt = &now
	receipt.ExecutionID = &executionID
	return nil
}

func (r *memoryOperationReceiptRepository) CompleteSuccess(_ context.Context, id, token uuid.UUID, result *ActionResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	receipt := r.byID[id]
	if receipt == nil || receipt.ClaimToken != token {
		return errors.New("receipt not found")
	}
	receipt.Status = OperationReceiptStatusSucceeded
	receipt.ProviderRequestID = result.ProviderRequestID
	receipt.ResultPayload = cloneJSONMap(result.Output)
	receipt.ResultCount = result.ResultCount
	return nil
}

func (r *memoryOperationReceiptRepository) Release(_ context.Context, id, token uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	receipt := r.byID[id]
	if receipt == nil || receipt.ClaimToken != token {
		return errors.New("receipt not found")
	}
	delete(r.byKey, receipt.OperationKey)
	delete(r.byID, id)
	return nil
}

func (r *memoryOperationReceiptRepository) MarkOutcomeUnknown(_ context.Context, id, token, executionID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	receipt := r.byID[id]
	if receipt == nil || receipt.ClaimToken != token {
		return errors.New("receipt not found")
	}
	receipt.Status = OperationReceiptStatusOutcomeUnknown
	receipt.ExecutionID = &executionID
	return nil
}
