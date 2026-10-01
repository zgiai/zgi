package weixin

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
)

func TestSparseSendReceiptContract(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{}`, true}, {`{"ret":0}`, true}, {`{"errcode":0}`, true}, {`{"errmsg":""}`, true},
		{`null`, false}, {`[]`, false}, {`{"ret":null}`, false}, {`{"ret":"0"}`, false},
		{`{"ret":-14}`, false}, {`{"errcode":1}`, false}, {`{"ret":0,"ret":1}`, false}, {`{"unexpected":true}`, false},
	} {
		if err := validateSendReceipt([]byte(tc.body)); (err == nil) != tc.ok {
			t.Fatalf("receipt %s: %v", tc.body, err)
		}
	}
}

func TestPollingHintIsBoundedAndDoesNotInvalidateMessages(t *testing.T) {
	for _, hint := range []string{"45000", "1", "45001", "-1", "null", `"45000"`, "1.5"} {
		state := messaging.State{LongPollingTimeoutMS: 45000}
		calls := 0
		c, _ := New(&http.Client{Transport: reliabilityTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > 50*time.Second || time.Until(deadline) < 48*time.Second {
				t.Fatal("deadline outside budget")
			}
			return response(`{"msgs":[],"longpolling_timeout_ms":` + hint + `}`), nil
		})}, true, "fixture", nil)
		if err := c.pull(context.Background(), providers.CredentialInput{Values: map[string]string{"token": "fixture"}}, &state); err != nil {
			t.Fatal(err)
		}
		expected := 45000
		if hint == "1" {
			expected = 1
		}
		if calls != 1 || state.LongPollingTimeoutMS != expected {
			t.Fatal("unsafe hint persisted")
		}
	}
}
