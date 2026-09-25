// Package consolecred holds the per-instance credential that zgi-pre uses to call the
// Console control plane in Cloud mode.
//
// MERGE-NOTE(codex-video-image-rollback): the branch ships
// internal/infra/platform/consoleauth with the same shape (a Credentials type that
// attaches "authorization: Bearer <token>" to every RPC). This package is the
// main-based twin and additionally signs the instance HMAC headers the console
// interceptor verifies. Keep both; the branch version is Bearer-only.
//
// The credential is held in a process-wide store so the sync loop can clear it when the
// console link goes stale (plan P9 "credential invalidation" gate) without touching
// llm/gateway.
package consolecred

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

// Header names shared with the console control plane. The same spelling is used on the
// HTTP sync channel and in gRPC metadata (metadata keys are lower-cased by gRPC).
const (
	HeaderInstanceID    = "X-Internal-Instance-Id"
	HeaderTimestamp     = "X-Internal-Timestamp"
	HeaderSignature     = "X-Internal-Signature"
	HeaderAuthorization = "Authorization"
)

// Credential is the loaded per-instance identity. It is immutable once stored.
type Credential struct {
	InstanceID string
	Secret     string
}

// Store keeps the currently loaded credential and allows it to be cleared. A cleared
// store means "the console link is not trustworthy": no instance headers are sent, so
// the official channel fails closed on the console side.
type Store struct {
	mu   sync.RWMutex
	cred *Credential
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

// Clear drops the credential (stale gate).
func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cred = nil
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

// SignMessage builds the signed message exactly like the console internal channel:
// timestamp + "|" + path.
func SignMessage(timestamp, path string) string {
	return timestamp + "|" + path
}

// Sign returns the hex-encoded HMAC-SHA256 of timestamp|path using secret.
func Sign(secret, timestamp, path string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(SignMessage(timestamp, path)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Headers builds the instance authentication headers for one signed path, or nil when no
// credential is loaded.
func Headers(path string) map[string]string {
	cred, ok := global.Get()
	if !ok {
		return nil
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	return map[string]string{
		HeaderInstanceID: cred.InstanceID,
		HeaderTimestamp:  timestamp,
		HeaderSignature:  Sign(cred.Secret, timestamp, path),
	}
}

// BearerToken returns "Bearer <secret>" when a credential is loaded.
func BearerToken() string {
	cred, ok := global.Get()
	if !ok {
		return ""
	}
	return "Bearer " + cred.Secret
}
