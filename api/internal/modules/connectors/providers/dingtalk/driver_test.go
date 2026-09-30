package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(s string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s)), Header: http.Header{}}
}
func TestVerificationNeverSendsAndSendPreservesPartialReceipt(t *testing.T) {
	sends := 0
	c, err := New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			return response(`{"accessToken":"derived-private","expireIn":7200}`), nil
		}
		sends++
		if r.URL.Path != "/v1.0/robot/oToMessages/batchSend" || r.Header.Get("x-acs-dingtalk-access-token") != "derived-private" {
			t.Fatal("wire")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["robotCode"] != "app1" || body["msgKey"] != "sampleText" {
			t.Fatal("body")
		}
		var param map[string]string
		if json.Unmarshal([]byte(body["msgParam"].(string)), &param) != nil || param["content"] != "hello" {
			t.Fatal("message")
		}
		return response(`{"processQueryKey":"receipt-1","invalidStaffIdList":["u2"]}`), nil
	})}, true)
	if err != nil {
		t.Fatal(err)
	}
	input := providers.CredentialInput{Values: map[string]string{"client_id": "app1", "client_secret": "private-secret"}}
	v, err := c.VerifyCredentials(context.Background(), input)
	if err != nil || sends != 0 || v.Evidence.Messaging.Operations[0].Availability != "conditional" {
		t.Fatal(err)
	}
	r, err := c.InvokeCredentials(context.Background(), "dingtalk.messages.send_direct", input, map[string]any{"user_ids": []string{"u1", "u2"}, "text": "hello"})
	if err != nil || sends != 1 || r.Value["status"] != "partial" {
		t.Fatal(r, err)
	}
}
func TestDisabledAndUnknownWriteAreNeverRetried(t *testing.T) {
	calls := 0
	c, _ := New(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.Contains(r.URL.Path, "accessToken") {
			return response(`{"accessToken":"derived","expireIn":60}`), nil
		}
		return nil, errors.New("transport")
	})}, false)
	in := providers.CredentialInput{Values: map[string]string{"client_id": "app1", "client_secret": "secret"}}
	if _, err := c.VerifyCredentials(context.Background(), in); providers.FailureOf(err).Code != core.ErrorCodeConnectorUnavailable || calls != 0 {
		t.Fatal("gate")
	}
	c.enabled = true
	_, err := c.InvokeCredentials(context.Background(), "dingtalk.messages.send_group", in, map[string]any{"conversation_id": "cid1", "text": "hello"})
	if !providers.FailureOf(err).Unknown || calls != 2 {
		t.Fatal("write retried")
	}
}
