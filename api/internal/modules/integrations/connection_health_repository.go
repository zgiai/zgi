package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConnectionHealthEventRepository interface {
	Record(ctx context.Context, observation ConnectionHealthObservation) (ConnectionHealthEvent, error)
	List(ctx context.Context, organizationID, connectionID uuid.UUID, page, pageSize int) ([]ConnectionHealthEvent, int64, error)
}

type GormConnectionHealthRepository struct {
	db *gorm.DB
}

func NewGormConnectionHealthRepository(db *gorm.DB) *GormConnectionHealthRepository {
	return &GormConnectionHealthRepository{db: db}
}

func (repository *GormConnectionHealthRepository) Record(ctx context.Context, observation ConnectionHealthObservation) (ConnectionHealthEvent, error) {
	if repository == nil || repository.db == nil {
		return ConnectionHealthEvent{}, fmt.Errorf("connection health repository is unavailable")
	}
	if observation.OrganizationID == uuid.Nil || observation.ConnectionID == uuid.Nil || observation.CredentialVersion < 1 {
		return ConnectionHealthEvent{}, invalidInput("connection health identity is required", nil)
	}
	observation = normalizeConnectionHealthObservation(observation)
	var recorded ConnectionHealthEvent
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if observation.ExecutionID != nil && *observation.ExecutionID != uuid.Nil {
			var existing ConnectionHealthEvent
			err := tx.Where("connection_id = ? AND execution_id = ? AND source = ?", observation.ConnectionID, *observation.ExecutionID, observation.Source).
				First(&existing).Error
			if err == nil {
				recorded = existing
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("find connection health event: %w", err)
			}
		}

		var connection IntegrationConnection
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND id = ?", observation.OrganizationID, observation.ConnectionID).
			First(&connection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConnectionNotFound
			}
			return fmt.Errorf("lock connection for health update: %w", err)
		}

		event := connectionHealthEventFromObservation(connection, observation)
		stale := connectionHealthObservationIsStale(connection, observation)
		if !stale && observation.Classification != ConnectionHealthClassificationIgnored {
			if !observation.SummaryAlreadyApplied {
				applyConnectionHealthObservation(&connection, observation, &event)
			}
			event.Applied = true
			event.HealthRevision = connection.HealthRevision
			event.HealthStatusAfter = connection.HealthStatus
			event.AuthStatusAfter = connection.AuthStatus
			event.ScopeStatusAfter = connection.ScopeStatus
			event.AttentionCodeAfter = cloneStringPointer(connection.AttentionCode)
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("create connection health event: %w", err)
		}
		if event.Applied && !observation.SummaryAlreadyApplied {
			updates := connectionHealthSummaryUpdates(connection)
			result := tx.Model(&IntegrationConnection{}).
				Where("organization_id = ? AND id = ? AND credential_version = ? AND health_revision = ?", connection.OrganizationID, connection.ID, observation.CredentialVersion, connection.HealthRevision-1).
				Updates(updates)
			if result.Error != nil {
				return fmt.Errorf("update connection health summary: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrConnectionChanged
			}
		}
		recorded = event
		return nil
	})
	return recorded, err
}

func connectionHealthObservationIsStale(connection IntegrationConnection, observation ConnectionHealthObservation) bool {
	if connection.CredentialVersion != observation.CredentialVersion {
		return true
	}
	if observation.ExpectedHealthRevision > 0 && connection.HealthRevision != observation.ExpectedHealthRevision {
		return true
	}
	var latest *time.Time
	for _, candidate := range []*time.Time{
		connection.LastHealthCheckedAt,
		connection.LastRuntimeSuccessAt,
		connection.LastRuntimeFailureAt,
		connection.ScopeCheckedAt,
	} {
		if candidate != nil && (latest == nil || candidate.After(*latest)) {
			latest = candidate
		}
	}
	return latest != nil && observation.ObservedAt.Before(*latest)
}

func (repository *GormConnectionHealthRepository) List(ctx context.Context, organizationID, connectionID uuid.UUID, page, pageSize int) ([]ConnectionHealthEvent, int64, error) {
	if repository == nil || repository.db == nil {
		return nil, 0, fmt.Errorf("connection health repository is unavailable")
	}
	if organizationID == uuid.Nil || connectionID == uuid.Nil {
		return nil, 0, invalidInput("organization and connection are required", nil)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := repository.db.WithContext(ctx).Model(&ConnectionHealthEvent{}).
		Where("organization_id = ? AND connection_id = ?", organizationID, connectionID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count connection health events: %w", err)
	}
	var events []ConnectionHealthEvent
	if err := query.Order("observed_at DESC, id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&events).Error; err != nil {
		return nil, 0, fmt.Errorf("list connection health events: %w", err)
	}
	return events, total, nil
}

func normalizeConnectionHealthObservation(observation ConnectionHealthObservation) ConnectionHealthObservation {
	observation.IntegrationID = strings.ToLower(strings.TrimSpace(observation.IntegrationID))
	observation.DriverID = strings.ToLower(strings.TrimSpace(observation.DriverID))
	observation.ReasonCode = boundedHealthValue(observation.ReasonCode, 64)
	observation.ProviderRequestID = boundedHealthValue(observation.ProviderRequestID, 128)
	observation.ErrorFingerprint = strings.ToLower(strings.TrimSpace(observation.ErrorFingerprint))
	if len(observation.ErrorFingerprint) != 64 {
		observation.ErrorFingerprint = ""
	}
	if observation.Source == "" {
		observation.Source = ConnectionHealthSourceRuntime
	}
	if observation.CheckKind == "" {
		observation.CheckKind = ConnectionHealthCheckPassive
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now().UTC()
	} else {
		observation.ObservedAt = observation.ObservedAt.UTC()
	}
	if observation.FailureThreshold < 1 {
		observation.FailureThreshold = 3
	}
	observation.GrantedScopes = normalizeScopes(observation.GrantedScopes)
	observation.MissingScopes = normalizeScopes(observation.MissingScopes)
	return observation
}

func connectionHealthEventFromObservation(connection IntegrationConnection, observation ConnectionHealthObservation) ConnectionHealthEvent {
	event := ConnectionHealthEvent{
		ID:                 uuid.New(),
		OrganizationID:     observation.OrganizationID,
		ConnectionID:       observation.ConnectionID,
		IntegrationID:      firstHealthValue(observation.IntegrationID, connection.IntegrationID),
		DriverID:           firstHealthValue(observation.DriverID, connection.DriverID),
		Source:             observation.Source,
		CheckKind:          observation.CheckKind,
		Classification:     observation.Classification,
		HealthStatusAfter:  connection.HealthStatus,
		AuthStatusAfter:    connection.AuthStatus,
		ScopeStatusAfter:   connection.ScopeStatus,
		AttentionCodeAfter: cloneStringPointer(connection.AttentionCode),
		CredentialVersion:  observation.CredentialVersion,
		HealthRevision:     connection.HealthRevision,
		ExecutionID:        cloneUUIDPointer(observation.ExecutionID),
		ActorID:            cloneUUIDPointer(observation.ActorID),
		ProviderHTTPStatus: observation.ProviderHTTPStatus,
		LatencyMS:          max(observation.LatencyMS, 0),
		RetryAfterAt:       cloneTimePointer(observation.RetryAfterAt),
		GrantedScopes:      append([]string(nil), observation.GrantedScopes...),
		MissingScopes:      append([]string(nil), observation.MissingScopes...),
		ObservedAt:         observation.ObservedAt,
	}
	if observation.ReasonCode != "" {
		event.ReasonCode = &observation.ReasonCode
	}
	if observation.ProviderRequestID != "" {
		event.ProviderRequestID = &observation.ProviderRequestID
	}
	if observation.ErrorFingerprint != "" {
		event.ErrorFingerprint = &observation.ErrorFingerprint
	}
	return event
}

func applyConnectionHealthObservation(connection *IntegrationConnection, observation ConnectionHealthObservation, event *ConnectionHealthEvent) {
	previousScopes := normalizeScopes(connection.GrantedScopes)
	if observation.Source == ConnectionHealthSourceRuntime {
		if observation.Classification == ConnectionHealthClassificationSuccess {
			connection.LastRuntimeSuccessAt = cloneTimePointer(&observation.ObservedAt)
		} else {
			connection.LastRuntimeFailureAt = cloneTimePointer(&observation.ObservedAt)
		}
	} else {
		connection.LastHealthCheckedAt = cloneTimePointer(&observation.ObservedAt)
	}

	switch observation.Classification {
	case ConnectionHealthClassificationSuccess:
		// A passive runtime success only proves that this particular request
		// completed. It must not clear an explicit reconnect/expired state set
		// by an active probe or credential lifecycle event. Recovery from those
		// states requires an active test or credential rotation.
		if observation.Source == ConnectionHealthSourceRuntime &&
			(connection.AuthStatus == ConnectionAuthReconnectRequired || connection.AuthStatus == ConnectionAuthExpired) {
			break
		}
		connection.AuthStatus = ConnectionAuthValid
		connection.ConsecutiveFailures = 0
		connection.LastHealthyAt = cloneTimePointer(&observation.ObservedAt)
		if connection.ScopeStatus == ConnectionScopeDrifted && !observation.ScopeSnapshotObserved {
			connection.HealthStatus = ConnectionHealthDegraded
			connection.AttentionCode = stringPointer(ConnectionAttentionScopeUpdateRequired)
		} else {
			connection.HealthStatus = ConnectionHealthHealthy
			connection.AttentionCode = nil
		}
	case ConnectionHealthClassificationAuthInvalid:
		connection.HealthStatus = ConnectionHealthUnhealthy
		connection.AuthStatus = ConnectionAuthReconnectRequired
		connection.AttentionCode = stringPointer(ConnectionAttentionReconnectRequired)
		connection.ConsecutiveFailures++
	case ConnectionHealthClassificationOAuthExpired:
		connection.HealthStatus = ConnectionHealthUnhealthy
		connection.AuthStatus = ConnectionAuthExpired
		connection.AttentionCode = stringPointer(ConnectionAttentionReconnectRequired)
		connection.ConsecutiveFailures++
	case ConnectionHealthClassificationScopeDrift:
		connection.HealthStatus = ConnectionHealthDegraded
		connection.ScopeStatus = ConnectionScopeDrifted
		connection.AttentionCode = stringPointer(ConnectionAttentionScopeUpdateRequired)
		connection.MissingRequiredScopes = append([]string(nil), observation.MissingScopes...)
	case ConnectionHealthClassificationAccessDenied:
		connection.HealthStatus = ConnectionHealthDegraded
		if len(observation.MissingScopes) > 0 {
			connection.ScopeStatus = ConnectionScopeDrifted
			connection.MissingRequiredScopes = append([]string(nil), observation.MissingScopes...)
			connection.AttentionCode = stringPointer(ConnectionAttentionScopeUpdateRequired)
		} else {
			connection.AttentionCode = stringPointer(ConnectionAttentionAdminCheckRequired)
		}
	case ConnectionHealthClassificationBudgetExhausted:
		connection.HealthStatus = ConnectionHealthDegraded
		connection.AttentionCode = stringPointer(ConnectionAttentionBillingRequired)
	case ConnectionHealthClassificationRateLimited:
		connection.HealthStatus = ConnectionHealthDegraded
		connection.ConsecutiveFailures++
	case ConnectionHealthClassificationProviderIncident:
		connection.ConsecutiveFailures++
		if connection.ConsecutiveFailures >= observation.FailureThreshold {
			connection.HealthStatus = ConnectionHealthDegraded
			connection.AttentionCode = stringPointer(ConnectionAttentionProviderIncident)
		}
	case ConnectionHealthClassificationTransient:
		connection.ConsecutiveFailures++
		if connection.ConsecutiveFailures >= observation.FailureThreshold {
			connection.HealthStatus = ConnectionHealthDegraded
		}
	}

	if observation.ScopeSnapshotObserved {
		connection.GrantedScopes = append([]string(nil), observation.GrantedScopes...)
		connection.ScopeCheckedAt = cloneTimePointer(&observation.ObservedAt)
		event.AddedScopes, event.RemovedScopes = scopeDiff(previousScopes, observation.GrantedScopes)
		if len(observation.MissingScopes) > 0 {
			connection.ScopeStatus = ConnectionScopeDrifted
			connection.MissingRequiredScopes = append([]string(nil), observation.MissingScopes...)
			connection.HealthStatus = ConnectionHealthDegraded
			connection.AttentionCode = stringPointer(ConnectionAttentionScopeUpdateRequired)
		} else {
			connection.ScopeStatus = ConnectionScopeVerified
			connection.MissingRequiredScopes = []string{}
		}
	}
	connection.HealthRevision++
}

func connectionHealthSummaryUpdates(connection IntegrationConnection) map[string]any {
	return map[string]any{
		"health_status":           connection.HealthStatus,
		"auth_status":             connection.AuthStatus,
		"scope_status":            connection.ScopeStatus,
		"attention_code":          connection.AttentionCode,
		"granted_scopes":          healthScopesJSON(connection.GrantedScopes),
		"missing_required_scopes": healthScopesJSON(connection.MissingRequiredScopes),
		"last_health_checked_at":  connection.LastHealthCheckedAt,
		"last_healthy_at":         connection.LastHealthyAt,
		"last_runtime_success_at": connection.LastRuntimeSuccessAt,
		"last_runtime_failure_at": connection.LastRuntimeFailureAt,
		"scope_checked_at":        connection.ScopeCheckedAt,
		"consecutive_failures":    connection.ConsecutiveFailures,
		"health_revision":         connection.HealthRevision,
		"updated_at":              gorm.Expr("CURRENT_TIMESTAMP"),
	}
}

func healthScopesJSON(scopes []string) datatypes.JSON {
	encoded, err := json.Marshal(normalizeScopes(scopes))
	if err != nil {
		return datatypes.JSON([]byte("[]"))
	}
	return datatypes.JSON(encoded)
}

func scopeDiff(previous, current []string) ([]string, []string) {
	previousSet := make(map[string]struct{}, len(previous))
	currentSet := make(map[string]struct{}, len(current))
	for _, scope := range previous {
		previousSet[scope] = struct{}{}
	}
	for _, scope := range current {
		currentSet[scope] = struct{}{}
	}
	added := make([]string, 0)
	removed := make([]string, 0)
	for scope := range currentSet {
		if _, exists := previousSet[scope]; !exists {
			added = append(added, scope)
		}
	}
	for scope := range previousSet {
		if _, exists := currentSet[scope]; !exists {
			removed = append(removed, scope)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func boundedHealthValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func firstHealthValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func stringPointer(value string) *string { return &value }
