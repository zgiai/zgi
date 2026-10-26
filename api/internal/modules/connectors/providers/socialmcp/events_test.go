package socialmcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialevents"
	"github.com/zgiai/zgi/api/pkg/mcpclient"
)

type eventStoreFixture struct {
	calls, acks      int
	org, owner, conn uuid.UUID
	after            int64
	limit            int
	ctx              context.Context
	page             socialevents.Page
	err              error
}

func (f *eventStoreFixture) List(ctx context.Context, org, owner, conn uuid.UUID, after int64, limit int) (socialevents.Page, error) {
	f.calls++
	f.ctx = ctx
	f.org, f.owner, f.conn, f.after, f.limit = org, owner, conn, after, limit
	return f.page, f.err
}
func (f *eventStoreFixture) Acknowledge(ctx context.Context, org, owner, conn uuid.UUID, through int64) error {
	f.acks++
	f.ctx = ctx
	f.org, f.owner, f.conn, f.after = org, owner, conn, through
	return f.err
}
func eventClient(t *testing.T, id string, store *eventStoreFixture) *Client {
	t.Helper()
	c, err := New(id, Options{Events: store})
	if err != nil {
		t.Fatal(err)
	}
	c.open = func(context.Context, string) (*mcpclient.Connection, func(), error) {
		t.Fatal("local operation started bridge")
		return nil, nil, nil
	}
	return c
}
func eventScope() providers.ResourceScope {
	return providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New(), CredentialVersion: 1}
}
func TestLocalEventsUseOnlyRuntimeScope(t *testing.T) {
	f := &eventStoreFixture{page: socialevents.Page{Items: []socialevents.Event{}, NextAfterID: 7}}
	c := eventClient(t, "facebook-social", f)
	scope := eventScope()
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "request")
	result, err := c.InvokeWithScope(ctx, "facebook-social.event.list", "private-token", map[string]any{"after_id": 7, "limit": 2}, scope)
	if err != nil || result.Value["transport"] != "local" || result.Value["provider"] != "facebook-social" {
		t.Fatalf("local result: %#v %v", result, err)
	}
	if f.calls != 1 || f.org != scope.OrganizationID || f.owner != scope.OwnerAccountID || f.conn != scope.ConnectionID || f.after != 7 || f.limit != 2 || f.ctx.Value(key{}) != "request" {
		t.Fatal("lost authorized scope or context")
	}
	if _, err = c.InvokeWithScope(ctx, "facebook-social.event.ack", "private-token", map[string]any{"through_id": 7}, scope); err != nil || f.acks != 1 || f.after != 7 {
		t.Fatalf("ack: %v", err)
	}
}
func TestLocalEventsRejectUnscopedForgedAndForeignInputs(t *testing.T) {
	f := &eventStoreFixture{}
	c := eventClient(t, "facebook-social", f)
	if _, err := c.Invoke(t.Context(), "facebook-social.event.list", "token", nil); err == nil {
		t.Fatal("accepted missing scope")
	}
	for _, input := range []map[string]any{{"owner_account_id": uuid.NewString()}, {"organization_id": uuid.NewString()}, {"account_id": "other"}, {"connection_id": uuid.NewString()}, {"limit": 21}, {"after_id": -1}, {"after_id": 9007199254740992.0}, {"after_id": 0.5}} {
		if _, err := c.InvokeWithScope(t.Context(), "facebook-social.event.list", "token", input, eventScope()); err == nil {
			t.Fatal("accepted spoofed/invalid args")
		}
	}
	for _, input := range []map[string]any{nil, {"through_id": 0}, {"through_id": 9007199254740992.0}} {
		if _, err := c.InvokeWithScope(t.Context(), "facebook-social.event.ack", "token", input, eventScope()); err == nil {
			t.Fatal("accepted invalid ack")
		}
	}
	for _, scope := range []providers.ResourceScope{{}, {OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New()}} {
		if _, err := c.InvokeWithScope(t.Context(), "facebook-social.event.list", "token", nil, scope); err == nil {
			t.Fatal("accepted invalid scope")
		}
	}
	for _, id := range []string{"x-social", "linkedin-social"} {
		if _, err := eventClient(t, id, f).InvokeWithScope(t.Context(), id+".event.list", "token", nil, eventScope()); err == nil {
			t.Fatal("foreign provider accepted events")
		}
	}
	if f.calls != 0 || f.acks != 0 {
		t.Fatal("rejected operation reached store")
	}
}
func TestLocalEventOversizeAndFailureSafe(t *testing.T) {
	f := &eventStoreFixture{page: socialevents.Page{Items: []socialevents.Event{{ID: 1, Payload: json.RawMessage(`{"text":"` + strings.Repeat("x", 61<<10) + `"}`)}}}}
	c := eventClient(t, "facebook-social", f)
	if _, err := c.InvokeWithScope(t.Context(), "facebook-social.event.list", "token", nil, eventScope()); err == nil || providers.FailureOf(err).Code != core.ErrorCodeProviderUnavailable {
		t.Fatalf("oversized output accepted: %v", err)
	}
	if f.acks != 0 {
		t.Fatal("oversized events acknowledged")
	}
	f.err = core.NewError(core.ErrorCodeAccessDenied, false, nil)
	if _, err := c.InvokeWithScope(t.Context(), "facebook-social.event.list", "token", nil, eventScope()); err == nil || providers.FailureOf(err).Code != core.ErrorCodeAccessDenied {
		t.Fatalf("unsafe service error: %v", err)
	}
}
func TestScopedRemoteOperationsRetainBridge(t *testing.T) {
	c, calls := fixture(t, "", map[string]any{"id": "ok"}, false)
	if _, err := c.InvokeWithScope(t.Context(), "x-social.post.create", "token", map[string]any{"text": "hi"}, eventScope()); err != nil || *calls != 1 {
		t.Fatalf("remote operation: %v", err)
	}
}

func TestVerifyLocalEventsOnlyAfterAccountVerification(t *testing.T) {
	c, _ := fixtureForProvider(t, "facebook-social", "", map[string]any{"id": "ok"}, false)
	assertLocal := func(want bool) {
		t.Helper()
		verified, err := c.Verify(t.Context(), "token")
		if err != nil {
			t.Fatal(err)
		}
		for _, op := range []string{"facebook-social.event.list", "facebook-social.event.ack"} {
			found := false
			for _, got := range verified.Operations {
				if got == op {
					found = true
				}
			}
			if found != want {
				t.Fatalf("local operation availability %s=%v want %v", op, found, want)
			}
		}
	}
	assertLocal(false)
	c.options.Events = &eventStoreFixture{}
	assertLocal(true)
	c.open = func(context.Context, string) (*mcpclient.Connection, func(), error) {
		return nil, func() {}, providers.Invalid()
	}
	if verified, err := c.Verify(t.Context(), "token"); err == nil || len(verified.Operations) != 0 {
		t.Fatal("failed verification exposed local operations")
	}
}

func TestLocalEventCatalogContract(t *testing.T) {
	for _, id := range []string{"x-social", "linkedin-social", "facebook-social"} {
		c, err := New(id, Options{})
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, op := range c.definition.Operations {
			if !localEventOperation(op.ID) {
				continue
			}
			count++
			if op.DataEgress || !op.Idempotent || op.RiskHint != "low" || (op.ID == "facebook-social.event.ack" && op.Effect != "update") {
				t.Fatalf("incorrect local policy: %#v", op)
			}
		}
		if id == "facebook-social" && count != 2 {
			t.Fatal("missing local operations")
		}
		if id != "facebook-social" && count != 0 {
			t.Fatal("foreign local operations")
		}
	}
}
