package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

type CreateRequest struct {
	RuntimeProfile    string            `json:"runtime_profile"`
	TTLSeconds        int               `json:"ttl_seconds"`
	Metadata          map[string]string `json:"metadata"`
	NetworkEnabled    bool              `json:"network_enabled"`
	NetworkPolicy     string            `json:"network_policy"`
	DependencyProfile string            `json:"dependency_profile"`
	WorkspaceBinding  string            `json:"workspace_binding"`
}

type RegisterEndpointRequest struct {
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
	Scheme     string `json:"scheme"`
	Status     string `json:"status"`
}

type Store interface {
	SaveSandbox(sandbox.Sandbox) error
	GetSandbox(string) (*sandbox.Sandbox, error)
	ListSandboxes() ([]sandbox.Sandbox, error)
	CountActive(time.Time) (int, error)
	SaveEndpoint(sandbox.Endpoint) error
	GetEndpoint(string, string) (*sandbox.Endpoint, error)
}

type Cache interface {
	Get(context.Context, string) (*sandbox.Sandbox, bool, error)
	Set(context.Context, sandbox.Sandbox, time.Duration) error
	Delete(context.Context, string) error
}

type Manager struct {
	store         Store
	cache         Cache
	observer      *observer.Recorder
	policy        *policy.Service
	baseDir       string
	publicBaseURL string
	workerID      string
	workerAddr    string
}

func NewManager(recorder *observer.Recorder, policyService *policy.Service) (*Manager, error) {
	return NewManagerWithConfig(recorder, policyService, config.FromEnv(), newMemoryStore(), newNoopCache())
}

func NewManagerWithConfig(recorder *observer.Recorder, policyService *policy.Service, cfg config.Config, store Store, cache Cache) (*Manager, error) {
	if store == nil {
		store = newMemoryStore()
	}
	if cache == nil {
		cache = newNoopCache()
	}

	baseDir := filepath.Join(cfg.DataDir, "workspaces")
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absBaseDir, 0o755); err != nil {
		return nil, err
	}

	return &Manager{
		store:         store,
		cache:         cache,
		observer:      recorder,
		policy:        policyService,
		baseDir:       absBaseDir,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
		workerID:      cfg.WorkerID,
		workerAddr:    strings.TrimRight(cfg.AdvertiseURL, "/"),
	}, nil
}

func (m *Manager) Create(req CreateRequest) (*sandbox.Sandbox, error) {
	activeCount, err := m.store.CountActive(time.Now().UTC())
	if err != nil {
		return nil, err
	}

	decision, err := m.policy.NormalizeCreate(
		req.RuntimeProfile,
		req.TTLSeconds,
		req.NetworkEnabled,
		req.NetworkPolicy,
		req.DependencyProfile,
		activeCount,
	)
	if err != nil {
		return nil, err
	}

	id := "sbx_" + token()
	root := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	item := sandbox.Sandbox{
		ID:                id,
		RuntimeProfile:    decision.RuntimeProfile,
		Status:            sandbox.StatusActive,
		CreatedAt:         now,
		UpdatedAt:         now,
		ExpiresAt:         now.Add(decision.TTL),
		RootPath:          root,
		Metadata:          cloneMetadata(req.Metadata),
		NetworkEnabled:    decision.NetworkEnabled,
		NetworkPolicy:     decision.NetworkPolicy,
		DependencyProfile: decision.DependencyProfile,
		WorkspaceBinding:  strings.TrimSpace(req.WorkspaceBinding),
		TTLSeconds:        int(decision.TTL.Seconds()),
		WorkerID:          m.workerID,
		WorkerAddr:        m.workerAddr,
		EffectiveLimits:   &decision.EffectiveLimits,
	}

	if err := m.store.SaveSandbox(item); err != nil {
		return nil, err
	}
	_ = m.cache.Set(context.Background(), item, m.cacheTTL(item))

	m.observer.Record("sandbox.created", item.ID, "sandbox created", map[string]any{
		"runtime_profile":    item.RuntimeProfile,
		"ttl_seconds":        item.TTLSeconds,
		"network_policy":     item.NetworkPolicy,
		"dependency_profile": item.DependencyProfile,
		"worker_id":          item.WorkerID,
		"limit_decisions": map[string]any{
			"ttl_seconds":              item.TTLSeconds,
			"max_active_sandboxes":     decision.EffectiveLimits.MaxActiveSandboxes,
			"max_file_size_bytes":      decision.EffectiveLimits.MaxFileSizeBytes,
			"max_archive_files":        decision.EffectiveLimits.MaxArchiveFiles,
			"max_archive_total_bytes":  decision.EffectiveLimits.MaxArchiveTotalBytes,
			"network_policy_enforced":  decision.EffectiveLimits.NetworkPolicyEnforced,
			"workspace_bytes_enforced": decision.EffectiveLimits.WorkspaceByteLimitEnforced,
		},
	})

	copyItem := item
	return &copyItem, nil
}

func (m *Manager) Get(id string) (*sandbox.Sandbox, error) {
	if item, ok, err := m.cache.Get(context.Background(), id); err == nil && ok {
		if expired, updated, updateErr := m.expireIfNeeded(*item); updateErr != nil {
			return nil, updateErr
		} else if expired {
			return nil, errors.New("sandbox expired")
		} else if updated {
			_ = m.cache.Set(context.Background(), *item, m.cacheTTL(*item))
		}
		m.attachEffectiveLimits(item)
		return item, nil
	}

	item, err := m.store.GetSandbox(id)
	if err != nil {
		return nil, err
	}
	if expired, _, err := m.expireIfNeeded(*item); err != nil {
		return nil, err
	} else if expired {
		return nil, errors.New("sandbox expired")
	}

	_ = m.cache.Set(context.Background(), *item, m.cacheTTL(*item))
	m.attachEffectiveLimits(item)
	return item, nil
}

func (m *Manager) GetActive(id string) (*sandbox.Sandbox, error) {
	return m.Get(id)
}

func (m *Manager) List() []sandbox.Sandbox {
	items, err := m.store.ListSandboxes()
	if err != nil {
		return nil
	}

	filtered := make([]sandbox.Sandbox, 0, len(items))
	for _, item := range items {
		if expired, _, err := m.expireIfNeeded(item); err == nil && !expired {
			m.attachEffectiveLimits(&item)
			_ = m.cache.Set(context.Background(), item, m.cacheTTL(item))
			filtered = append(filtered, item)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	return filtered
}

func (m *Manager) Delete(id string) error {
	item, err := m.store.GetSandbox(id)
	if err != nil {
		return err
	}

	item.Status = sandbox.StatusDeleted
	item.UpdatedAt = time.Now().UTC()
	if err := m.store.SaveSandbox(*item); err != nil {
		return err
	}
	_ = m.cache.Delete(context.Background(), id)
	_ = os.RemoveAll(item.RootPath)
	m.observer.Record("sandbox.deleted", id, "sandbox deleted", map[string]any{
		"worker_id": item.WorkerID,
	})
	return nil
}

func (m *Manager) Renew(id string, ttlSeconds int) (*sandbox.Sandbox, error) {
	item, err := m.store.GetSandbox(id)
	if err != nil {
		return nil, err
	}
	if item.Status != sandbox.StatusActive {
		return nil, errors.New("sandbox is not active")
	}

	ttl, err := m.policy.NormalizeRenew(item.RuntimeProfile, ttlSeconds)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	item.ExpiresAt = now.Add(ttl)
	item.UpdatedAt = now
	item.TTLSeconds = int(ttl.Seconds())
	if err := m.store.SaveSandbox(*item); err != nil {
		return nil, err
	}
	_ = m.cache.Set(context.Background(), *item, m.cacheTTL(*item))

	m.observer.Record("sandbox.renewed", id, "sandbox renewed", map[string]any{
		"ttl_seconds": item.TTLSeconds,
	})

	copyItem := *item
	return &copyItem, nil
}

func (m *Manager) RegisterEndpoint(id string, port string, req RegisterEndpointRequest) (*sandbox.Endpoint, error) {
	item, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if item.RuntimeProfile != sandbox.RuntimeInteractive {
		return nil, errors.New("endpoint registration is only available for interactive sandboxes")
	}

	targetPort := req.TargetPort
	if targetPort <= 0 {
		targetPort, err = strconv.Atoi(port)
		if err != nil {
			return nil, errors.New("target port is required")
		}
	}

	now := time.Now().UTC()
	endpoint := sandbox.Endpoint{
		SandboxID:  id,
		Port:       port,
		URL:        m.endpointURL(id, port),
		Status:     defaultString(req.Status, "ready"),
		TargetHost: defaultString(req.TargetHost, "127.0.0.1"),
		TargetPort: targetPort,
		Scheme:     defaultString(req.Scheme, "http"),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := m.store.SaveEndpoint(endpoint); err != nil {
		return nil, err
	}

	m.observer.Record("sandbox.endpoint.registered", id, "sandbox endpoint registered", map[string]any{
		"port":        port,
		"target_host": endpoint.TargetHost,
		"target_port": endpoint.TargetPort,
		"scheme":      endpoint.Scheme,
	})

	return &endpoint, nil
}

func (m *Manager) ResolveEndpoint(id string, port string) (*sandbox.Endpoint, error) {
	return m.resolveEndpoint(id, port, true)
}

func (m *Manager) EndpointTarget(id string, port string) (*sandbox.Endpoint, error) {
	return m.resolveEndpoint(id, port, false)
}

func (m *Manager) resolveEndpoint(id string, port string, record bool) (*sandbox.Endpoint, error) {
	item, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if item.RuntimeProfile != sandbox.RuntimeInteractive {
		return nil, errors.New("endpoint resolution is only available for interactive sandboxes")
	}

	endpoint, err := m.store.GetEndpoint(id, port)
	if err != nil {
		targetPort, parseErr := strconv.Atoi(port)
		if parseErr != nil {
			return nil, err
		}
		endpoint = &sandbox.Endpoint{
			SandboxID:  id,
			Port:       port,
			URL:        m.endpointURL(id, port),
			Status:     "pending",
			TargetHost: "127.0.0.1",
			TargetPort: targetPort,
			Scheme:     "http",
		}
	}

	endpoint.URL = m.endpointURL(id, port)
	if record {
		m.observer.Record("sandbox.endpoint.resolved", id, "sandbox endpoint resolved", map[string]any{
			"port":        port,
			"url":         endpoint.URL,
			"target_host": endpoint.TargetHost,
			"target_port": endpoint.TargetPort,
		})
	}

	return endpoint, nil
}

func (m *Manager) cacheTTL(item sandbox.Sandbox) time.Duration {
	ttl := time.Until(item.ExpiresAt)
	if ttl <= 0 {
		return time.Second
	}
	if ttl > time.Minute {
		return time.Minute
	}
	return ttl
}

func (m *Manager) expireIfNeeded(item sandbox.Sandbox) (bool, bool, error) {
	if item.Status != sandbox.StatusActive {
		return true, false, nil
	}
	if time.Now().UTC().After(item.ExpiresAt) {
		item.Status = sandbox.StatusExpired
		item.UpdatedAt = time.Now().UTC()
		if err := m.store.SaveSandbox(item); err != nil {
			return true, false, err
		}
		_ = m.cache.Delete(context.Background(), item.ID)
		m.observer.Record("sandbox.expired", item.ID, "sandbox expired", nil)
		return true, true, nil
	}
	return false, false, nil
}

func (m *Manager) attachEffectiveLimits(item *sandbox.Sandbox) {
	if item == nil {
		return
	}
	limits := m.policy.EffectiveLimits()
	item.EffectiveLimits = &limits
}

func (m *Manager) endpointURL(id string, port string) string {
	base := strings.TrimRight(m.publicBaseURL, "/")
	return fmt.Sprintf("%s/_zgi/ports/%s/%s", base, id, port)
}

func cloneMetadata(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	target := make(map[string]string, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

func token() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

type memoryStore struct {
	mu        sync.RWMutex
	sandboxes map[string]*sandbox.Sandbox
	endpoints map[string]*sandbox.Endpoint
}

func newMemoryStore() Store {
	return &memoryStore{
		sandboxes: map[string]*sandbox.Sandbox{},
		endpoints: map[string]*sandbox.Endpoint{},
	}
}

func (s *memoryStore) SaveSandbox(item sandbox.Sandbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyItem := item
	s.sandboxes[item.ID] = &copyItem
	return nil
}

func (s *memoryStore) GetSandbox(id string) (*sandbox.Sandbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.sandboxes[id]
	if !ok {
		return nil, errors.New("sandbox not found")
	}
	copyItem := *item
	return &copyItem, nil
}

func (s *memoryStore) ListSandboxes() ([]sandbox.Sandbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]sandbox.Sandbox, 0, len(s.sandboxes))
	for _, item := range s.sandboxes {
		copyItem := *item
		items = append(items, copyItem)
	}
	return items, nil
}

func (s *memoryStore) CountActive(now time.Time) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, item := range s.sandboxes {
		if item.Status == sandbox.StatusActive && item.ExpiresAt.After(now) {
			count++
		}
	}
	return count, nil
}

func (s *memoryStore) SaveEndpoint(endpoint sandbox.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyEndpoint := endpoint
	s.endpoints[endpointKey(endpoint.SandboxID, endpoint.Port)] = &copyEndpoint
	return nil
}

func (s *memoryStore) GetEndpoint(sandboxID string, port string) (*sandbox.Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	endpoint, ok := s.endpoints[endpointKey(sandboxID, port)]
	if !ok {
		return nil, errors.New("sandbox endpoint not found")
	}
	copyEndpoint := *endpoint
	return &copyEndpoint, nil
}

func endpointKey(sandboxID string, port string) string {
	return sandboxID + ":" + port
}

type noopCache struct{}

func newNoopCache() Cache {
	return &noopCache{}
}

func (c *noopCache) Get(context.Context, string) (*sandbox.Sandbox, bool, error) {
	return nil, false, nil
}

func (c *noopCache) Set(context.Context, sandbox.Sandbox, time.Duration) error {
	return nil
}

func (c *noopCache) Delete(context.Context, string) error {
	return nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
