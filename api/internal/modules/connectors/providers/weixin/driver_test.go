package weixin

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type memoryStore struct {
	state        messaging.State
	known        bool
	puts         int
	conversation messaging.Conversation
	scope        providers.ResourceScope
}

func (s *memoryStore) Update(ctx context.Context, scope providers.ResourceScope, _ string, _ string, flow uuid.UUID, fn func(context.Context, *messaging.State) error) error {
	if !s.known && flow == uuid.Nil {
		return failure(core.ErrorCodeConnectionNotReady)
	}
	raw, _ := json.Marshal(s.state)
	var candidate messaging.State
	_ = json.Unmarshal(raw, &candidate)
	if err := fn(ctx, &candidate); err != nil {
		return err
	}
	s.state = candidate
	s.known = true
	return nil
}
func (s *memoryStore) Put(_ context.Context, scope providers.ResourceScope, _ string, c messaging.Conversation) (string, error) {
	s.puts++
	s.conversation = c
	s.scope = scope
	return "fixture-handle", nil
}
func (s *memoryStore) Resolve(_ context.Context, scope providers.ResourceScope, _, handle string) (messaging.Conversation, error) {
	if handle != "fixture-handle" || scope != s.scope {
		return messaging.Conversation{}, failure(core.ErrorCodeAccessDenied)
	}
	return s.conversation, nil
}
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestVerificationBufferAndExplicitContextReply(t *testing.T) {
	store := &memoryStore{}
	calls := 0
	c, _ := New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Ilink-App-Id") != "admitted-fixture" || r.Header.Get("Authorization") != "Bearer PRIVATE_TOKEN" || r.Header.Get("X-WECHAT-UIN") == "" {
			t.Fatal("headers")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/ilink/bot/getupdates" {
			if body["get_updates_buf"] != "" {
				t.Fatal("cursor")
			}
			return response(`{"ret":0,"get_updates_buf":"PRIVATE_CURSOR","msgs":[{"message_id":123,"from_user_id":"PRIVATE_RECIPIENT","message_type":1,"message_state":2,"context_token":"PRIVATE_CONTEXT","item_list":[{"type":1,"text_item":{"text":"hello"}}]}]}`), nil
		}
		if r.URL.Path != "/ilink/bot/sendmessage" {
			t.Fatal("endpoint")
		}
		msg := body["msg"].(map[string]any)
		if msg["context_token"] != "PRIVATE_CONTEXT" || msg["to_user_id"] != "PRIVATE_RECIPIENT" {
			t.Fatal("context")
		}
		return response(`{"ret":0}`), nil
	})}, true, "admitted-fixture", store)
	in := providers.CredentialInput{Values: map[string]string{"account_id": "bot1", "token": "PRIVATE_TOKEN", "base_url": "https://ilinkai.weixin.qq.com"}, ResourceScope: providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New()}}
	if _, err := c.VerifyCredentials(context.Background(), in); err == nil || calls != 0 {
		t.Fatal("manual identity accepted")
	}
	ctx := core.WithRegistrationProof(context.Background(), core.RegistrationProof{OrganizationID: in.ResourceScope.OrganizationID, OwnerAccountID: in.ResourceScope.OwnerAccountID, FlowID: uuid.New(), ConnectorID: "weixin", Identity: "weixin:account:bot1", Values: in.Values})
	if _, err := c.VerifyCredentials(ctx, in); err != nil || store.puts != 0 || len(store.state.Messages) != 1 || store.state.Cursor != "PRIVATE_CURSOR" {
		t.Fatal("verification consumed context", err)
	}
	in.ResourceScope.CredentialVersion = 1
	in.ResourceScope.Target = core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}
	in.ResourceScope.BindingID = uuid.New()
	in.ResourceScope.BindingRevision = 1
	in.ResourceScope.LogicalOperationID = "pull-1"
	out, err := c.InvokeCredentials(context.Background(), "weixin.messages.pull", in, nil)
	if err != nil || calls != 1 || store.puts != 1 || len(store.state.Messages) != 0 {
		t.Fatal("explicit buffered pull", err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("private context reached model")
	}
	// Logical operation is not part of the context target identity.
	in.ResourceScope.LogicalOperationID = "reply-1"
	store.scope = in.ResourceScope
	if _, err = c.InvokeCredentials(context.Background(), "weixin.messages.reply", in, map[string]any{"context_handle": "fixture-handle", "text": "reply"}); err != nil || calls != 2 {
		t.Fatal(err)
	}
}
func TestTimeoutNeverCountsAsSuccessfulVerification(t *testing.T) {
	for _, body := range []string{"timeout", `{}`, `{"ret":-14}`, `{"ret":0,"get_updates_buf":""}`} {
		t.Run(body, func(t *testing.T) {
			store := &memoryStore{}
			calls := 0
			c, _ := New(&http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				calls++
				if body == "timeout" {
					return nil, context.DeadlineExceeded
				}
				return response(body), nil
			})}, true, "fixture", store)
			in := providers.CredentialInput{Values: map[string]string{"account_id": "a", "token": "secret", "base_url": "https://ilinkai.weixin.qq.com"}, ResourceScope: providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New()}}
			ctx := core.WithRegistrationProof(context.Background(), core.RegistrationProof{OrganizationID: in.ResourceScope.OrganizationID, OwnerAccountID: in.ResourceScope.OwnerAccountID, FlowID: uuid.New(), ConnectorID: "weixin", Identity: "weixin:account:a", Values: in.Values})
			_, err := c.VerifyCredentials(ctx, in)
			valid := body == `{"ret":0,"get_updates_buf":""}`
			if (err == nil) != valid || store.known != valid || calls != 1 {
				t.Fatal("false evidence")
			}
		})
	}
}
func TestUnknownReplyIsNotReplayed(t *testing.T) {
	scope := providers.ResourceScope{LogicalOperationID: "reply-2"}
	store := &memoryStore{scope: scope, conversation: messaging.Conversation{Recipient: "user", ContextToken: "context"}}
	calls := 0
	c, _ := New(&http.Client{Transport: transport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("wire failed") })}, true, "fixture", store)
	_, err := c.InvokeCredentials(context.Background(), "weixin.messages.reply", providers.CredentialInput{Values: map[string]string{"account_id": "a", "token": "secret", "base_url": "https://ilinkai.weixin.qq.com"}, ResourceScope: scope}, map[string]any{"context_handle": "fixture-handle", "text": "reply"})
	if err == nil || !providers.FailureOf(err).Unknown || calls != 1 {
		t.Fatal("ambiguous write replayed")
	}
}
