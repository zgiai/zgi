package container

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
)

type socialGatewayResolverStub struct{ capability runtimeauth.ChatCapability }

func (s socialGatewayResolverStub) ResolveDigitalHumanCapabilities(context.Context, runtimeauth.DigitalHumanCapabilityRequest) (runtimeauth.ChatCapabilitySnapshot, error) {
	return runtimeauth.ChatCapabilitySnapshot{Capabilities: []runtimeauth.ChatCapability{s.capability}}, nil
}
func (s socialGatewayResolverStub) ResolveConnectorIdentity(_ context.Context, r connectordispatch.IdentityResolutionRequest) (connectordispatch.Identity, error) {
	return connectordispatch.Identity{ConversationID: r.ConversationID, OrganizationID: r.OrganizationID, OwnerAccountID: r.OwnerAccountID,
		ActingPrincipalID: r.ActingPrincipalID, BindingID: s.capability.BindingID, ConnectionID: s.capability.ConnectionID,
		ConnectorID: r.ConnectorID, Target: r.Target, OperationID: r.OperationID, LogicalOperationID: r.LogicalOperationID,
		CorrelationID: r.CorrelationID, Interactive: r.Interactive}, nil
}

type socialGatewayDispatcherStub struct {
	results  map[string]any
	calls    []connectordispatch.Invocation
	contexts []context.Context
}

func (s *socialGatewayDispatcherStub) Dispatch(ctx context.Context, in connectordispatch.Invocation) (execution.ExecutionResult, error) {
	s.calls = append(s.calls, in)
	s.contexts = append(s.contexts, ctx)
	return execution.ExecutionResult{State: execution.ExecutionStateCompleted, Value: s.results[in.Identity.OperationID.String()]}, nil
}

type socialGatewayConnectionStub struct {
	account                 string
	id, organization, owner uuid.UUID
	connector               connectorcore.Identifier
}

func (s socialGatewayConnectionStub) GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, error) {
	return connectorcore.PersonalConnection{ID: s.id, OrganizationID: s.organization, OwnerAccountID: s.owner, ConnectorID: s.connector, ExternalAccountID: &s.account}, nil
}

func socialGatewayFixture(platform string, operations ...string) (*socialOperationsGateway, socialops.Publication, *socialGatewayDispatcherStub) {
	org, workspace, owner, employee, version, conversation, connection, binding := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	connector := connectorcore.Identifier(platform + "-social")
	ops := make([]catalog.Operation, len(operations))
	for i, operation := range operations {
		ops[i].ID = connector.String() + "." + operation
	}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeDigitalHuman, ID: employee.String(), Scope: connectorcore.ScopePublished, VersionID: version.String()}
	capability := runtimeauth.ChatCapability{Target: target, BindingID: binding, ConnectionID: connection, OwnerAccountID: owner, ConnectorID: connector, Operations: ops}
	dispatcher := &socialGatewayDispatcherStub{results: map[string]any{}}
	message := uuid.New()
	restored := ActiveBackgroundTaskRun{Run: digitalhuman.Run{ID: uuid.New(), OrganizationID: org, WorkspaceID: workspace, DigitalHumanID: employee, DigitalHumanVersionID: version, ConversationID: &conversation, MessageID: &message}, WorkspaceID: workspace, OwnerID: owner}
	gateway, _ := newSocialOperationsGatewayWithDependencies(restored, socialGatewayResolverStub{capability}, dispatcher, socialGatewayConnectionStub{account: "acct-7", id: connection, organization: org, owner: owner, connector: connector}, nil)
	p := socialops.Publication{ID: uuid.New(), Scope: socialops.Scope{OrganizationID: org, WorkspaceID: workspace, OwnerID: owner, EmployeeID: employee}, ConnectionID: connection, Platform: platform, State: "dispatching", Revision: 3, RemoteID: "remote-1", ParentRemoteID: "parent-1"}
	return gateway, p, dispatcher
}

func TestSocialOperationsGatewayNormalizesNativeReadback(t *testing.T) {
	published := "2026-10-19T10:11:12Z"
	for _, test := range []struct {
		platform string
		data     map[string]any
	}{
		{"x", map[string]any{"id": "remote-1", "text": "hello", "author_id": "acct-7", "url": "https://x.com/acct/status/remote-1", "published_at": published, "parent_id": "parent-1", "media": []any{map[string]any{"id": "media-1", "url": "https://pbs.twimg.com/media/1.jpg"}}}},
		{"facebook", map[string]any{"id": "remote-1", "text": "hello", "author_id": "acct-7", "url": "https://facebook.com/remote-1", "published_at": published, "parent_id": "parent-1"}},
		{"linkedin", map[string]any{"id": "remote-1", "text": "hello", "author_id": "urn:li:organization:7", "url": "https://www.linkedin.com/feed/update/remote-1", "published_at": published, "parent_id": "urn:li:activity:11", "parent_post_id": "parent-1"}},
	} {
		t.Run(test.platform, func(t *testing.T) {
			operations := []string{"comment.get"}
			if test.platform == "linkedin" {
				operations = append(operations, "account.list")
			}
			g, p, dispatcher := socialGatewayFixture(test.platform, operations...)
			if test.platform == "linkedin" {
				dispatcher.results["linkedin-social.account.list"] = map[string]any{"provider": "linkedin-social", "transport": "mcp", "data": map[string]any{"accounts": []any{map[string]any{"id": "urn:li:organization:7", "name": "Org"}}}}
			}
			dispatcher.results[test.platform+"-social.comment.get"] = map[string]any{"provider": test.platform + "-social", "transport": "mcp", "data": test.data}
			got, err := g.Read(t.Context(), p)
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != "remote-1" || got.Text != "hello" || got.ParentID != "parent-1" || got.PublishedAt.IsZero() || got.URL == "" {
				t.Fatalf("readback = %#v", got)
			}
		})
	}
}

func TestSocialOperationsGatewayRejectsMismatchedAuthorAndParent(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(data map[string]any) { data["author_id"] = "other" },
		func(data map[string]any) { data["parent_id"] = "other" },
	} {
		g, p, dispatcher := socialGatewayFixture("x", "comment.get")
		data := map[string]any{"id": "remote-1", "text": "hello", "author_id": "acct-7", "url": "https://x.com/a/status/1", "published_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), "parent_id": "parent-1"}
		mutate(data)
		dispatcher.results["x-social.comment.get"] = map[string]any{"provider": "x-social", "transport": "mcp", "data": data}
		if _, err := g.Read(t.Context(), p); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSocialOperationsGatewayUsesConfirmedCreateReceipts(t *testing.T) {
	for _, platform := range []string{"x", "facebook", "linkedin"} {
		t.Run(platform, func(t *testing.T) {
			operations := []string{"post.create"}
			if platform == "linkedin" {
				operations = append(operations, "account.list")
			}
			g, p, dispatcher := socialGatewayFixture(platform, operations...)
			p.ParentRemoteID = ""
			p.Revision = 7
			if platform == "linkedin" {
				dispatcher.results["linkedin-social.account.list"] = map[string]any{"provider": "linkedin-social", "transport": "mcp", "data": map[string]any{"accounts": []any{map[string]any{"id": "urn:li:organization:7", "name": "Org"}}}}
			}
			payload := map[string]any{"id": "created-9"}
			if platform == "x" {
				payload = map[string]any{"data": map[string]any{"id": "created-9", "text": "hello"}}
			}
			dispatcher.results[platform+"-social.post.create"] = map[string]any{"provider": platform + "-social", "transport": "mcp", "data": payload}
			got, err := g.Send(t.Context(), p, socialops.DraftBundle{Action: "post", English: "hello", Chinese: "你好"})
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != "created-9" {
				t.Fatalf("receipt = %#v", got)
			}
			last := dispatcher.calls[len(dispatcher.calls)-1]
			background, ok := execution.BackgroundIdentityFromContext(dispatcher.contexts[len(dispatcher.contexts)-1])
			claim, claimed := dispatcher.contexts[len(dispatcher.contexts)-1].Value(socialPublicationClaimContextKey{}).(socialPublicationClaim)
			if !ok || background.RunID != g.restored.Run.ID || background.MessageID != *g.restored.Run.MessageID || !claimed || claim.RecordID != p.ID || claim.Revision != p.Revision {
				t.Fatalf("server claims = background %#v/%v publication %#v/%v", background, ok, claim, claimed)
			}
			if last.Input.(map[string]any)["text"] != "hello" || !strings.Contains(last.Identity.LogicalOperationID, ":7:post.create") {
				t.Fatalf("call = %#v", last)
			}
		})
	}
}

func TestSocialOperationsGatewayNormalizesXQuoteReceiptLinkage(t *testing.T) {
	g, p, dispatcher := socialGatewayFixture("x", "post.quote")
	p.ParentRemoteID = ""
	dispatcher.results["x-social.post.quote"] = map[string]any{"provider": "x-social", "transport": "mcp", "data": map[string]any{"data": map[string]any{"id": "quote-1", "text": "context"}, "quoted_id": "source-1"}}
	got, err := g.Send(t.Context(), p, socialops.DraftBundle{Action: "quote", English: "context", Chinese: "审核", TargetID: "source-1"})
	if err != nil || got.ID != "quote-1" || got.QuotedID != "source-1" {
		t.Fatalf("receipt=%#v err=%v", got, err)
	}
}

func TestSocialOperationsGatewayObservesExactXPostMetrics(t *testing.T) {
	g, p, dispatcher := socialGatewayFixture("x", "metrics.get")
	p.ParentRemoteID = ""
	dispatcher.results["x-social.metrics.get"] = map[string]any{"provider": "x-social", "transport": "mcp", "data": map[string]any{"data": map[string]any{
		"id": "remote-1", "author_id": "acct-7", "public_metrics": map[string]any{"like_count": float64(4), "reply_count": float64(2), "impression_count": "9007199254740993"},
	}}}
	got, err := g.ObserveMetrics(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	if got["public_metrics_like_count"] == nil || *got["public_metrics_like_count"] != 4 || got["public_metrics_reply_count"] == nil || *got["public_metrics_reply_count"] != 2 {
		t.Fatalf("metrics = %#v", got)
	}
	if got["public_metrics_impression_count"] == nil || *got["public_metrics_impression_count"] != 9007199254740993 {
		t.Fatalf("lossless metric = %#v", got)
	}
	for _, change := range []func(map[string]any){
		func(post map[string]any) { post["id"] = "other" },
		func(post map[string]any) { post["author_id"] = "other" },
	} {
		post := map[string]any{"id": "remote-1", "author_id": "acct-7", "public_metrics": map[string]any{"like_count": float64(4)}}
		change(post)
		dispatcher.results["x-social.metrics.get"] = map[string]any{"provider": "x-social", "transport": "mcp", "data": map[string]any{"data": post}}
		if _, err := g.ObserveMetrics(t.Context(), p); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSocialOperationsGatewayMarksOnlyPreDispatchFailureNotSubmitted(t *testing.T) {
	g, p, dispatcher := socialGatewayFixture("x", "post.get")
	p.ParentRemoteID = ""
	_, err := g.Send(t.Context(), p, socialops.DraftBundle{Action: "post", English: "hello", Chinese: "你好"})
	var marker *socialops.NotSubmittedError
	if !errors.As(err, &marker) || len(dispatcher.calls) != 0 {
		t.Fatalf("error = %v, calls = %d", err, len(dispatcher.calls))
	}
}

func TestSocialOperationsOutboundUsesExactSchemasAndRealReceipts(t *testing.T) {
	message := socialops.OutboundMessage{Text: "reviewed", Subject: "Subject"}
	tests := []struct {
		name, connector, operation, destination string
		value, wantInput                        map[string]any
		wantID, wantStatus                      string
	}{
		{"mail", "gmail", "gmail.message.send", "team@example.com", map[string]any{"provider": "gmail", "operation": "gmail.message.send", "status": "accepted_by_server", "message_id": "mail-1"}, map[string]any{"to": []string{"team@example.com"}, "subject": "Subject", "text": "reviewed"}, "mail-1", "accepted_by_server"},
		{"ding direct", "dingtalk", "dingtalk.messages.send_direct", "user-1", map[string]any{"provider": "dingtalk", "operation": "dingtalk.messages.send_direct", "status": "accepted", "receipt": "ding-1"}, map[string]any{"user_ids": []string{"user-1"}, "text": "reviewed", "format": "text"}, "ding-1", "accepted_by_platform"},
		{"feishu", "feishu", "feishu.message.send", "oc_group", map[string]any{"provider": "feishu", "operation": "feishu.message.send", "data": map[string]any{"message_id": "om_1"}}, map[string]any{"receive_id": "oc_group", "receive_id_type": "chat_id", "text": "reviewed"}, "om_1", "sent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := socialops.Outbound{Kind: "handoff", ConnectorID: test.connector, OperationID: test.operation, Destination: test.destination}
			input, err := outboundInput(out, message)
			if err != nil || !reflect.DeepEqual(input, test.wantInput) {
				t.Fatalf("input = %#v, %v", input, err)
			}
			receipt, err := normalizeOutboundReceipt(out, test.value)
			if err != nil || receipt.ID != test.wantID || receipt.Status != test.wantStatus {
				t.Fatalf("receipt = %#v, %v", receipt, err)
			}
			missing := map[string]any{"provider": test.connector, "operation": test.operation, "status": test.wantStatus, "data": map[string]any{}}
			if _, err = normalizeOutboundReceipt(out, missing); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeOutcomeUnknown) {
				t.Fatalf("missing real receipt = %v", err)
			}
		})
	}
}
