package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(v any) *http.Response {
	raw, _ := json.Marshal(v)
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}
}

type conversationStore struct{}

func (conversationStore) Update(context.Context, providers.ResourceScope, string, string, uuid.UUID, func(context.Context, *messaging.State) error) error {
	return errors.New("unexpected state update")
}
func (conversationStore) Put(context.Context, providers.ResourceScope, string, messaging.Conversation) (string, error) {
	return "", errors.New("unexpected context creation")
}
func (conversationStore) Resolve(_ context.Context, _ providers.ResourceScope, provider, handle string) (messaging.Conversation, error) {
	if provider != "wecom" || handle != "scoped-handle" {
		return messaging.Conversation{}, failure(core.ErrorCodeResourceAccessDenied)
	}
	return messaging.Conversation{Recipient: "private-chat"}, nil
}

func TestSendRefreshesOnlyExplicitTokenRefusal(t *testing.T) {
	for _, mode := range []string{"accepted", "refused_once", "ambiguous", "refused_twice"} {
		t.Run(mode, func(t *testing.T) {
			tokens, sends := 0, 0
			c, _ := New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "get_cli_config") {
					tokens++
					return response(map[string]any{"errcode": 0, "token": "derived-" + strconv.Itoa(tokens)}), nil
				}
				if r.URL.Path != "/cli/message/aibot/send" {
					t.Fatal("unexpected endpoint")
				}
				sends++
				var outer map[string]string
				if json.NewDecoder(r.Body).Decode(&outer) != nil {
					t.Fatal("payload envelope")
				}
				var body map[string]any
				if json.Unmarshal([]byte(outer["payload"]), &body) != nil || body["chat_id"] != "private-chat" || body["msg_type"] != "markdown" || body["markdown"].(map[string]any)["content"] != "approved message" {
					t.Fatal("send did not use scoped context")
				}
				if r.Header.Get("Authorization") != "Bearer derived-"+strconv.Itoa(tokens) {
					t.Fatal("stale derived token")
				}
				if mode == "ambiguous" {
					return nil, errors.New("lost response")
				}
				if mode == "refused_twice" || mode == "refused_once" && sends == 1 {
					return response(map[string]any{"errcode": 853004}), nil
				}
				return response(envelope(map[string]any{"success": true})), nil
			})}, true, conversationStore{})
			out, err := c.InvokeCredentials(context.Background(), "wecom.messages.send", providers.CredentialInput{Values: map[string]string{"bot_id": "bot1", "secret": "private-secret"}}, map[string]any{"context_handle": "scoped-handle", "text": "approved message"})
			if mode == "ambiguous" {
				if err == nil || !providers.FailureOf(err).Unknown || sends != 1 || tokens != 1 {
					t.Fatal("ambiguous write was retried", sends, tokens, err)
				}
			} else if mode == "refused_twice" {
				if err == nil || sends != 2 || tokens != 2 {
					t.Fatal("token refresh was not bounded", sends, tokens, err)
				}
			} else {
				want := 1
				if mode == "refused_once" {
					want = 2
				}
				if err != nil || sends != want || tokens != want || out.Value == nil {
					t.Fatal("send", sends, tokens, err)
				}
			}
		})
	}
}
func envelope(data any) any {
	value, _ := json.Marshal(data)
	inner, _ := json.Marshal(map[string]string{"result": string(value)})
	return map[string]any{"errcode": 0, "results_json": string(inner)}
}
func TestVerificationUsesSignedBootstrapAndRealBusinessReads(t *testing.T) {
	paths := []string{}
	c, err := New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(r.URL.Path, "get_cli_config") {
			if r.Header.Get("Authorization") != "" || body["bind_source"] != float64(1) {
				t.Fatal("bootstrap auth")
			}
			sum := sha256.Sum256([]byte("private-secretbot1" + strconv.FormatInt(int64(body["time"].(float64)), 10) + body["nonce"].(string)))
			if body["signature"] != hex.EncodeToString(sum[:]) {
				t.Fatal("signature")
			}
			return response(map[string]any{"errcode": 0, "token": "derived-private"}), nil
		}
		if r.Header.Get("Authorization") != "Bearer derived-private" || body["payload"] != "{}" {
			t.Fatal("business envelope")
		}
		if r.URL.Path == "/cli/identity/whoami" {
			return response(envelope(map[string]string{"extra_identity_context": "Provider authorization description"})), nil
		}
		if r.URL.Path == "/cli/message/aibot/sessions/list" {
			return response(envelope(map[string]any{"sessions": []any{}})), nil
		}
		t.Fatal("unexpected operation", r.URL.Path)
		return nil, nil
	})}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.VerifyCredentials(context.Background(), providers.CredentialInput{Values: map[string]string{"bot_id": "bot1", "secret": "private-secret"}})
	if err != nil || len(paths) != 3 || v.AccountID != "wecom:bot:bot1" || v.Evidence.Messaging.Operations[2].Availability != "context_required" {
		t.Fatal(err)
	}
}
func TestPublicSchemaAndHTTP200AreNotVerification(t *testing.T) {
	for _, body := range []any{map[string]any{"errcode": 0}, map[string]any{"errcode": 853004}, envelope(map[string]any{"schemas": map[string]any{}})} {
		calls := 0
		c, _ := New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return response(map[string]any{"errcode": 0, "token": "derived"}), nil
			}
			return response(body), nil
		})}, true, nil)
		_, err := c.VerifyCredentials(context.Background(), providers.CredentialInput{Values: map[string]string{"bot_id": "bot1", "secret": "private-secret"}})
		if err == nil || providers.FailureOf(err).Code == core.ErrorCodeOutcomeUnknown {
			t.Fatal("false verification")
		}
	}
}
