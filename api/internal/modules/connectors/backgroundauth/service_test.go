package backgroundauth

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func fixture(t *testing.T) (*Service, IssueRequest, time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err = db.AutoMigrate(&Grant{}, &Consumption{}, &RunBudget{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	s, err := New(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{OrganizationID: uuid.New(), OwnerID: uuid.New(), WorkspaceID: uuid.New(), EmployeeID: uuid.New(), VersionID: uuid.New(), BindingID: uuid.New(), ConnectionID: uuid.New(), BindingRevision: 2, ConnectionRevision: 3, CredentialVersion: 1, ContractDigest: strings.Repeat("a", 64)}
	return s, IssueRequest{Scope: scope, Operations: []string{"x-social.post.create"}, Destinations: map[string]string{"account_id": "account-1"}, ExpiresAt: now.Add(time.Hour), MaxInvocations: 2}, now
}

func TestConsumeEnforcesAtomicServerRunBudget(t *testing.T) {
	s, issue, _ := fixture(t)
	issue.MaxInvocations = 10
	if _, err := s.Issue(t.Context(), issue); err != nil {
		t.Fatal(err)
	}
	runID := uuid.New()
	for index := 0; index < 2; index++ {
		request := consumeRequest(issue)
		request.RunID, request.MaxRunInvocations = runID, 2
		if _, err := s.Consume(t.Context(), request); err != nil {
			t.Fatalf("consume %d: %v", index, err)
		}
	}
	request := consumeRequest(issue)
	request.RunID, request.MaxRunInvocations = runID, 2
	if _, err := s.Consume(t.Context(), request); err == nil {
		t.Fatal("run connector budget exceeded without denial")
	}
	var budget RunBudget
	if err := s.db.Where("run_id = ?", runID).Take(&budget).Error; err != nil || budget.UsedInvocations != 2 || budget.MaxInvocations != 2 {
		t.Fatalf("run budget = %#v, %v", budget, err)
	}
	request = consumeRequest(issue)
	request.RunID, request.MaxRunInvocations = runID, 3
	if _, err := s.Consume(t.Context(), request); err == nil {
		t.Fatal("caller changed restored run budget")
	}
}
func consumeRequest(issue IssueRequest) ConsumeRequest {
	return ConsumeRequest{Scope: issue.Scope, OperationID: issue.Operations[0], LogicalOperationID: uuid.NewString(), Input: map[string]any{"account_id": "account-1", "text": "hello"}}
}
func TestDestinationAllowsOnlyExactSingletonRecipient(t *testing.T) {
	s, issue, _ := fixture(t)
	issue.Destinations = map[string]string{"to": "team@example.com"}
	issue.MaxInvocations = 10
	ctx := context.Background()
	if _, err := s.Issue(ctx, issue); err != nil {
		t.Fatal(err)
	}
	for _, input := range []any{[]string{"team@example.com"}, []any{"team@example.com"}} {
		r := consumeRequest(issue)
		r.Input = map[string]any{"to": input}
		if _, err := s.Consume(ctx, r); err != nil {
			t.Fatalf("exact singleton denied: %v", err)
		}
	}
	for _, input := range []any{nil, []string{}, []any{"other@example.com"}, []any{"team@example.com", "other@example.com"}, []any{"team@example.com", "team@example.com"}, []any{123}, map[string]any{"to": "team@example.com"}} {
		r := consumeRequest(issue)
		r.Input = map[string]any{"to": input}
		if _, err := s.Consume(ctx, r); err == nil {
			t.Fatalf("non-exact recipient accepted: %#v", input)
		}
	}
}
func TestConsumeRequiresExactScopeAndDestination(t *testing.T) {
	s, issue, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.Consume(ctx, consumeRequest(issue)); err == nil {
		t.Fatal("missing grant authorized")
	}
	g, err := s.Issue(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ConsumeRequest){
		func(r *ConsumeRequest) { r.OwnerID = uuid.New() }, func(r *ConsumeRequest) { r.OrganizationID = uuid.New() },
		func(r *ConsumeRequest) { r.WorkspaceID = uuid.New() }, func(r *ConsumeRequest) { r.EmployeeID = uuid.New() },
		func(r *ConsumeRequest) { r.VersionID = uuid.New() }, func(r *ConsumeRequest) { r.BindingID = uuid.New() },
		func(r *ConsumeRequest) { r.ConnectionID = uuid.New() }, func(r *ConsumeRequest) { r.BindingRevision++ },
		func(r *ConsumeRequest) { r.ConnectionRevision++ }, func(r *ConsumeRequest) { r.CredentialVersion++ },
		func(r *ConsumeRequest) { r.ContractDigest = strings.Repeat("b", 64) }, func(r *ConsumeRequest) { r.OperationID = "x-social.dm.send" },
		func(r *ConsumeRequest) { r.Input = map[string]any{"account_id": "other"} },
	} {
		r := consumeRequest(issue)
		mutate(&r)
		if _, err = s.Consume(ctx, r); err == nil {
			t.Fatal("mismatched authorization accepted", r)
		}
	}
	r := consumeRequest(issue)
	receipt, err := s.Consume(ctx, r)
	if err != nil || receipt.GrantID != g.ID {
		t.Fatal(receipt, err)
	}
	if _, err = s.Consume(ctx, r); err == nil {
		t.Fatal("duplicate dispatch authorized")
	}
	if _, err = s.Consume(ctx, consumeRequest(issue)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Consume(ctx, consumeRequest(issue)); err == nil {
		t.Fatal("quota exceeded")
	}
}
func TestRevocationExpiryAndValidation(t *testing.T) {
	s, issue, now := fixture(t)
	ctx := context.Background()
	for _, mutate := range []func(*IssueRequest){func(r *IssueRequest) { r.ExpiresAt = now }, func(r *IssueRequest) { r.MaxInvocations = 0 }, func(r *IssueRequest) { r.Operations = nil }, func(r *IssueRequest) { r.ContractDigest = "" }, func(r *IssueRequest) { r.Destinations = map[string]string{"recipient": "*"} }} {
		r := issue
		mutate(&r)
		if _, err := s.Issue(ctx, r); err == nil {
			t.Fatal("invalid grant issued")
		}
	}
	g, err := s.Issue(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Revoke(ctx, issue.OrganizationID, uuid.New(), g.ID); err == nil {
		t.Fatal("other owner revoked")
	}
	if err = s.Revoke(ctx, issue.OrganizationID, issue.OwnerID, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Consume(ctx, consumeRequest(issue)); err == nil {
		t.Fatal("revoked accepted")
	}
	if _, err = s.Issue(ctx, issue); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err = s.Consume(ctx, consumeRequest(issue)); err == nil {
		t.Fatal("expired accepted")
	}
}
func TestConcurrentBudgetAndRestart(t *testing.T) {
	s, issue, now := fixture(t)
	issue.MaxInvocations = 3
	ctx := context.Background()
	if _, err := s.Issue(ctx, issue); err != nil {
		t.Fatal(err)
	}
	restored, err := New(s.db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := restored.Consume(ctx, consumeRequest(issue)); err == nil {
				calls.Add(1)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 3 {
		t.Fatalf("authorized %d, want 3", calls.Load())
	}
}
