package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelChannelBoundedFileEndpoint(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "test-model-file-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.store.Close(); err != nil {
			t.Errorf("close test store: %v", err)
		}
	})
	request := func(method, path, body, authHeader string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if authHeader == "Authorization" {
			r.Header.Set(authHeader, "Bearer "+cfg.APIKey)
		} else if authHeader != "" {
			r.Header.Set(authHeader, cfg.APIKey)
		}
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}
	created := request(http.MethodPost, "/v1/sandboxes", `{"runtime_profile":"session","ttl_seconds":60,"organization_id":"org-a"}`, "X-API-Key")
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	var box struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &box); err != nil {
		t.Fatal(err)
	}
	upload := request(http.MethodPost, "/v1/files/upload", fmt.Sprintf(`{"sandbox_id":%q,"organization_id":"org-a","path":"request.json","content":"12345"}`, box.Data.ID), "X-API-Key")
	if upload.Code != 200 {
		t.Fatal(upload.Body.String())
	}
	base := "/v1/files/download?sandbox_id=" + box.Data.ID + "&path=request.json&encoding=base64"
	for _, tc := range []struct {
		name, query, authHeader string
		status                  int
	}{
		{"at-limit", "&organization_id=org-a&max_bytes=5", "X-API-Key", http.StatusOK},
		{"over-limit", "&organization_id=org-a&max_bytes=4", "X-API-Key", http.StatusBadRequest},
		{"zero-limit", "&organization_id=org-a&max_bytes=0", "X-API-Key", http.StatusBadRequest},
		{"invalid-limit", "&organization_id=org-a&max_bytes=invalid", "X-API-Key", http.StatusBadRequest},
		{"cross-organization", "&organization_id=org-b&max_bytes=5", "X-API-Key", http.StatusForbidden},
		{"unauthenticated", "&organization_id=org-a&max_bytes=5", "", http.StatusUnauthorized},
		{"bearer-not-legacy-key", "&organization_id=org-a&max_bytes=5", "Authorization", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := request(http.MethodGet, base+tc.query, "", tc.authHeader)
			if got.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %s", got.Code, tc.status, got.Body.String())
			}
			if tc.status == http.StatusOK && (!strings.Contains(got.Body.String(), `"max_bytes":5`) || !strings.Contains(got.Body.String(), `"content":"MTIzNDU="`)) {
				t.Fatalf("bounded response missing limit or exact content: %s", got.Body.String())
			}
		})
	}
}
