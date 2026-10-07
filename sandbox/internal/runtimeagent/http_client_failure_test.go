package runtimeagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
