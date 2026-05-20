package httprequest

import (
	"testing"
)

// TestHTTPRequestExamples tests all HTTP request examples
func TestHTTPRequestExamples(t *testing.T) {
	example := NewHTTPRequestExample()

	t.Run("SimpleGETRequest", func(t *testing.T) {
		// Test simple GET request
		example.SimpleGETRequest()
	})

	t.Run("POSTJSONRequest", func(t *testing.T) {
		// Test POST JSON request
		example.POSTJSONRequest()
	})

	t.Run("FormDataRequest", func(t *testing.T) {
		// Test form data request
		example.FormDataRequest()
	})

	t.Run("AuthorizedRequest", func(t *testing.T) {
		// Test authorized request
		example.AuthorizedRequest()
	})

	t.Run("BasicAuthRequest", func(t *testing.T) {
		// Test Basic authentication request
		example.BasicAuthRequest()
	})
}

// TestHTTPRequestAdvancedFeatures tests advanced features
func TestHTTPRequestAdvancedFeatures(t *testing.T) {
	example := NewHTTPRequestExample()

	t.Run("CustomTimeoutRequest", func(t *testing.T) {
		// Test custom timeout
		example.CustomTimeoutRequest()
	})

	t.Run("ErrorHandlingExample", func(t *testing.T) {
		// Test error handling
		example.ErrorHandlingExample()
	})

	t.Run("FactoryExample", func(t *testing.T) {
		// Test factory pattern
		example.FactoryExample()
	})
}

// TestAllHTTPRequestExamples runs complete test for all examples
func TestAllHTTPRequestExamples(t *testing.T) {
	example := NewHTTPRequestExample()
	example.RunAllExamples()
}

// BenchmarkSimpleGETRequest performance test example
func BenchmarkSimpleGETRequest(b *testing.B) {
	example := NewHTTPRequestExample()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		example.SimpleGETRequest()
	}
}

// TestHTTPRequestNodeIntegration integration test - tests complete execution flow of HTTP request node
func TestHTTPRequestNodeIntegration(t *testing.T) {
	// Here can test complete execution flow of HTTPRequestNode
	// Including node creation, execution, event handling, etc.
	t.Skip("Integration test requires complete node environment, skipping temporarily")
}

func TestHTTPRequestNodeBuildInputSnapshot(t *testing.T) {
	node := &HTTPRequestNode{
		NodeData: NodeData{
			Method:  HTTPMethodPost,
			URL:     "https://example.com/api",
			Headers: "Authorization: secret\nX-Trace: abc",
			Params:  "q: test\npage: 1",
			Authorization: HttpRequestNodeAuthorization{
				Type: AuthorizationTypeAPIKey,
				Config: &HttpRequestNodeAuthorizationConfig{
					Type:   AuthorizationConfigTypeBearer,
					APIKey: "secret",
					Header: "Authorization",
				},
			},
			Body: &HttpRequestNodeBody{
				Type: BodyTypeJSON,
				Data: []BodyData{{Type: BodyDataTypeText, Value: `{"hello":"world"}`}},
			},
		},
	}

	inputs := node.buildInputSnapshot()
	if got := inputs["url"]; got != "https://example.com/api" {
		t.Fatalf("url = %#v, want configured url", got)
	}
	if got := inputs["method"]; got != "POST" {
		t.Fatalf("method = %#v, want POST", got)
	}
	headers, ok := inputs["header"].(map[string]any)
	if !ok || headers["X-Trace"] != "abc" {
		t.Fatalf("header = %#v, want parsed headers", inputs["header"])
	}
	params, ok := inputs["param"].(map[string]any)
	if !ok || params["q"] != "test" || params["page"] != "1" {
		t.Fatalf("param = %#v, want parsed params", inputs["param"])
	}
	auth, ok := inputs["auth"].(map[string]any)
	if !ok || auth["type"] != AuthorizationTypeAPIKey {
		t.Fatalf("auth = %#v, want non-secret auth metadata", inputs["auth"])
	}
	config, ok := auth["config"].(map[string]any)
	if !ok {
		t.Fatalf("auth config = %#v, want map", auth["config"])
	}
	if _, exists := config["api_key"]; exists {
		t.Fatalf("auth config should not expose api_key: %#v", config)
	}
	if inputs["body"] == nil {
		t.Fatalf("body should preserve configured body content")
	}
}

func TestHTTPRequestNodeBuildInputSnapshotDefaultsEmptyObjects(t *testing.T) {
	node := &HTTPRequestNode{
		NodeData: NodeData{
			Method:        HTTPMethodGET,
			URL:           "http://baidu.com",
			Authorization: HttpRequestNodeAuthorization{Type: AuthorizationTypeNoAuth},
		},
	}

	inputs := node.buildInputSnapshot()
	if got := inputs["header"]; len(got.(map[string]any)) != 0 {
		t.Fatalf("header = %#v, want empty map", got)
	}
	if got := inputs["param"]; len(got.(map[string]any)) != 0 {
		t.Fatalf("param = %#v, want empty map", got)
	}
	if inputs["body"] != nil {
		t.Fatalf("body = %#v, want nil", inputs["body"])
	}
	if inputs["auth"] != nil {
		t.Fatalf("auth = %#v, want nil", inputs["auth"])
	}
}
