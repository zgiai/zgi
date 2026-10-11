// Package runtimeplatform provides run-owned authorization and idempotency for
// requests relayed from an isolated Job. It does not expose an HTTP endpoint or
// grant ordinary sandbox network access.
package runtimeplatform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

const (
	MaxGrantTTL      = 5 * time.Minute
	MaxRequestBytes  = 64 << 10
	MaxResponseBytes = 32 << 10
	MaxRequests      = 32
)

// Action identifies a platform operation, not a URL or arbitrary API route.
type Action string

const (
	ReadFile   Action = "file.read"
	ExportFile Action = "file.export"
	CallModel  Action = "model.call"
	CallTool   Action = "tool.call"
)

// Scope is supplied by the authenticated relay, never by script arguments.
// Changing any field requires a new session and invalidates the old grant.
type Scope struct {
	ExecutionIdentity execution.Identity
	OrganizationID    string
	SubjectID         string
	AgentID           string
	ConversationID    string
	RunID             string
	BindingID         string
	BindingGeneration int64
	JobID             string
}

// Request IDs are scoped to one session. Reusing an ID with different arguments
// is a conflict. Failed or indeterminate calls cannot be retried under that ID.
type Request struct {
	ID        string          `json:"id"`
	Action    Action          `json:"action"`
	Arguments json.RawMessage `json:"arguments"`
}

type Status string

const (
	Running          Status = "running"
	AwaitingApproval Status = "awaiting_approval"
	Completed        Status = "completed"
	Failed           Status = "failed"
)

// Response is bounded model-facing data. Transport adapters must not copy it
// into public audit traces without their own safe projection.
type Response struct {
	Status Status          `json:"status"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Options requires a live authorizer and a governed handler. Permission checks
// run on every request, including cached requests, and after execution. Handler
// errors must use AppError; their diagnostic text is never returned to scripts.
type Options struct {
	TTL       time.Duration
	Now       func() time.Time
	Authorize func(context.Context, Scope, Action) error
	Handle    func(context.Context, Scope, Request) (Response, error)
}

type requestState struct {
	fingerprint [32]byte
	response    Response
	err         error
}

// Session owns an ephemeral grant and bounded request receipts for a single
// Job. The lifecycle context is deliberately retained: canceling its Run closes
// the session and cancels handlers. Close must be called when the Job finishes.
// Nothing in this type may be serialized into a Job, log or Workspace snapshot.
type Session struct {
	scope       Scope
	digest      [32]byte
	expires     time.Time
	now         func() time.Time
	actions     map[Action]bool
	authorize   func(context.Context, Scope, Action) error
	handle      func(context.Context, Scope, Request) (Response, error)
	ctx         context.Context
	cancel      context.CancelFunc
	stopCleanup func() bool
	mu          sync.Mutex
	requests    map[string]*requestState
	closed      bool
}

// NewSession returns the raw grant exactly once. Only a trusted relay may
// deliver it to the corresponding Job; the Session retains only its hash.
func NewSession(parent context.Context, scope Scope, actions []Action, options Options) (*Session, string, error) {
	if parent == nil || parent.Err() != nil || !validScope(scope) ||
		options.TTL <= 0 || options.TTL > MaxGrantTTL || options.Authorize == nil || options.Handle == nil {
		return nil, "", invalid("runtime_platform.session")
	}
	allowed := make(map[Action]bool, len(actions))
	for _, action := range actions {
		if !validAction(action) || allowed[action] {
			return nil, "", invalid("runtime_platform.actions")
		}
		allowed[action] = true
	}
	if len(allowed) == 0 {
		return nil, "", invalid("runtime_platform.actions")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, "", apperror.Wrap(err, catalog.CodeInternal, apperror.WithOperation("runtime_platform.grant"))
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	clear(secret)
	ctx, cancel := context.WithTimeout(parent, options.TTL)
	session := &Session{
		scope: scope, digest: sha256.Sum256([]byte(token)), expires: options.Now().Add(options.TTL),
		now: options.Now, actions: allowed, authorize: options.Authorize, handle: options.Handle,
		ctx: ctx, cancel: cancel, requests: make(map[string]*requestState),
	}
	session.stopCleanup = context.AfterFunc(ctx, session.clear)
	return session, token, nil
}

// Close revokes the grant, cancels running calls and forgets cached results.
// It is idempotent. Callers must separately wait for owned handlers to exit.
func (s *Session) Close() {
	s.cancel()
	s.stopCleanup()
	s.clear()
}

func (s *Session) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	clear(s.requests)
}

// Execute validates a relayed request and starts its handler at most once.
// Concurrent duplicates receive running, not another handler invocation.
// Receipt retention is intentionally process-local: a restart loses all grants
// and cannot silently replay a possibly completed side effect.
func (s *Session) Execute(ctx context.Context, token string, scope Scope, request Request) (Response, error) {
	if ctx == nil || ctx.Err() != nil {
		return Response{}, apperror.New(catalog.CodeResourceConflict, apperror.WithOperation("runtime_platform.interrupted"))
	}
	if err := s.checkGrant(token, scope); err != nil {
		return Response{}, err
	}
	request, fingerprint, err := normalizeRequest(request)
	if err != nil {
		return Response{}, err
	}
	if !s.actions[request.Action] {
		return Response{}, denied("runtime_platform.action")
	}
	operationCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	if err := s.authorize(operationCtx, scope, request.Action); err != nil {
		return Response{}, apperror.Wrap(err, catalog.CodePermissionDenied, apperror.WithOperation("runtime_platform.authorization"))
	}
	s.mu.Lock()
	if !s.activeLocked() {
		s.mu.Unlock()
		return Response{}, unauthorized()
	}
	if previous, exists := s.requests[request.ID]; exists {
		if previous.fingerprint != fingerprint {
			s.mu.Unlock()
			return Response{}, apperror.New(catalog.CodeResourceConflict, apperror.WithOperation("runtime_platform.request_conflict"))
		}
		response, err := copyResponse(previous.response), previous.err
		s.mu.Unlock()
		return response, err
	}
	if len(s.requests) >= MaxRequests {
		s.mu.Unlock()
		return Response{}, apperror.New(catalog.CodeQuotaExceeded, apperror.WithOperation("runtime_platform.request_limit"))
	}
	state := &requestState{fingerprint: fingerprint, response: Response{Status: Running}}
	s.requests[request.ID] = state
	s.mu.Unlock()

	response, callErr := s.handle(operationCtx, scope, request)
	if callErr == nil && !validResponse(response) {
		callErr = invalid("runtime_platform.response")
	}
	if callErr == nil {
		// Authorization may have been revoked while a model or tool was busy.
		if err := s.authorize(operationCtx, scope, request.Action); err != nil {
			callErr = apperror.Wrap(err, catalog.CodePermissionDenied, apperror.WithOperation("runtime_platform.authorization"))
		}
	}
	if callErr != nil {
		response = Response{Status: Failed}
		if _, ok := apperror.As(callErr); !ok {
			callErr = apperror.Wrap(callErr, catalog.CodeInternal, apperror.WithOperation("runtime_platform.handler"))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.activeLocked() {
		return Response{}, unauthorized()
	}
	if operationCtx.Err() != nil {
		response = Response{Status: Failed}
		callErr = apperror.Wrap(operationCtx.Err(), catalog.CodeResourceConflict, apperror.WithOperation("runtime_platform.interrupted"))
	}
	state.response, state.err = copyResponse(response), callErr
	return copyResponse(response), callErr
}

func (s *Session) checkGrant(token string, scope Scope) error {
	if len(token) != 43 || scope != s.scope {
		return unauthorized()
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(digest[:], s.digest[:]) != 1 {
		return unauthorized()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.activeLocked() {
		return unauthorized()
	}
	return nil
}

func (s *Session) activeLocked() bool {
	return !s.closed && s.ctx.Err() == nil && s.now().Before(s.expires)
}

func normalizeRequest(request Request) (Request, [32]byte, error) {
	if !validIdentifier(request.ID) || !validAction(request.Action) ||
		len(request.Arguments) == 0 || len(request.Arguments) > MaxRequestBytes {
		return Request{}, [32]byte{}, invalid("runtime_platform.request")
	}
	var arguments map[string]json.RawMessage
	if err := json.Unmarshal(request.Arguments, &arguments); err != nil || arguments == nil {
		return Request{}, [32]byte{}, invalid("runtime_platform.arguments")
	}
	encoded, err := json.Marshal(arguments)
	if err != nil || len(encoded) > MaxRequestBytes {
		return Request{}, [32]byte{}, invalid("runtime_platform.arguments")
	}
	request.Arguments = encoded
	fingerprint := sha256.Sum256(append([]byte(string(request.Action)+"\x00"), encoded...))
	return request, fingerprint, nil
}

func validResponse(response Response) bool {
	return (response.Status == Completed || response.Status == AwaitingApproval) &&
		len(response.Data) <= MaxResponseBytes && (len(response.Data) == 0 || json.Valid(response.Data))
}

func copyResponse(response Response) Response {
	response.Data = append(json.RawMessage(nil), response.Data...)
	return response
}

func validScope(scope Scope) bool {
	if identity := scope.ExecutionIdentity; identity != (execution.Identity{}) {
		if identity.Validate() != nil || identity.ConversationID != scope.ConversationID ||
			(identity.ApplicationType == "agent" && identity.ApplicationID != scope.AgentID) ||
			(identity.ApplicationType == "digital_human" && scope.AgentID != "") {
			return false
		}
	} else if !validIdentifier(scope.AgentID) {
		return false
	}
	return validIdentifier(scope.OrganizationID) && validIdentifier(scope.SubjectID) &&
		validIdentifier(scope.ConversationID) &&
		validIdentifier(scope.RunID) && validIdentifier(scope.BindingID) &&
		validIdentifier(scope.JobID) && scope.BindingGeneration > 0
}

func validIdentifier(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func validAction(action Action) bool {
	return action == ReadFile || action == ExportFile || action == CallModel || action == CallTool
}

func invalid(operation string) error {
	return apperror.New(catalog.CodeRequestInvalid, apperror.WithOperation(operation))
}

func denied(operation string) error {
	return apperror.New(catalog.CodePermissionDenied, apperror.WithOperation(operation))
}

func unauthorized() error {
	return apperror.New(catalog.CodeAuthenticationRequired, apperror.WithOperation("runtime_platform.grant_invalid"))
}
