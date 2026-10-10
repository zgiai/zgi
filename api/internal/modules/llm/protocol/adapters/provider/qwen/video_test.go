package qwen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestQwenCreateVideoUsesNativeDashScopeContract(t *testing.T) {
	var gotPath, gotAsync string
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAsync = r.Header.Get("X-DashScope-Async")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"request_id":"trace-only","output":{"task_id":"video-task-1","task_status":"PENDING"}}`)
	}))
	defer server.Close()

	a, err := newTestAdapter(&adapter.AdapterConfig{APIKey: "dash-key", BaseURL: server.URL + "/compatible-mode/v1"})
	if err != nil {
		t.Fatalf("newTestAdapter: %v", err)
	}
	duration, count := 5, 2
	resp, err := a.CreateVideo(context.Background(), &adapter.VideoRequest{
		Model: "wan2.1-t2v-plus", Prompt: "sunrise", Size: "1280x720", Resolution: "720p",
		Duration: &duration, N: &count,
	})
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}
	if gotPath != "/api/v1/services/aigc/video-generation/video-synthesis" || gotAsync != "enable" {
		t.Fatalf("path/header = %q/%q", gotPath, gotAsync)
	}
	if got["model"] != "wan2.1-t2v-plus" {
		t.Fatalf("model = %#v", got["model"])
	}
	params := got["parameters"].(map[string]any)
	if params["size"] != "1280*720" || params["resolution"] != "720p" || params["duration"] != float64(5) || params["n"] != float64(2) {
		t.Fatalf("parameters = %#v", params)
	}
	if resp.TaskID != "video-task-1" || resp.Status != "PENDING" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestQwenVideoTaskIDPrefersOutputOverRequestID(t *testing.T) {
	resp, err := decodeQwenVideoResponse([]byte(`{"request_id":"trace-only","output":{"task_id":"actual-task","task_status":"RUNNING"}}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TaskID != "actual-task" {
		t.Fatalf("task id = %q, want actual-task", resp.TaskID)
	}
	if resp.TaskID == "trace-only" {
		t.Fatal("request_id must never be used as DashScope task id")
	}
}

func TestQwenGetVideoTaskEscapesTaskID(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		fmt.Fprint(w, `{"output":{"task_id":"task/video 1","task_status":"SUCCEEDED","video_url":"https://cdn.example/video.mp4"}}`)
	}))
	defer server.Close()
	a, err := newTestAdapter(&adapter.AdapterConfig{APIKey: "dash-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("newTestAdapter: %v", err)
	}
	resp, err := a.GetVideoTask(context.Background(), &adapter.VideoTaskRequest{TaskID: "task/video 1"})
	if err != nil {
		t.Fatalf("GetVideoTask: %v", err)
	}
	if gotPath != "/api/v1/tasks/task%2Fvideo%201" {
		t.Fatalf("escaped path = %q", gotPath)
	}
	if resp.VideoURL == "" || resp.Status != "SUCCEEDED" {
		t.Fatalf("response = %#v", resp)
	}
}
