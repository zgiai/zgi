package httprequest

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

// TestRebuiltFactory tests the rebuilt factory.go functionality
func TestRebuiltFactory(t *testing.T) {
	t.Run("FactoryCreation", func(t *testing.T) {
		factory := NewHTTPRequestFactory()
		if factory == nil {
			t.Fatal("Factory creation failed")
		}
	})

	t.Run("ProcessorCreation", func(t *testing.T) {
		factory := NewHTTPRequestFactory()
		nodeData := &NodeData{
			Method: HTTPMethodGET,
			URL:    "https://httpbin.org/get",
			Authorization: HttpRequestNodeAuthorization{
				Type: AuthorizationTypeNoAuth,
			},
		}

		variablePool := entities.NewVariablePool()
		processor := factory.CreateProcessor(nodeData, nil, variablePool, 0, nil)

		if processor == nil {
			t.Fatal("Processor creation failed")
		}
	})

	t.Run("ExecutorCreation", func(t *testing.T) {
		nodeData := &NodeData{
			Method: HTTPMethodGET,
			URL:    "https://httpbin.org/get",
			Authorization: HttpRequestNodeAuthorization{
				Type: AuthorizationTypeNoAuth,
			},
		}

		variablePool := entities.NewVariablePool()
		executor := NewHTTPRequestExecutor(nodeData, variablePool, nil, 3, nil)

		if executor == nil {
			t.Fatal("Executor creation failed")
		}

		// Test actual execution
		ctx := context.Background()
		response, err := executor.Execute(ctx)
		if err != nil {
			t.Fatalf("Executor execution failed: %v", err)
		}

		if response.StatusCode != 200 {
			t.Errorf("Expected status code 200, got %d", response.StatusCode)
		}

		t.Logf("Response received: %d bytes", response.Size())
	})

	t.Run("ResultBuilder", func(t *testing.T) {
		builder := NewHTTPRequestResultBuilder()
		if builder == nil {
			t.Fatal("Result builder creation failed")
		}

		// Create mock response and executor
		nodeData := &NodeData{
			Method: HTTPMethodGET,
			URL:    "https://httpbin.org/get",
			Authorization: HttpRequestNodeAuthorization{
				Type: AuthorizationTypeNoAuth,
			},
		}

		variablePool := entities.NewVariablePool()
		executor := NewHTTPRequestExecutor(nodeData, variablePool, nil, 3, nil)

		// Create a simple response
		response := &Response{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"test": "data"}`),
		}

		result, err := builder.WithResponse(response).WithExecutor(executor).Build()
		if err != nil {
			t.Fatalf("Result building failed: %v", err)
		}

		if result.StatusCode != 200 {
			t.Errorf("Expected status code 200, got %d", result.StatusCode)
		}

		if result.Body != `{"test": "data"}` {
			t.Errorf("Expected body '{\"test\": \"data\"}', got '%s'", result.Body)
		}
	})

	t.Run("HelperValidations", func(t *testing.T) {
		helper := NewHTTPRequestHelper()
		if helper == nil {
			t.Fatal("Helper creation failed")
		}

		// Test method validation
		validMethods := []HTTPMethod{
			HTTPMethodGET, HTTPMethodPOST, HTTPMethodPUT,
			HTTPMethodPATCH, HTTPMethodDELETE, HTTPMethodHEAD, HTTPMethodOPTIONS,
		}

		for _, method := range validMethods {
			err := helper.ValidateMethod(method)
			if err != nil {
				t.Errorf("Method %s validation failed: %v", method, err)
			}
		}

		// Test invalid method
		err := helper.ValidateMethod("INVALID")
		if err == nil {
			t.Error("Expected invalid method validation to fail")
		}

		// Test URL validation
		validURLs := []string{
			"http://example.com",
			"https://example.com",
			"https://api.example.com/v1/test",
		}

		for _, url := range validURLs {
			err := helper.ValidateURL(url)
			if err != nil {
				t.Errorf("URL %s validation failed: %v", url, err)
			}
		}

		// Test invalid URLs
		invalidURLs := []string{
			"",
			"ftp://example.com",
			"invalid-url",
			"example.com",
		}

		for _, url := range invalidURLs {
			err := helper.ValidateURL(url)
			if err == nil {
				t.Errorf("Expected URL %s validation to fail", url)
			}
		}

		// Test header parsing
		testHeaders := "Content-Type: application/json\nAuthorization: Bearer token\nUser-Agent: Test Agent"
		headers, err := helper.ParseHeaders(testHeaders)
		if err != nil {
			t.Fatalf("Header parsing failed: %v", err)
		}

		expectedHeaders := map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer token",
			"User-Agent":    "Test Agent",
		}

		for key, expectedValue := range expectedHeaders {
			if value, exists := headers[key]; !exists {
				t.Errorf("Expected header %s not found", key)
			} else if value != expectedValue {
				t.Errorf("Expected header %s value '%s', got '%s'", key, expectedValue, value)
			}
		}

		// Test parameter parsing
		testParams := "key1: value1\nkey2: value2\nkey3: value with spaces"
		params, err := helper.ParseParams(testParams)
		if err != nil {
			t.Fatalf("Parameter parsing failed: %v", err)
		}

		expectedParams := map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value with spaces",
		}

		for key, expectedValue := range expectedParams {
			if value, exists := params[key]; !exists {
				t.Errorf("Expected parameter %s not found", key)
			} else if value != expectedValue {
				t.Errorf("Expected parameter %s value '%s', got '%s'", key, expectedValue, value)
			}
		}
	})
}
