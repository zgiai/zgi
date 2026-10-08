package platformrelay

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// ServeHTTP must only be mounted on a dedicated in-sandbox Unix socket. It
// never shares the privileged Runtime Agent listener or its bearer token.
func (b *Broker) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobID := request.Header.Get("X-ZGI-Job-ID")
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	var response agentexecution.PlatformResponse
	var err error
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/requests":
		var body agentexecution.PlatformRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, MaxRequestBytes+1024))
		decoder.DisallowUnknownFields()
		if decodeErr := decoder.Decode(&body); decodeErr != nil {
			err = agentexecution.ErrInvalidRequest
			break
		}
		if decodeErr := decoder.Decode(&struct{}{}); !errors.Is(decodeErr, io.EOF) {
			err = agentexecution.ErrInvalidRequest
			break
		}
		response, err = b.Submit(jobID, token, body)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/requests/"):
		id := strings.TrimPrefix(request.URL.Path, "/v1/requests/")
		if !validID(id) {
			err = agentexecution.ErrInvalidRequest
			break
		}
		response, err = b.Receipt(jobID, token, id)
	default:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error_code": "not_found"})
		return
	}
	if err != nil {
		status, code := http.StatusBadRequest, "invalid_request"
		switch {
		case errors.Is(err, agentexecution.ErrNotFound):
			status, code = http.StatusUnauthorized, "unauthorized"
		case errors.Is(err, agentexecution.ErrIdempotencyConflict):
			status, code = http.StatusConflict, "request_conflict"
		case errors.Is(err, agentexecution.ErrOutputLimit):
			status, code = http.StatusRequestEntityTooLarge, "limit_exceeded"
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error_code": code})
		return
	}
	_ = json.NewEncoder(w).Encode(response)
}
