//go:build linux

package platformcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/platformrelay"
)

func unixClient(t *testing.T, handler http.Handler, token string) *Client {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "platform.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Error(err)
		}
	})
	client, err := NewClient(socket, "job-1", token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestCLIUnixRequestExportAndPendingApproval(t *testing.T) {
	for _, status := range []string{"completed", "awaiting_approval", "failed"} {
		t.Run(status, func(t *testing.T) {
			broker := platformrelay.New()
			t.Cleanup(broker.Close)
			token, err := broker.OpenJob(t.Context(), "job-1", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			client := unixClient(t, broker, token)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for {
					requests, err := broker.Pending("job-1")
					if err != nil {
						done <- err
						return
					}
					if len(requests) > 0 {
						if len(requests) != 1 || requests[0].Action != "file.export" {
							done <- errors.New("unexpected platform request")
							return
						}
						response := agentexecution.PlatformResponse{ID: requests[0].ID, Status: status}
						if status == "failed" {
							response.ErrorCode = "auth.forbidden"
						} else {
							response.Data = json.RawMessage(`{"receipt":"test-receipt"}`)
						}
						done <- broker.Resolve("job-1", response)
						return
					}
					select {
					case <-ctx.Done():
						done <- ctx.Err()
						return
					case <-ticker.C:
					}
				}
			}()
			var output, diagnostic bytes.Buffer
			exit := Run(ctx, client, []string{"file", "export", "--id", "export-1", "--path", "result.txt"}, &output, &diagnostic)
			wantExit := map[string]int{"completed": 0, "awaiting_approval": ExitAwaitingApproval, "failed": 1}[status]
			if exit != wantExit || !json.Valid(output.Bytes()) || diagnostic.Len() != 0 || strings.Contains(output.String(), token) {
				t.Errorf("exit=%d want=%d output=%s diagnostic=%s", exit, wantExit, output.String(), diagnostic.String())
			}
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-ctx.Done():
				t.Fatal("control response did not finish")
			}
		})
	}
}

func TestClientRejectsRedirectOversizeAndWrongCorrelation(t *testing.T) {
	for _, state := range []string{"redirect", "oversize", "wrong_id", "bad_status"} {
		t.Run(state, func(t *testing.T) {
			client := unixClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch state {
				case "redirect":
					http.Redirect(w, r, "https://example.invalid/private", http.StatusFound)
				case "oversize":
					_, _ = w.Write([]byte(strings.Repeat("x", platformrelay.MaxResponseBytes+2048)))
				case "wrong_id":
					_ = json.NewEncoder(w).Encode(agentexecution.PlatformResponse{ID: "other", Status: "completed"})
				case "bad_status":
					_ = json.NewEncoder(w).Encode(agentexecution.PlatformResponse{ID: "r1", Status: "unknown"})
				}
			}), strings.Repeat("x", 43))
			_, err := client.Call(t.Context(), agentexecution.PlatformRequest{ID: "r1", Action: "file.export", Arguments: json.RawMessage(`{"path":"result.txt"}`)})
			if err == nil || strings.Contains(err.Error(), "example.invalid") || strings.Contains(err.Error(), strings.Repeat("x", 43)) {
				t.Fatalf("state=%s error=%v", state, err)
			}
		})
	}
}

func TestCLIInvalidArgumentsDoNotEchoPromptOrCredential(t *testing.T) {
	var output, diagnostic bytes.Buffer
	exit := Run(t.Context(), nil, []string{"model", "call", "--prompt", "private prompt", "--invalid", "credential"}, &output, &diagnostic)
	if exit != 2 || output.Len() != 0 || strings.Contains(diagnostic.String(), "private prompt") || strings.Contains(diagnostic.String(), "credential") {
		t.Fatalf("exit=%d output=%s diagnostic=%s", exit, output.String(), diagnostic.String())
	}
	for _, args := range [][]string{
		{"file", "read", "--id", "read-1", "--ref", "input-1", "--path", "inputs/a.csv"},
		{"model", "call", "--id", "model-1", "--prompt", "hello", "--max-tokens", "64"},
		{"tool", "call", "--id", "tool-1", "--name", "bound-tool", "--arguments", `{"value":1}`},
		{"request", "status", "--id", "tool-1"},
	} {
		if _, _, err := parseArguments(args); err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
	}
}
