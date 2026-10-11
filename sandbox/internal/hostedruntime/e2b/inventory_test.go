package e2b

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInventoryIsReadOnlyIncludesPausedAndPaginates(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/v2/sandboxes" || r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Query().Get("state") + r.URL.Query().Get("nextToken") {
		case "running":
			w.Header().Set("X-Next-Token", "page-2")
			_, _ = w.Write([]byte(`[{"sandboxID":"one","state":"running","metadata":{"zgi.binding_id":"b1","sensitive":"not-returned"}}]`))
		case "runningpage-2":
			_, _ = w.Write([]byte(`[{"sandboxID":"unmanaged","state":"running","metadata":{}}]`))
		case "paused":
			_, _ = w.Write([]byte(`[{"sandboxID":"two","state":"paused","metadata":{"zgi.binding_id":"b2"}}]`))
		default:
			t.Error("unexpected pagination")
		}
	}))
	defer server.Close()
	backend := &Backend{client: &controlClient{apiURL: server.URL, apiKey: "test-key", httpClient: server.Client()}}
	resources, err := backend.Inventory(t.Context())
	if err != nil || len(resources) != 2 || calls != 3 || resources[1].State != "paused" {
		t.Fatalf("inventory=%+v calls=%d err=%v", resources, calls, err)
	}
}

func TestInventoryFailsRatherThanReportingPartialSuccess(t *testing.T) {
	for _, mode := range []string{"unauthorized", "repeat-page", "malformed", "missing-state"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "unauthorized":
					w.WriteHeader(401)
				case "repeat-page":
					w.Header().Set("X-Next-Token", "same")
					_, _ = w.Write([]byte(`[]`))
				case "malformed":
					_, _ = w.Write([]byte(`{}`))
				case "missing-state":
					_, _ = w.Write([]byte(`[{"sandboxID":"one","metadata":{"zgi.binding_id":"b1"}}]`))
				}
			}))
			defer server.Close()
			backend := &Backend{client: &controlClient{apiURL: server.URL, apiKey: "test-key", httpClient: server.Client()}}
			if _, err := backend.Inventory(t.Context()); err == nil {
				t.Fatal("partial or invalid inventory accepted")
			}
		})
	}
}

func TestInventoryDoesNotForwardCredentialsOnRedirect(t *testing.T) {
	var forwarded bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true; _, _ = w.Write([]byte(`[]`)) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer server.Close()
	backend := &Backend{client: &controlClient{apiURL: server.URL, apiKey: "test-key", httpClient: server.Client()}}
	if _, err := backend.Inventory(t.Context()); err == nil || forwarded {
		t.Fatalf("redirect followed=%v error=%v", forwarded, err)
	}
}
