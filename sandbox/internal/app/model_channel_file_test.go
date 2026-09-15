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
	request := func(method, path, body string, authorize bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if authorize {
			r.Header.Set("Authorization", "Bearer test-model-file-key")
		}
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}
	created := request("POST", "/v1/sandboxes", `{"runtime_profile":"session","ttl_seconds":60,"organization_id":"org-a"}`, true)
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
	upload := request("POST", "/v1/files/upload", fmt.Sprintf(`{"sandbox_id":%q,"organization_id":"org-a","path":"request.json","content":"12345"}`, box.Data.ID), true)
	if upload.Code != 200 {
		t.Fatal(upload.Body.String())
	}
	base := "/v1/files/download?sandbox_id=" + box.Data.ID + "&path=request.json&encoding=base64"
	for _, tc := range []struct {
		query       string
		auth, valid bool
	}{
		{"&organization_id=org-a&max_bytes=5", true, true},
		{"&organization_id=org-a&max_bytes=4", true, false},
		{"&organization_id=org-a&max_bytes=0", true, false},
		{"&organization_id=org-a&max_bytes=invalid", true, false},
		{"&organization_id=org-b&max_bytes=5", true, false},
		{"&organization_id=org-a&max_bytes=5", false, false},
	} {
		got := request(http.MethodGet, base+tc.query, "", tc.auth)
		if tc.valid {
			if got.Code != 200 || !strings.Contains(got.Body.String(), `"max_bytes":5`) {
				t.Fatal(got.Code, got.Body.String())
			}
		} else if got.Code == 200 {
			t.Fatal("invalid bounded read authorized", tc.query)
		}
	}
}
