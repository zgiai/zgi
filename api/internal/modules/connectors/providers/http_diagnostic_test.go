package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPBodyDeadlineRecordsWholeAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()
	client := NewHTTP(&http.Client{Timeout: 20 * time.Millisecond}, server.URL)
	var out map[string]any
	_, err := client.JSON(WithDiagnosticPhase(context.Background(), "operation"), "GET", "/fixture", nil, http.Header{}, nil, &out)
	if err == nil {
		t.Fatal("deadline accepted")
	}
	failure := FailureOf(err)
	d := failure.SafeDiagnostic()
	if !failure.Unknown || d == nil || d.Category != "deadline" || d.HTTPStatus != 200 || !d.ResponseReceived || !d.RequestWritten || d.ElapsedMS < 10 {
		t.Fatal("body deadline metadata missing", d)
	}
}
func TestJSONObjectRejectsDuplicateAndNonObjectEnvelope(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"x":1,"x":2}`, `{"a":{"x":1,"x":2}}`, `{} {}`} {
		if ValidJSONObject([]byte(raw)) {
			t.Fatal("ambiguous envelope accepted")
		}
	}
	if !ValidJSONObject([]byte(`{"ret":0,"msgs":[]}`)) {
		t.Fatal("valid envelope rejected")
	}
}
