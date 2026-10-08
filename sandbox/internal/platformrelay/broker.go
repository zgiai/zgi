// Package platformrelay holds bounded Job-owned requests for the authenticated
// control plane. It never executes user-supplied routes or opens an API network
// connection. All operations remain subject to the platform's live authorizer.
package platformrelay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const (
	MaxRequests      = 32
	MaxRequestBytes  = 64 << 10
	MaxResponseBytes = 32 << 10
	MaxLifetime      = 5 * time.Minute
)

type receipt struct {
	request     agentexecution.PlatformRequest
	fingerprint [32]byte
	response    agentexecution.PlatformResponse
}

type jobGrant struct {
	ctx         context.Context
	cancel      context.CancelFunc
	stopCleanup func() bool
	digest      [32]byte
	requests    map[string]*receipt
	order       []string
}

// Broker owns only ephemeral grants and receipts. CloseJob must be called after
// the process tree stops. Restarting a Runtime Agent discards every grant.
type Broker struct {
	mu     sync.Mutex
	jobs   map[string]*jobGrant
	closed bool
}

func New() *Broker { return &Broker{jobs: make(map[string]*jobGrant)} }

// OpenJob returns the raw grant once. It must not be persisted in Job specs,
// database records, workspace files, checkpoints or control-plane output.
func (b *Broker) OpenJob(parent context.Context, jobID string, lifetime time.Duration) (string, error) {
	if parent == nil || parent.Err() != nil || !validID(jobID) || lifetime <= 0 || lifetime > MaxLifetime {
		return "", agentexecution.ErrInvalidRequest
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", agentexecution.ErrNotFound
	}
	if _, exists := b.jobs[jobID]; exists {
		return "", agentexecution.ErrIdempotencyConflict
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("create platform grant: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	clear(secret)
	ctx, cancel := context.WithTimeout(parent, lifetime)
	grant := &jobGrant{ctx: ctx, cancel: cancel, digest: sha256.Sum256([]byte(token)), requests: make(map[string]*receipt)}
	b.jobs[jobID] = grant
	grant.stopCleanup = context.AfterFunc(ctx, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.jobs[jobID] == grant {
			delete(b.jobs, jobID)
		}
	})
	return token, nil
}

// CloseJob invalidates the grant and removes its arguments and responses.
func (b *Broker) CloseJob(jobID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if grant := b.jobs[jobID]; grant != nil {
		grant.cancel()
		grant.stopCleanup()
		delete(b.jobs, jobID)
	}
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for id, grant := range b.jobs {
		grant.cancel()
		grant.stopCleanup()
		delete(b.jobs, id)
	}
}

// Submit is called only through the in-sandbox socket with the transient Job
// grant. Duplicate IDs return the receipt; they cannot enqueue another action.
func (b *Broker) Submit(jobID, token string, request agentexecution.PlatformRequest) (agentexecution.PlatformResponse, error) {
	if !validRequest(request) {
		return agentexecution.PlatformResponse{}, agentexecution.ErrInvalidRequest
	}
	request.Arguments = slices.Clone(request.Arguments)
	fingerprint := sha256.Sum256(append([]byte(request.Action+"\x00"), request.Arguments...))
	b.mu.Lock()
	defer b.mu.Unlock()
	grant, err := b.authorized(jobID, token)
	if err != nil {
		return agentexecution.PlatformResponse{}, err
	}
	if existing := grant.requests[request.ID]; existing != nil {
		if existing.fingerprint != fingerprint {
			return agentexecution.PlatformResponse{}, agentexecution.ErrIdempotencyConflict
		}
		return cloneResponse(existing.response), nil
	}
	if len(grant.requests) >= MaxRequests {
		return agentexecution.PlatformResponse{}, agentexecution.ErrOutputLimit
	}
	response := agentexecution.PlatformResponse{ID: request.ID, Status: "queued"}
	grant.requests[request.ID] = &receipt{request: request, fingerprint: fingerprint, response: response}
	grant.order = append(grant.order, request.ID)
	return response, nil
}

func (b *Broker) Receipt(jobID, token, requestID string) (agentexecution.PlatformResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	grant, err := b.authorized(jobID, token)
	if err != nil {
		return agentexecution.PlatformResponse{}, err
	}
	entry := grant.requests[requestID]
	if entry == nil {
		return agentexecution.PlatformResponse{}, agentexecution.ErrNotFound
	}
	return cloneResponse(entry.response), nil
}

// Pending requires the trusted Runtime Agent control credential at its caller.
// Claim each request once before handing it to the API. A lost claim or API
// restart must fail closed, not repeat an indeterminate billed/side-effecting
// action. Response delivery can be retried separately with Resolve.
func (b *Broker) Pending(jobID string) ([]agentexecution.PlatformRequest, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	grant, err := b.active(jobID)
	if err != nil {
		return nil, err
	}
	requests := make([]agentexecution.PlatformRequest, 0)
	for _, id := range grant.order {
		entry := grant.requests[id]
		if entry.response.Status == "queued" {
			entry.response.Status = "running"
			request := entry.request
			request.Arguments = slices.Clone(request.Arguments)
			requests = append(requests, request)
			if len(requests) == 4 {
				break
			}
		}
	}
	return requests, nil
}

func (b *Broker) Resolve(jobID string, response agentexecution.PlatformResponse) error {
	if !validID(response.ID) || len(response.Data) > MaxResponseBytes ||
		(len(response.Data) > 0 && !json.Valid(response.Data)) || !validStatus(response.Status) || !validErrorCode(response.ErrorCode) ||
		(response.Status == "failed" && (response.ErrorCode == "" || len(response.Data) != 0)) ||
		(response.Status != "failed" && response.ErrorCode != "") {
		return agentexecution.ErrInvalidRequest
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	grant, err := b.active(jobID)
	if err != nil {
		return err
	}
	entry := grant.requests[response.ID]
	if entry == nil {
		return agentexecution.ErrNotFound
	}
	if entry.response.Status != "queued" && entry.response.Status != "running" {
		old, _ := json.Marshal(entry.response)
		current, _ := json.Marshal(response)
		if string(old) != string(current) {
			return agentexecution.ErrIdempotencyConflict
		}
		return nil
	}
	entry.response = cloneResponse(response)
	return nil
}

func (b *Broker) authorized(jobID, token string) (*jobGrant, error) {
	grant, err := b.active(jobID)
	if err != nil || len(token) != 43 {
		return nil, agentexecution.ErrNotFound
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(digest[:], grant.digest[:]) != 1 {
		return nil, agentexecution.ErrNotFound
	}
	return grant, nil
}

func (b *Broker) active(jobID string) (*jobGrant, error) {
	grant := b.jobs[jobID]
	if grant == nil {
		return nil, agentexecution.ErrNotFound
	}
	if grant.ctx.Err() != nil {
		grant.cancel()
		grant.stopCleanup()
		delete(b.jobs, jobID)
		return nil, agentexecution.ErrNotFound
	}
	return grant, nil
}

func validRequest(request agentexecution.PlatformRequest) bool {
	if !validID(request.ID) || len(request.Arguments) == 0 || len(request.Arguments) > MaxRequestBytes {
		return false
	}
	switch request.Action {
	case "file.read", "file.export", "model.call", "tool.call":
	default:
		return false
	}
	var args map[string]json.RawMessage
	return json.Unmarshal(request.Arguments, &args) == nil && args != nil
}

func validStatus(status string) bool {
	return status == "completed" || status == "awaiting_approval" || status == "failed"
}

func validErrorCode(code string) bool {
	if code == "" {
		return true
	}
	if len(code) > 96 {
		return false
	}
	for _, ch := range code {
		if ch != '.' && ch != '_' && !(ch >= 'a' && ch <= 'z') && !(ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

func validID(value string) bool {
	return value != "" && len(value) <= 128 && strings.IndexFunc(value, func(ch rune) bool {
		return !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '-' && ch != '_' && ch != '.'
	}) == -1 && value != "." && value != ".."
}

func cloneResponse(response agentexecution.PlatformResponse) agentexecution.PlatformResponse {
	response.Data = slices.Clone(response.Data)
	return response
}
