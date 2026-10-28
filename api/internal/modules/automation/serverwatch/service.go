// Package serverwatch keeps task server watches (ADR 0031): a model-free check
// of a few systemd services that starts the watch's DigitalHuman task when a
// service or the host goes down.
package serverwatch

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
)

const (
	maxServices             = 10
	defaultIntervalMinutes  = 5
	defaultFailureThreshold = 2
	// triggerSpacing keeps alerts that arrive soon after a run for the next check.
	triggerSpacing = 10 * time.Minute
)

// Outcomes recorded on a watch after each check.
const (
	OutcomeHealthy            = "healthy"
	OutcomeDegraded           = "degraded"
	OutcomeTriggered          = "triggered"
	OutcomeSpacing            = "waiting_after_last_run"
	OutcomeConnectionUnusable = "connection_unusable"
	OutcomeCheckFailed        = "check_failed"
)

var serviceUnitPattern = regexp.MustCompile(serverops.SystemdServiceNamePattern)

// ServiceUnit is the systemd unit a watched service is read as: people
// usually type "nginx" for "nginx.service".
func ServiceUnit(name string) string {
	if strings.HasSuffix(name, ".service") {
		return name
	}
	return name + ".service"
}

// ErrNotUsable is returned by Connections and Prober when the DigitalHuman has
// no active binding on the connection or the connection is not ready.
var ErrNotUsable = errors.New("server watch connection is not usable")

// Watch is one stored watch.
type Watch struct {
	ID               uuid.UUID                                 `gorm:"type:uuid;primaryKey"`
	OrganizationID   uuid.UUID                                 `gorm:"type:uuid;not null"`
	WorkspaceID      uuid.UUID                                 `gorm:"type:uuid;not null"`
	DigitalHumanID   uuid.UUID                                 `gorm:"type:uuid;not null"`
	TaskID           uuid.UUID                                 `gorm:"type:uuid;not null"`
	OwnerAccountID   uuid.UUID                                 `gorm:"type:uuid;not null"`
	ConnectionID     uuid.UUID                                 `gorm:"type:uuid;not null"`
	Services         []string                                  `gorm:"type:jsonb;serializer:json;not null"`
	IntervalSeconds  int                                       `gorm:"not null"`
	FailureThreshold int                                       `gorm:"not null"`
	Enabled          bool                                      `gorm:"not null"`
	State            map[string]digitalhuman.ServerWatchTarget `gorm:"type:jsonb;serializer:json;not null"`
	NextCheckAt      time.Time                                 `gorm:"not null"`
	LastCheckedAt    *time.Time
	LastOutcome      string `gorm:"not null"`
	LastTriggeredAt  *time.Time
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (Watch) TableName() string { return "digital_human_server_watches" }

// Target names the connection a probe reads through.
type Target struct {
	OrganizationID uuid.UUID
	WorkspaceID    uuid.UUID
	OwnerAccountID uuid.UUID
	DigitalHumanID uuid.UUID
	ConnectionID   uuid.UUID
}

// Observation is one check. HostError is set when the host could not be
// reached or logged in to; Services is then empty.
type Observation struct {
	HostError string
	// Services maps each watched service to its systemd active state, or to
	// "not-found" or "check_failed".
	Services map[string]string
}

// Connections confirms the DigitalHuman may use one of the owner's connections.
type Connections interface {
	Usable(ctx context.Context, target Target) error
}

// Prober reads the watched services. It returns ErrNotUsable when the
// connection may no longer be used.
type Prober interface {
	Probe(ctx context.Context, target Target, services []string) (Observation, error)
}

// Trigger starts a run of the watch's task. It reports false with a reason
// when the task cannot start now.
type Trigger interface {
	TriggerServerEvent(ctx context.Context, taskID uuid.UUID, event map[string]any) (bool, string, error)
}

// Service implements digitalhuman.TaskServerWatchService and the sweep.
type Service struct {
	db          *gorm.DB
	connections Connections
	prober      Prober
	trigger     Trigger
	now         func() time.Time
}

func NewService(db *gorm.DB, connections Connections, prober Prober, trigger Trigger) *Service {
	return &Service{db: db, connections: connections, prober: prober, trigger: trigger, now: func() time.Time { return time.Now().UTC() }}
}

var _ digitalhuman.TaskServerWatchService = (*Service)(nil)

func (s *Service) GetServerWatch(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID) (*digitalhuman.ServerWatch, error) {
	watch, err := s.load(ctx, s.db, scope, digitalHumanID, taskID)
	if err != nil || watch == nil {
		return nil, err
	}
	return publicWatch(watch, scope.ActorID), nil
}

func (s *Service) PutServerWatch(ctx context.Context, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID, req digitalhuman.PutServerWatchRequest) (*digitalhuman.ServerWatch, error) {
	if s == nil || s.db == nil || s.connections == nil || scope.ActorID == uuid.Nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	existing, err := s.load(ctx, s.db, scope, digitalHumanID, taskID)
	if err != nil {
		return nil, err
	}
	enabled := req.Enabled == nil || *req.Enabled
	// Turning a watch off needs nothing else, and anyone who may change the
	// task may do it.
	if !enabled {
		if existing == nil {
			return nil, nil
		}
		existing.Enabled, existing.UpdatedAt = false, s.now()
		if err := s.db.WithContext(ctx).Model(existing).Updates(map[string]any{"enabled": false, "updated_at": existing.UpdatedAt}).Error; err != nil {
			return nil, fmt.Errorf("disable server watch: %w", err)
		}
		return publicWatch(existing, scope.ActorID), nil
	}
	services, err := normalizeServices(req.Services)
	if err != nil {
		return nil, err
	}
	interval := req.IntervalMinutes
	if interval == 0 {
		interval = defaultIntervalMinutes
	}
	threshold := req.FailureThreshold
	if threshold == 0 {
		threshold = defaultFailureThreshold
	}
	if interval < 1 || interval > 60 || threshold < 1 || threshold > 5 || req.ConnectionID == uuid.Nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	// Only the connection's owner points a watch at it, like a task grant.
	target := Target{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, OwnerAccountID: scope.ActorID, DigitalHumanID: digitalHumanID, ConnectionID: req.ConnectionID}
	if err := s.connections.Usable(ctx, target); err != nil {
		if errors.Is(err, ErrNotUsable) {
			return nil, digitalhuman.ErrServerWatchConnection
		}
		return nil, err
	}
	now := s.now()
	watch := existing
	if watch == nil {
		watch = &Watch{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID, TaskID: taskID, CreatedAt: now}
	}
	if watch.ConnectionID != req.ConnectionID || !sameServices(watch.Services, services) {
		watch.State = map[string]digitalhuman.ServerWatchTarget{}
		watch.LastOutcome = ""
	}
	watch.OwnerAccountID, watch.ConnectionID, watch.Services = scope.ActorID, req.ConnectionID, services
	watch.IntervalSeconds, watch.FailureThreshold, watch.Enabled = interval*60, threshold, enabled
	watch.NextCheckAt, watch.UpdatedAt = now, now
	if watch.State == nil {
		watch.State = map[string]digitalhuman.ServerWatchTarget{}
	}
	if err := s.db.WithContext(ctx).Save(watch).Error; err != nil {
		return nil, fmt.Errorf("save server watch: %w", err)
	}
	return publicWatch(watch, scope.ActorID), nil
}

func (s *Service) load(ctx context.Context, db *gorm.DB, scope digitalhuman.Scope, digitalHumanID, taskID uuid.UUID) (*Watch, error) {
	if s == nil || db == nil {
		return nil, digitalhuman.ErrInvalidRequest
	}
	var watch Watch
	err := db.WithContext(ctx).Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND task_id = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, taskID).Take(&watch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load server watch: %w", err)
	}
	return &watch, nil
}

// Sweep checks the watches that are due and returns how many it checked.
func (s *Service) Sweep(ctx context.Context, limit int) (int, error) {
	if s == nil || s.db == nil || s.prober == nil || s.trigger == nil {
		return 0, nil
	}
	now := s.now()
	var due []Watch
	if err := s.db.WithContext(ctx).Where("enabled AND next_check_at <= ?", now).Order("next_check_at").Limit(limit).Find(&due).Error; err != nil {
		return 0, fmt.Errorf("list due server watches: %w", err)
	}
	checked := 0
	for i := range due {
		watch := &due[i]
		// Claim the check so a concurrent sweep skips it.
		claim := s.db.WithContext(ctx).Model(&Watch{}).Where("id = ? AND next_check_at = ?", watch.ID, watch.NextCheckAt).
			Update("next_check_at", now.Add(time.Duration(watch.IntervalSeconds)*time.Second))
		if claim.Error != nil {
			return checked, fmt.Errorf("claim server watch: %w", claim.Error)
		}
		if claim.RowsAffected != 1 {
			continue
		}
		if err := s.check(ctx, watch, now); err != nil {
			return checked, err
		}
		checked++
	}
	return checked, nil
}

func (s *Service) check(ctx context.Context, watch *Watch, now time.Time) error {
	target := Target{OrganizationID: watch.OrganizationID, WorkspaceID: watch.WorkspaceID, OwnerAccountID: watch.OwnerAccountID, DigitalHumanID: watch.DigitalHumanID, ConnectionID: watch.ConnectionID}
	observation, err := s.prober.Probe(ctx, target, watch.Services)
	outcome := ""
	switch {
	case errors.Is(err, ErrNotUsable):
		outcome = OutcomeConnectionUnusable
	case err != nil:
		outcome = OutcomeCheckFailed
	default:
		var alerts []map[string]any
		watch.State, alerts = Evaluate(watch.State, watch.Services, observation, watch.FailureThreshold, now)
		outcome = OutcomeHealthy
		if unhealthy(watch.State) {
			outcome = OutcomeDegraded
		}
		if len(alerts) > 0 {
			outcome = s.raise(ctx, watch, alerts, now)
		}
	}
	watch.LastCheckedAt, watch.LastOutcome, watch.UpdatedAt = &now, outcome, now
	return s.db.WithContext(ctx).Model(&Watch{}).Where("id = ?", watch.ID).Updates(map[string]any{
		"state": watch.State, "last_checked_at": now, "last_outcome": outcome, "last_triggered_at": watch.LastTriggeredAt, "updated_at": now,
	}).Error
}

// raise starts the task for the alerts, or leaves them to be raised again.
func (s *Service) raise(ctx context.Context, watch *Watch, alerts []map[string]any, now time.Time) string {
	if watch.LastTriggeredAt != nil && now.Sub(*watch.LastTriggeredAt) < triggerSpacing {
		return OutcomeSpacing
	}
	event := map[string]any{"watch_id": watch.ID.String(), "connection_id": watch.ConnectionID.String(), "detected_at": now.Format(time.RFC3339), "targets": alerts}
	started, reason, err := s.trigger.TriggerServerEvent(ctx, watch.TaskID, event)
	if err != nil {
		return OutcomeCheckFailed
	}
	if !started {
		return reason
	}
	for _, alert := range alerts {
		name, _ := alert["name"].(string)
		state := watch.State[name]
		state.Alerted = true
		watch.State[name] = state
	}
	watch.LastTriggeredAt = &now
	return OutcomeTriggered
}

// Evaluate folds one observation into the watch state and returns the
// targets that reached the threshold without an alert in this outage.
func Evaluate(previous map[string]digitalhuman.ServerWatchTarget, services []string, observation Observation, threshold int, now time.Time) (map[string]digitalhuman.ServerWatchTarget, []map[string]any) {
	state := make(map[string]digitalhuman.ServerWatchTarget, len(services)+1)
	for _, name := range services {
		if value, ok := previous[name]; ok {
			state[name] = value
		}
	}
	if value, ok := previous[digitalhuman.ServerWatchHostKey]; ok {
		state[digitalhuman.ServerWatchHostKey] = value
	}
	var alerts []map[string]any
	observe := func(name string, healthy bool, observed string) {
		target := state[name]
		target.Observed, target.At = observed, now
		if healthy {
			target.Healthy, target.Failures, target.Alerted = true, 0, false
		} else {
			target.Healthy = false
			target.Failures++
			if target.Failures >= threshold && !target.Alerted {
				alerts = append(alerts, map[string]any{"name": name, "observed": observed, "failures": target.Failures})
			}
		}
		state[name] = target
	}
	if observation.HostError != "" {
		// Services are not judged while the host cannot be read.
		observe(digitalhuman.ServerWatchHostKey, false, observation.HostError)
		return state, alerts
	}
	observe(digitalhuman.ServerWatchHostKey, true, "reachable")
	for _, name := range services {
		observed := observation.Services[name]
		if observed == "" {
			observed = "check_failed"
		}
		observe(name, observed == "active", observed)
	}
	return state, alerts
}

func unhealthy(state map[string]digitalhuman.ServerWatchTarget) bool {
	for _, target := range state {
		if !target.Healthy {
			return true
		}
	}
	return false
}

func normalizeServices(values []string) ([]string, error) {
	seen := map[string]bool{}
	services := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" || seen[name] {
			continue
		}
		if !serviceUnitPattern.MatchString(ServiceUnit(name)) {
			return nil, digitalhuman.ErrInvalidRequest
		}
		seen[name] = true
		services = append(services, name)
	}
	if len(services) == 0 || len(services) > maxServices {
		return nil, digitalhuman.ErrInvalidRequest
	}
	return services, nil
}

func sameServices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func publicWatch(watch *Watch, viewer uuid.UUID) *digitalhuman.ServerWatch {
	state := watch.State
	if state == nil {
		state = map[string]digitalhuman.ServerWatchTarget{}
	}
	return &digitalhuman.ServerWatch{
		TaskID: watch.TaskID, ConnectionID: watch.ConnectionID, Services: append([]string{}, watch.Services...),
		IntervalMinutes: watch.IntervalSeconds / 60, FailureThreshold: watch.FailureThreshold, Enabled: watch.Enabled,
		OwnedByViewer: watch.OwnerAccountID == viewer, LastCheckedAt: watch.LastCheckedAt, LastOutcome: watch.LastOutcome,
		LastTriggeredAt: watch.LastTriggeredAt, State: state,
	}
}
