package runtimeagent

import (
	"context"
	"net/http"
	"net/url"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func (b *Backend) PlatformHandler() http.Handler {
	if b.platform == nil {
		return http.NotFoundHandler()
	}
	return b.platform
}

func (b *Backend) PlatformRequests(ctx context.Context, _ agentexecution.Allocation, jobID string) ([]agentexecution.PlatformRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.platform == nil {
		return nil, agentexecution.ErrBackendUnavailable
	}
	return b.platform.Pending(jobID)
}

func (b *Backend) ResolvePlatformRequest(ctx context.Context, _ agentexecution.Allocation, jobID string, response agentexecution.PlatformResponse) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.platform == nil {
		return agentexecution.ErrBackendUnavailable
	}
	return b.platform.Resolve(jobID, response)
}

func (b *RemoteBackend) PlatformRequests(ctx context.Context, _ agentexecution.Allocation, jobID string) ([]agentexecution.PlatformRequest, error) {
	var requests []agentexecution.PlatformRequest
	err := b.doJSON(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(jobID)+"/platform-requests", nil, &requests)
	return requests, err
}

func (b *RemoteBackend) ResolvePlatformRequest(ctx context.Context, _ agentexecution.Allocation, jobID string, response agentexecution.PlatformResponse) error {
	return b.doJSON(ctx, http.MethodPost, "/v1/jobs/"+url.PathEscape(jobID)+"/platform-requests", response, nil)
}

func (s *HTTPServer) platformRequests(w http.ResponseWriter, request *http.Request, jobID string) {
	backend, ok := s.backend.(agentexecution.PlatformBackend)
	if !ok {
		s.writeResult(w, http.StatusOK, nil, agentexecution.ErrBackendUnavailable)
		return
	}
	switch request.Method {
	case http.MethodGet:
		requests, err := backend.PlatformRequests(request.Context(), agentexecution.Allocation{}, jobID)
		s.writeResult(w, http.StatusOK, requests, err)
	case http.MethodPost:
		var response agentexecution.PlatformResponse
		if !s.decodeJSON(w, request, &response) {
			return
		}
		err := backend.ResolvePlatformRequest(request.Context(), agentexecution.Allocation{}, jobID, response)
		s.writeResult(w, http.StatusOK, nil, err)
	default:
		writeRuntimeAgentMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}
