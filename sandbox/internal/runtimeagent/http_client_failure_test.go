package runtimeagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestRemoteWaitCancelsOnObservationFailure(t *testing.T) {
	for _, phase := range []string{"output", "status"} {
		for _, denyCancel := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/stopped", true: "/unsafe"}[denyCancel], func(t *testing.T) {
				var cancelCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/cancel") {
						cancelCalls.Add(1)
						if denyCancel {
							w.WriteHeader(http.StatusServiceUnavailable)
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"data": runtimeJobStatus{}})
						return
					}
					if cancelCalls.Load() > 0 && !denyCancel {
						_ = json.NewEncoder(w).Encode(map[string]any{"data": runtimeJobStatus{Done: true, Result: &agentexecution.JobResult{ExitCode: -1, Canceled: true}}})
						return
					}
					if strings.HasSuffix(r.URL.Path, "/output") && phase == "status" {
						_ = json.NewEncoder(w).Encode(map[string]any{"data": []agentexecution.OutputChunk{}})
						return
					}
					w.WriteHeader(http.StatusBadGateway)
					_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
				}))
				defer server.Close()
				backend, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "test-token", HTTPClient: server.Client()})
				if err != nil {
					t.Fatal(err)
				}
				job := &remoteRunningJob{backend: backend, jobID: "running", sink: func(agentexecution.OutputChunk) error { return nil }}
				result, err := job.Wait(t.Context())
				if err == nil || cancelCalls.Load() != 1 || result.TerminationFailed != denyCancel || result.ExitCode != -1 {
					t.Fatalf("wait=%+v err=%v cancellation=%d", result, err, cancelCalls.Load())
				}
			})
		}
	}
}

func TestRemoteObservationFailureRequiresConfirmedTermination(t *testing.T) {
	for _, state := range []string{"ack_only", "confirmation_lost", "unsafe_result", "missing_result", "confirmed"} {
		t.Run(state, func(t *testing.T) {
			var cancelCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/cancel") {
					cancelCalls.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"data": runtimeJobStatus{}})
					return
				}
				if cancelCalls.Load() == 0 || state == "confirmation_lost" {
					w.WriteHeader(http.StatusBadGateway)
					_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
					return
				}
				status := runtimeJobStatus{}
				if state != "ack_only" {
					status.Done = true
					if state != "missing_result" {
						status.Result = &agentexecution.JobResult{ExitCode: -1, TerminationFailed: state == "unsafe_result"}
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": status})
			}))
			defer server.Close()
			backend, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "test-token", HTTPClient: server.Client(), PollInterval: time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			backend.stopTimeout = 30 * time.Millisecond
			job := &remoteRunningJob{backend: backend, jobID: "running", sink: func(agentexecution.OutputChunk) error { return nil }}
			result, err := job.Wait(t.Context())
			if err == nil || cancelCalls.Load() != 1 || result.TerminationFailed != (state != "confirmed") {
				t.Fatalf("state=%s result=%+v err=%v cancellation=%d", state, result, err, cancelCalls.Load())
			}
		})
	}
}
