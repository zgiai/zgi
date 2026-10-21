// Package consolecred holds the per-instance credential that Pre uses to call the
// Console control plane in Cloud mode.
//
// MERGE-NOTE(codex-video-image-rollback): the branch ships
// internal/infra/platform/consoleauth with the same shape (a Credentials type that
// attaches "authorization: Bearer <token>" to every RPC). This package is the
// main-based twin and additionally signs the instance HMAC headers the console
// interceptor verifies. Keep both; the branch version is Bearer-only.
//
// The credential is held in a process-wide store. The console link's "stale" state is a
// separate flag on that store: the sync loop flips it when the link goes stale while the
// credential itself stays loaded, so the sync/inventory client can keep signing and repair
// the link. Official-channel callers (HTTP SignRequest, gRPC GRPCCredentials, the official
// credit checker) consult the flag and fail closed while it is set.
package consolecred

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
)

// ErrConsoleLinkStale is returned to official-channel callers while the sync loop has
// marked the console link stale. It is intentionally not a console API error: the call is
// refused locally instead of being sent unsigned.
var ErrConsoleLinkStale = errors.New("console link stale: official channel suspended")

// Header names shared with the console control plane. The same spelling is used on the
// HTTP sync channel and in gRPC metadata (metadata keys are lower-cased by gRPC).
const (
	HeaderInstanceID    = "X-Internal-Instance-Id"
	HeaderTimestamp     = "X-Internal-Timestamp"
	HeaderSignature     = "X-Internal-Signature"
	HeaderNonce         = "X-Internal-Nonce"
	HeaderAuthorization = "Authorization"
)

// nonceBytes is the number of random bytes behind one request nonce. The nonce travels as
// lowercase hex, so a nonce is always 2*nonceBytes = 32 characters long.
const nonceBytes = 16

// emptyBodyDigest is hex(sha256("")): the digest that commits a request without a body to
// the empty payload.
const emptyBodyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Credential is the loaded per-instance identity. It is immutable once stored.
type Credential struct {
	InstanceID string
	Secret     string
}

// Store keeps the currently loaded credential plus the console link's stale state.
//
// The credential and the stale flag are deliberately independent:
//   - the credential is what the sync/inventory client signs with, and it must stay loaded
//     while the link is stale, otherwise the sync loop can never authenticate again and
//     the stale state becomes a self-lock;
//   - the stale flag is what the official channel is gated on.
type Store struct {
	mu    sync.RWMutex
	cred  *Credential
	stale bool
}

// NewStore creates an empty credential store.
func NewStore() *Store { return &Store{} }

// Set loads (or replaces) the credential.
func (s *Store) Set(instanceID, secret string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if instanceID == "" || secret == "" {
		s.cred = nil
		return
	}
	s.cred = &Credential{InstanceID: instanceID, Secret: secret}
}

// Clear drops the credential (explicit credential.disable revocation). It also resets the
// stale flag: a store without a credential carries no link state.
func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cred = nil
	s.stale = false
}

// SetStale marks the console link stale (or healthy) without touching the loaded
// credential. The sync/inventory client ignores this flag and keeps signing; the official
// channel must honour it and fail closed.
func (s *Store) SetStale(stale bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stale = stale
}

// Stale reports whether the console link has been marked stale by the sync loop. It is the
// explicit accessor official-channel callers use to gate themselves.
func (s *Store) Stale() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stale
}

// Get returns the loaded credential.
func (s *Store) Get() (*Credential, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cred == nil {
		return nil, false
	}
	copy := *s.cred
	return &copy, true
}

// Loaded reports whether a credential is currently available.
func (s *Store) Loaded() bool {
	_, ok := s.Get()
	return ok
}

var global = NewStore()

// Global returns the process-wide credential store.
func Global() *Store { return global }

// Configure loads the instance credential from configuration into the global store.
func Configure(instanceID, secret string) {
	global.Set(instanceID, secret)
}

// NewNonce returns a fresh per-request nonce: 16 random bytes encoded as 32 lowercase hex
// characters. The nonce makes every signed request unique, so a captured header set cannot
// be replayed inside the timestamp window even when method, path and body are unchanged.
func NewNonce() (string, error) {
	buf := make([]byte, nonceBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// BodyDigest returns the lowercase hex SHA-256 of body. An empty body digests as
// sha256(""), so the signature always commits to the payload that is actually sent.
func BodyDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// SignedString builds the string covered by the instance signature:
//
//	nonce + "|" + timestamp + "|" + METHOD + "|" + path + "|" + bodyDigest
//
// METHOD is the uppercase HTTP method and path is the request path ONLY: the query string
// travels on the wire but is never signed. bodyDigest is hex(sha256(body)) for the exact
// bytes sent. Callers must build nothing themselves: every HTTP caller goes through
// SignRequest/SignSyncRequest (or the raw Headers accessor), which use this helper.
func SignedString(nonce, timestamp, method, path, bodyDigest string) string {
	return nonce + "|" + timestamp + "|" + method + "|" + path + "|" + bodyDigest
}

// Sign returns the lowercase hex HMAC-SHA256, under secret, of the signed string assembled
// from its parts. Splitting the parts out keeps every caller off the string format.
func Sign(secret, nonce, timestamp, method, path, bodyDigest string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(SignedString(nonce, timestamp, method, path, bodyDigest)))
	return hex.EncodeToString(mac.Sum(nil))
}

// BearerToken returns "Bearer <secret>" when a credential is loaded.
func BearerToken() string {
	cred, ok := global.Get()
	if !ok {
		return ""
	}
	return "Bearer " + cred.Secret
}
