package executionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
)

type agentExecutionPlatformResponse struct {
	ID        string                 `json:"id"`
	Status    runtimeplatform.Status `json:"status"`
	Data      json.RawMessage        `json:"data,omitempty"`
	ErrorCode string                 `json:"error_code,omitempty"`
}

// Optional to the legacy execution interface. Admission metadata contains no
// grant; the Runtime Agent creates the short-lived socket credential in memory.
type agentExecutionPlatformGateway interface {
	agentExecutionPlatformFileGateway
	SubmitPlatformJob(context.Context, agentExecutionScope, string, string, string, []string, time.Duration) (agentExecutionJob, error)
	PlatformRequests(context.Context, agentExecutionScope, string) ([]runtimeplatform.Request, error)
	ResolvePlatformRequest(context.Context, agentExecutionScope, string, agentExecutionPlatformResponse) error
}

type agentExecutionPlatformFileGateway interface {
	UploadPlatformFile(context.Context, agentExecutionScope, string, string, []byte) error
	DownloadPlatformFile(context.Context, agentExecutionScope, string, string) ([]byte, error)
}

func (g *agentExecutionHTTPGateway) UploadPlatformFile(ctx context.Context, scope agentExecutionScope, jobID, path string, data []byte) error {
	return g.uploadRuntimeFile(ctx, scope, "jobs/"+url.PathEscape(jobID)+"/platform-files?path="+url.QueryEscape(path), data)
}

func (g *agentExecutionHTTPGateway) DownloadPlatformFile(ctx context.Context, scope agentExecutionScope, jobID, path string) ([]byte, error) {
	return g.downloadRuntimeFile(ctx, scope, "jobs/"+url.PathEscape(jobID)+"/platform-files?path="+url.QueryEscape(path))
}

func (g *agentExecutionHTTPGateway) SubmitPlatformJob(ctx context.Context, scope agentExecutionScope, bindingID, idempotencyKey, command string, args []string, timeout time.Duration) (agentExecutionJob, error) {
	var result agentExecutionJob
	err := g.request(ctx, scope, http.MethodPost, "bindings/"+url.PathEscape(bindingID)+"/jobs", map[string]any{
		"idempotency_key": idempotencyKey, "command": command, "args": args, "timeout_ms": timeout.Milliseconds(),
		"environment": map[string]string{"ZGI_JOB_PLATFORM_ENABLED": "true"},
	}, &result)
	return result, err
}

func (g *agentExecutionHTTPGateway) PlatformRequests(ctx context.Context, scope agentExecutionScope, jobID string) ([]runtimeplatform.Request, error) {
	var requests []runtimeplatform.Request
	err := g.request(ctx, scope, http.MethodGet, "jobs/"+url.PathEscape(jobID)+"/platform-requests", nil, &requests)
	return requests, err
}

func (g *agentExecutionHTTPGateway) ResolvePlatformRequest(ctx context.Context, scope agentExecutionScope, jobID string, response agentExecutionPlatformResponse) error {
	return g.request(ctx, scope, http.MethodPost, "jobs/"+url.PathEscape(jobID)+"/platform-requests", response, nil)
}
