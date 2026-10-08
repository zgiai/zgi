package mcpclient

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPythonSDKInterop(t *testing.T) {
	python := os.Getenv("MCP_PYTHON_EXECUTABLE")
	if python == "" {
		t.Skip("set MCP_PYTHON_EXECUTABLE to a Python environment with mcp==2.3.0")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "..", "testdata", "mcp_python_server.py"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(ServerConfig{ID: "python", Transport: TransportStdio, Process: &ProcessSpec{Command: python, Args: []string{fixture}}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if !conn.Info().Capabilities.Tools || !conn.Info().Capabilities.Resources || !conn.Info().Capabilities.Prompts {
		t.Fatalf("Python capabilities: %#v", conn.Info())
	}
	tools, err := conn.ListToolsPage(ctx, nil)
	if err != nil || len(tools.Items) != 1 || tools.Items[0].Name != "Case.Tool" {
		t.Fatalf("Python tools: %#v %v", tools, err)
	}
	result, err := conn.CallTool(ctx, ToolCallRequest{Name: "Case.Tool", Arguments: json.RawMessage(`{"value":"hello"}`)})
	if err != nil || !result.StructuredContent.Present {
		t.Fatalf("Python tool result: %#v %v", result, err)
	}
	resources, err := conn.ListResourcesPage(ctx, nil)
	if err != nil || len(resources.Items) == 0 {
		t.Fatalf("Python resources: %#v %v", resources, err)
	}
	read, err := conn.ReadResource(ctx, ResourceReadRequest{URI: "demo://readme"})
	if err != nil || len(read.Contents) == 0 {
		t.Fatalf("Python resource: %#v %v", read, err)
	}
	prompts, err := conn.ListPromptsPage(ctx, nil)
	if err != nil || len(prompts.Items) == 0 {
		t.Fatalf("Python prompts: %#v %v", prompts, err)
	}
	got, err := conn.GetPrompt(ctx, PromptGetRequest{Name: "summarize", Arguments: map[string]string{"topic": "MCP"}})
	if err != nil || len(got.Messages) == 0 {
		t.Fatalf("Python prompt: %#v %v", got, err)
	}
}

func TestPythonSDKHTTPInterop(t *testing.T) {
	python := os.Getenv("MCP_PYTHON_EXECUTABLE")
	if python == "" {
		t.Skip("set MCP_PYTHON_EXECUTABLE to a Python environment with mcp==2.3.0")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "..", "testdata", "mcp_python_server.py"))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	cmd := exec.Command(python, fixture, "--http", "--port", strconv.Itoa(port))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	client, err := NewClient(ServerConfig{ID: "python-http", Transport: TransportStreamableHTTP, Endpoint: "http://127.0.0.1:" + strconv.Itoa(port) + "/mcp", Limits: Limits{ConnectTimeout: time.Second}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var conn *Connection
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		attemptCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		conn, err = client.Connect(attemptCtx)
		cancel()
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect to Python HTTP SDK: %v", err)
	}
	defer conn.Close(t.Context())
	tools, err := conn.ListToolsPage(t.Context(), nil)
	if err != nil || len(tools.Items) != 1 || tools.Items[0].Name != "Case.Tool" {
		t.Fatalf("Python HTTP tools: %#v %v", tools, err)
	}
	result, err := conn.CallTool(t.Context(), ToolCallRequest{Name: "Case.Tool", Arguments: json.RawMessage(`{"value":"http"}`)})
	if err != nil || !result.StructuredContent.Present {
		t.Fatalf("Python HTTP tool: %#v %v", result, err)
	}
	resources, err := conn.ListResourcesPage(t.Context(), nil)
	if err != nil || len(resources.Items) == 0 {
		t.Fatalf("Python HTTP resources: %#v %v", resources, err)
	}
	prompts, err := conn.ListPromptsPage(t.Context(), nil)
	if err != nil || len(prompts.Items) == 0 {
		t.Fatalf("Python HTTP prompts: %#v %v", prompts, err)
	}
}
