package httprequest

import (
	"context"
	"fmt"
	"log"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
)

// HTTPRequestExample HTTP request usage examples
type HTTPRequestExample struct{}

// NewHTTPRequestExample creates example instance
func NewHTTPRequestExample() *HTTPRequestExample {
	return &HTTPRequestExample{}
}

// SimpleGETRequest simple GET request example
func (e *HTTPRequestExample) SimpleGETRequest() {
	fmt.Println("=== Simple GET Request Example ===")

	// Create node data
	nodeData := &NodeData{
		Method: HTTPMethodGET,
		URL:    "https://httpbin.org/get",
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeNoAuth,
		},
		Headers: "User-Agent: HTTPRequest-Go-Client\nAccept: application/json",
		Params:  "test: value\nfoo: bar",
	}

	// Validate node data
	if err := nodeData.Validate(); err != nil {
		log.Printf("Node data validation failed: %v", err)
		return
	}

	// Create executor
	variablePool := entities.NewVariablePool()
	executor := NewHTTPRequestExecutor(nodeData, variablePool, nil, 3, nil)

	// Execute request
	ctx := context.Background()
	response, err := executor.Execute(ctx)
	if err != nil {
		log.Printf("Request execution failed: %v", err)
		return
	}

	// Print results
	fmt.Printf("Status code: %d\n", response.StatusCode)
	fmt.Printf("Response size: %s\n", response.ReadableSize())
	fmt.Printf("Content type: %s\n", response.GetContentType())
	fmt.Printf("Is file: %v\n", response.IsFile())
	fmt.Printf("Response content: %s\n", response.Text())
}

// POSTJSONRequest POST JSON request example
func (e *HTTPRequestExample) POSTJSONRequest() {
	fmt.Println("\n=== POST JSON Request Example ===")

	// Create JSON request body
	jsonBody := `{
		"name": "John Doe",
		"email": "john@example.com",
		"age": 30
	}`

	nodeData := &NodeData{
		Method: HTTPMethodPOST,
		URL:    "https://httpbin.org/post",
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeNoAuth,
		},
		Headers: "Content-Type: application/json\nUser-Agent: HTTPRequest-Go-Client",
		Body: &HttpRequestNodeBody{
			Type: BodyTypeJSON,
			Data: []BodyData{
				{
					Key:   "",
					Type:  BodyDataTypeText,
					Value: jsonBody,
				},
			},
		},
	}

	if err := nodeData.Validate(); err != nil {
		log.Printf("Node data validation failed: %v", err)
		return
	}

	variablePool := entities.NewVariablePool()
	executor := NewHTTPRequestExecutor(nodeData, variablePool, nil, 3, nil)

	ctx := context.Background()
	response, err := executor.Execute(ctx)
	if err != nil {
		log.Printf("Request execution failed: %v", err)
		return
	}

	fmt.Printf("Status code: %d\n", response.StatusCode)
	fmt.Printf("Response size: %s\n", response.ReadableSize())
	fmt.Printf("Response content: %s\n", response.Text())
}

// FormDataRequest form data request example
func (e *HTTPRequestExample) FormDataRequest() {
	fmt.Println("\n=== Form Data Request Example ===")

	nodeData := &NodeData{
		Method: HTTPMethodPOST,
		URL:    "https://httpbin.org/post",
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeNoAuth,
		},
		Body: &HttpRequestNodeBody{
			Type: BodyTypeFormData,
			Data: []BodyData{
				{
					Key:   "username",
					Type:  BodyDataTypeText,
					Value: "testuser",
				},
				{
					Key:   "password",
					Type:  BodyDataTypeText,
					Value: "testpass",
				},
				{
					Key:   "remember",
					Type:  BodyDataTypeText,
					Value: "on",
				},
			},
		},
	}

	if err := nodeData.Validate(); err != nil {
		log.Printf("Node data validation failed: %v", err)
		return
	}

	variablePool := entities.NewVariablePool()
	executor := NewHTTPRequestExecutor(nodeData, variablePool, nil, 3, nil)

	ctx := context.Background()
	response, err := executor.Execute(ctx)
	if err != nil {
		log.Printf("Request execution failed: %v", err)
		return
	}

	fmt.Printf("Status code: %d\n", response.StatusCode)
	fmt.Printf("Response size: %s\n", response.ReadableSize())
	fmt.Printf("Response content: %s\n", response.Text())
}

// AuthorizedRequest authorized request example
func (e *HTTPRequestExample) AuthorizedRequest() {
	fmt.Println("\n=== Authorized Request Example ===")

	nodeData := &NodeData{
		Method: HTTPMethodGET,
		URL:    "https://httpbin.org/bearer",
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeAPIKey,
			Config: &HttpRequestNodeAuthorizationConfig{
				Type:   AuthorizationConfigTypeBearer,
				APIKey: "test-token-12345",
				Header: "Authorization",
			},
		},
	}

	if err := nodeData.Validate(); err != nil {
		log.Printf("Node data validation failed: %v", err)
		return
	}

	variablePool := entities.NewVariablePool()
	executor := NewHTTPRequestExecutor(nodeData, variablePool, nil, 3, nil)

	ctx := context.Background()
	response, err := executor.Execute(ctx)
	if err != nil {
		log.Printf("Request execution failed: %v", err)
		return
	}

	fmt.Printf("Status code: %d\n", response.StatusCode)
	fmt.Printf("Response size: %s\n", response.ReadableSize())
	fmt.Printf("Response content: %s\n", response.Text())
}

// BasicAuthRequest Basic authentication request example
func (e *HTTPRequestExample) BasicAuthRequest() {
	fmt.Println("\n=== Basic Authentication Request Example ===")

	nodeData := &NodeData{
		Method: HTTPMethodGET,
		URL:    "https://httpbin.org/basic-auth/user/pass",
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeAPIKey,
			Config: &HttpRequestNodeAuthorizationConfig{
				Type:   AuthorizationConfigTypeBasic,
				APIKey: "user:pass", // username:password format
				Header: "Authorization",
			},
		},
	}

	if err := nodeData.Validate(); err != nil {
		log.Printf("Node data validation failed: %v", err)
		return
	}

	variablePool := entities.NewVariablePool()
	executor := NewHTTPRequestExecutor(nodeData, variablePool, nil, 3, nil)

	ctx := context.Background()
	response, err := executor.Execute(ctx)
	if err != nil {
		log.Printf("Request execution failed: %v", err)
		return
	}

	fmt.Printf("Status code: %d\n", response.StatusCode)
	fmt.Printf("Response size: %s\n", response.ReadableSize())
	fmt.Printf("Response content: %s\n", response.Text())
}

// CustomTimeoutRequest custom timeout request example
func (e *HTTPRequestExample) CustomTimeoutRequest() {
	fmt.Println("\n=== Custom Timeout Request Example ===")

	nodeData := &NodeData{
		Method: HTTPMethodGET,
		URL:    "https://httpbin.org/delay/2", // 2 seconds delay response
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeNoAuth,
		},
	}

	// Custom timeout configuration
	timeout := &HttpRequestNodeTimeout{
		Connect: 5,  // 5 seconds connection timeout
		Read:    10, // 10 seconds read timeout
		Write:   10, // 10 seconds write timeout
	}

	nodeData.Timeout = timeout

	if err := nodeData.Validate(); err != nil {
		log.Printf("Node data validation failed: %v", err)
		return
	}

	variablePool := entities.NewVariablePool()
	executor := NewHTTPRequestExecutor(nodeData, variablePool, timeout, 3, nil)

	ctx := context.Background()
	response, err := executor.Execute(ctx)
	if err != nil {
		log.Printf("Request execution failed: %v", err)
		return
	}

	fmt.Printf("Status code: %d\n", response.StatusCode)
	fmt.Printf("Response size: %s\n", response.ReadableSize())
	fmt.Printf("Response content: %s\n", response.Text())
}

// ErrorHandlingExample error handling example
func (e *HTTPRequestExample) ErrorHandlingExample() {
	fmt.Println("\n=== Error Handling Example ===")

	// Invalid URL example
	nodeData := &NodeData{
		Method: HTTPMethodGET,
		URL:    "invalid-url", // Invalid URL
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeNoAuth,
		},
	}

	if err := nodeData.Validate(); err != nil {
		fmt.Printf("Caught URL validation error: %v\n", err)
	}

	// Invalid authentication configuration example
	nodeData2 := &NodeData{
		Method: HTTPMethodGET,
		URL:    "https://httpbin.org/get",
		Authorization: HttpRequestNodeAuthorization{
			Type:   AuthorizationTypeAPIKey,
			Config: nil, // Missing configuration
		},
	}

	if err := nodeData2.Validate(); err != nil {
		fmt.Printf("Caught authentication validation error: %v\n", err)
	}

	// 404 error example
	nodeData3 := &NodeData{
		Method: HTTPMethodGET,
		URL:    "https://httpbin.org/status/404",
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeNoAuth,
		},
	}

	if err := nodeData3.Validate(); err != nil {
		log.Printf("Node data validation failed: %v", err)
		return
	}

	variablePool := entities.NewVariablePool()
	executor := NewHTTPRequestExecutor(nodeData3, variablePool, nil, 3, nil)

	ctx := context.Background()
	response, err := executor.Execute(ctx)
	if err != nil {
		fmt.Printf("Caught request error: %v\n", err)
		return
	}

	fmt.Printf("404 status code: %d\n", response.StatusCode)
	fmt.Printf("Response content: %s\n", response.Text())
}

// RunAllExamples runs all examples
func (e *HTTPRequestExample) RunAllExamples() {
	fmt.Println("HTTP Request Node Usage Examples")
	fmt.Println("====================")

	e.SimpleGETRequest()
	e.POSTJSONRequest()
	e.FormDataRequest()
	e.AuthorizedRequest()
	e.BasicAuthRequest()
	e.CustomTimeoutRequest()
	e.ErrorHandlingExample()

	fmt.Println("\nAll examples completed!")
}

// FactoryExample factory pattern usage example
func (e *HTTPRequestExample) FactoryExample() {
	fmt.Println("\n=== Factory Pattern Usage Example ===")

	// Use factory to create default configuration
	factory := NewHTTPRequestFactory()
	defaultConfig := factory.GetDefaultConfig()

	fmt.Printf("Default configuration: %+v\n", defaultConfig)

	// Use helper tools
	helper := NewHTTPRequestHelper()

	// Validate HTTP method
	if err := helper.ValidateMethod(HTTPMethodGET); err != nil {
		fmt.Printf("Method validation failed: %v\n", err)
	} else {
		fmt.Println("GET method validation passed")
	}

	// Validate URL
	if err := helper.ValidateURL("https://example.com"); err != nil {
		fmt.Printf("URL validation failed: %v\n", err)
	} else {
		fmt.Println("URL validation passed")
	}

	// Parse headers
	headers, err := helper.ParseHeaders("Content-Type: application/json\nAuthorization: Bearer token")
	if err != nil {
		fmt.Printf("Header parsing failed: %v\n", err)
	} else {
		fmt.Printf("Parsed headers: %+v\n", headers)
	}

	// Parse parameters
	params, err := helper.ParseParams("key1: value1\nkey2: value2")
	if err != nil {
		fmt.Printf("Parameter parsing failed: %v\n", err)
	} else {
		fmt.Printf("Parsed parameters: %+v\n", params)
	}
}
