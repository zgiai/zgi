package executionruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// A relay is owned by exactly one synchronous shell invocation. It uses the
// existing authenticated Worker connection, never an API URL inside the Job.
type agentPlatformRelay struct {
	agentExecutionGateway
	platform  agentExecutionPlatformGateway
	scope     runtimeplatform.Scope
	services  skillloop.RuntimePlatformServices
	files     *agentPlatformFiles
	authorize func(context.Context) error
	session   *runtimeplatform.Session
	token     string
	job       agentExecutionJob
	pending   skillloop.RuntimePlatformToolResult
}

func (g *agentPlatformRelay) SubmitJob(ctx context.Context, scope agentExecutionScope, bindingID, key, command string, args []string, timeout time.Duration) (agentExecutionJob, error) {
	// Package extraction/setup retains the legacy path and obtains no grant.
	if !strings.HasPrefix(key, "tool:") {
		return g.agentExecutionGateway.SubmitJob(ctx, scope, bindingID, key, command, args, timeout)
	}
	capabilities, err := g.Capabilities(ctx, scope, bindingID)
	if err != nil {
		return agentExecutionJob{}, err
	}
	if capabilities.PlatformProtocolVersion != 1 {
		return g.agentExecutionGateway.SubmitJob(ctx, scope, bindingID, key, command, args, timeout)
	}
	if g.platform == nil || g.authorize == nil || g.session != nil {
		return agentExecutionJob{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.relay_unavailable", nil)
	}
	if err := g.authorize(ctx); err != nil {
		return agentExecutionJob{}, err
	}
	binding, err := g.GetBinding(ctx, scope, bindingID)
	if err != nil || binding.Status != "active" || binding.Generation <= 0 {
		return agentExecutionJob{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.binding", err)
	}
	job, err := g.platform.SubmitPlatformJob(ctx, scope, bindingID, key, command, args, timeout)
	if err != nil {
		return agentExecutionJob{}, err
	}
	g.job = job
	if job.BindingID != binding.ID || job.BindingGeneration <= 0 {
		_ = cancelInterruptedAgentExecutionJob(ctx, g.agentExecutionGateway, scope, job)
		return agentExecutionJob{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.job_scope", nil)
	}
	g.scope.BindingID, g.scope.BindingGeneration, g.scope.JobID = binding.ID, job.BindingGeneration, job.ID
	actions := []runtimeplatform.Action{runtimeplatform.ReadFile, runtimeplatform.ExportFile}
	if g.services.Model != nil {
		actions = append(actions, runtimeplatform.CallModel)
	}
	if g.services.Tool != nil {
		actions = append(actions, runtimeplatform.CallTool)
	}
	g.session, g.token, err = runtimeplatform.NewSession(ctx, g.scope, actions, runtimeplatform.Options{TTL: timeout, Authorize: g.authorizeRequest, Handle: g.handle})
	if err != nil {
		_ = cancelInterruptedAgentExecutionJob(ctx, g.agentExecutionGateway, scope, job)
		return agentExecutionJob{}, err
	}
	return job, nil
}

func (g *agentPlatformRelay) authorizeRequest(ctx context.Context, scope runtimeplatform.Scope, _ runtimeplatform.Action) error {
	if err := g.authorize(ctx); err != nil {
		return err
	}
	executionScope := agentExecutionScope{OrganizationID: scope.OrganizationID, SubjectID: scope.SubjectID}
	if scope.ExecutionIdentity.Version != 0 {
		executionScope.Identity = &scope.ExecutionIdentity
	}
	binding, err := g.agentExecutionGateway.GetBinding(ctx, executionScope, scope.BindingID)
	if err != nil || binding.Generation != scope.BindingGeneration || binding.Status != "active" {
		return platformFileError(catalog.CodePermissionDenied, "runtime_platform.binding_changed", err)
	}
	job, err := g.agentExecutionGateway.GetJob(ctx, executionScope, scope.JobID)
	if err != nil || job.ID != g.job.ID || job.Generation != g.job.Generation || job.BindingID != scope.BindingID || job.Status != "running" || job.CancelRequested {
		return platformFileError(catalog.CodePermissionDenied, "runtime_platform.job_changed", err)
	}
	capabilities, err := g.agentExecutionGateway.Capabilities(ctx, executionScope, scope.BindingID)
	if err != nil || capabilities.PlatformProtocolVersion != 1 {
		return platformFileError(catalog.CodePermissionDenied, "runtime_platform.operator_revoked", err)
	}
	return nil
}

func (g *agentPlatformRelay) handle(ctx context.Context, scope runtimeplatform.Scope, request runtimeplatform.Request) (runtimeplatform.Response, error) {
	switch request.Action {
	case runtimeplatform.ReadFile, runtimeplatform.ExportFile:
		return g.files.Handle(ctx, scope, request)
	case runtimeplatform.CallModel:
		var args struct {
			Prompt    string `json:"prompt"`
			MaxTokens int    `json:"max_tokens"`
		}
		if err := decodePlatformArguments(request.Arguments, &args); err != nil {
			return runtimeplatform.Response{}, err
		}
		if g.services.Model == nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.model_disabled", nil)
		}
		text, err := g.services.Model(ctx, args.Prompt, args.MaxTokens)
		if err != nil {
			return runtimeplatform.Response{}, err
		}
		data, _ := json.Marshal(map[string]string{"text": text})
		return runtimeplatform.Response{Status: runtimeplatform.Completed, Data: data}, nil
	case runtimeplatform.CallTool:
		var args struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := decodePlatformArguments(request.Arguments, &args); err != nil {
			return runtimeplatform.Response{}, err
		}
		if g.services.Tool == nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.tool_disabled", nil)
		}
		result, err := g.services.Tool(ctx, "platform:"+scope.JobID+":"+request.ID, args.Name, args.Arguments)
		if err != nil {
			return runtimeplatform.Response{}, err
		}
		if result.NeedsContinuation() {
			g.pending = result
			return runtimeplatform.Response{Status: runtimeplatform.AwaitingApproval, Data: json.RawMessage(`{"continuation":"console"}`)}, nil
		}
		return runtimeplatform.Response{Status: runtimeplatform.Completed, Data: result.Data}, nil
	default:
		return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.action", nil)
	}
}

func (g *agentPlatformRelay) GetJob(ctx context.Context, scope agentExecutionScope, jobID string) (agentExecutionJob, error) {
	job, err := g.agentExecutionGateway.GetJob(ctx, scope, jobID)
	if jobID == g.job.ID && err == nil {
		g.job = job
	}
	if err != nil || g.session == nil || jobID != g.job.ID || job.Status != "running" || job.CancelRequested {
		return job, err
	}
	if g.pending.NeedsContinuation() {
		return job, nil
	}
	requests, err := g.platform.PlatformRequests(ctx, scope, jobID)
	if err != nil {
		// The process may have completed between status and queue reads.
		latest, statusErr := g.agentExecutionGateway.GetJob(ctx, scope, jobID)
		if statusErr == nil {
			if latest.Status != "running" && latest.Status != "queued" {
				g.job = latest
				return latest, nil
			}
			// Claiming a SQL Job precedes Runtime Agent process startup, and
			// grant cleanup precedes publishing its terminal SQL status. In
			// either gap the queue is absent, not a failed user command. Wait
			// for the owning Job's bounded lifecycle; never fabricate a grant,
			// dispatch an action or replay a claimed request. Other transport,
			// scope and generation errors still fail closed immediately.
			var gatewayErr agentExecutionGatewayHTTPError
			if errors.As(err, &gatewayErr) && gatewayErr.statusCode == 404 {
				g.job = latest
				return latest, nil
			}
		}
		logPlatformRelayFailure(ctx, "poll", err)
		return job, err
	}
	for _, request := range requests {
		result, callErr := g.session.Execute(ctx, g.token, g.scope, request)
		response := agentExecutionPlatformResponse{ID: request.ID, Status: result.Status, Data: result.Data}
		if callErr != nil {
			logPlatformRelayFailure(ctx, "dispatch", callErr)
			g.pending = skillloop.RuntimePlatformToolResult{}
			response.Status, response.Data = runtimeplatform.Failed, nil
			response.ErrorCode = catalog.CodeInternal.String()
			if code, ok := apperror.CodeOf(callErr); ok {
				response.ErrorCode = code.String()
			}
		}
		if err := g.platform.ResolvePlatformRequest(ctx, scope, jobID, response); err != nil {
			logPlatformRelayFailure(ctx, "resolve", err)
			return job, err
		}
		if apperror.IsCode(callErr, catalog.CodePermissionDenied) {
			if err := g.agentExecutionGateway.CancelJob(ctx, scope, job.ID, job.Generation); err != nil {
				return job, err
			}
			break
		}
		if g.pending.NeedsContinuation() {
			// Approval terminates this Job. Its grant cannot survive or execute
			// later queued requests; the existing console continuation owns resume.
			if err := g.agentExecutionGateway.CancelJob(ctx, scope, job.ID, job.Generation); err != nil {
				return job, err
			}
			break
		}
	}
	return job, nil
}

// Log only stable classifications. Request arguments, grants, upstream bodies
// and transport URLs must never enter these control-plane diagnostics.
func logPlatformRelayFailure(ctx context.Context, phase string, err error) {
	fields := []any{"phase", phase}
	if app, ok := apperror.As(err); ok {
		fields = append(fields, "error_code", app.Code().String(), "operation", app.Operation())
	}
	var gatewayErr agentExecutionGatewayHTTPError
	if errors.As(err, &gatewayErr) {
		fields = append(fields, "worker_status", gatewayErr.statusCode, "worker_code", gatewayErr.code)
	}
	logger.WarnContext(ctx, "runtime platform relay failed", fields...)
}

func decodePlatformArguments(data json.RawMessage, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return platformFileError(catalog.CodeRequestInvalid, "runtime_platform.arguments", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return platformFileError(catalog.CodeRequestInvalid, "runtime_platform.arguments", nil)
	}
	return nil
}

func (g *agentPlatformRelay) Close(ctx context.Context) {
	if g.session != nil {
		g.session.Close()
		g.token = ""
	}
	if g.job.ID != "" && (g.job.Status == "running" || g.job.Status == "queued") {
		scope := agentExecutionScope{OrganizationID: g.scope.OrganizationID, SubjectID: g.scope.SubjectID}
		if g.scope.ExecutionIdentity.Version != 0 {
			scope.Identity = &g.scope.ExecutionIdentity
		}
		_ = cancelInterruptedAgentExecutionJob(ctx, g.agentExecutionGateway, scope, g.job)
	}
}
