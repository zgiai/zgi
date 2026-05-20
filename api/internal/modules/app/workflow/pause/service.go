package pause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrPauseNotFound = errors.New("workflow pause not found")

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Save(ctx context.Context, params SaveParams) (*RunPause, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if params.State.Version == "" {
		params.State.Version = StateVersion
	}
	stateJSON, err := json.Marshal(params.State)
	if err != nil {
		return nil, fmt.Errorf("marshal workflow pause state: %w", err)
	}

	var pause RunPause
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing RunPause
		loadErr := tx.Where("workflow_run_id = ? AND resumed_at IS NULL", params.WorkflowRunID).First(&existing).Error
		if loadErr == nil {
			pause = existing
			if err := tx.Model(&pause).Updates(map[string]interface{}{
				"tenant_id":       params.TenantID,
				"app_id":          params.AppID,
				"node_id":         params.NodeID,
				"reason":          params.Reason,
				"conversation_id": nullableString(params.ConversationID),
				"state_json":      string(stateJSON),
				"created_at":      time.Now(),
				"resumed_at":      nil,
			}).Error; err != nil {
				return fmt.Errorf("update workflow pause: %w", err)
			}
			if err := tx.Where("pause_id = ?", pause.ID).Delete(&RunPauseReason{}).Error; err != nil {
				return fmt.Errorf("delete workflow pause reasons: %w", err)
			}
		} else if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			pause = RunPause{
				ID:             uuid.NewString(),
				TenantID:       params.TenantID,
				AppID:          params.AppID,
				WorkflowRunID:  params.WorkflowRunID,
				NodeID:         params.NodeID,
				Reason:         params.Reason,
				ConversationID: nullableString(params.ConversationID),
				StateJSON:      string(stateJSON),
				CreatedAt:      time.Now(),
			}
			if err := tx.Create(&pause).Error; err != nil {
				return fmt.Errorf("create workflow pause: %w", err)
			}
		} else {
			return fmt.Errorf("load workflow pause: %w", loadErr)
		}

		reasons := make([]RunPauseReason, 0, len(params.Reasons))
		for _, reason := range params.Reasons {
			reasons = append(reasons, RunPauseReason{
				ID:        uuid.NewString(),
				PauseID:   pause.ID,
				Type:      reason.Type,
				NodeID:    reason.NodeID,
				FormID:    reason.FormID,
				CreatedAt: time.Now(),
			})
		}
		if len(reasons) > 0 {
			if err := tx.Create(&reasons).Error; err != nil {
				return fmt.Errorf("create workflow pause reasons: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).First(&pause, "id = ?", pause.ID).Error; err != nil {
		return nil, fmt.Errorf("reload workflow pause: %w", err)
	}
	return &pause, nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) GetActiveByConversationID(ctx context.Context, tenantID, appID, conversationID, reason string) (*RunPause, []RunPauseReason, *State, error) {
	if s == nil || s.db == nil {
		return nil, nil, nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if conversationID == "" {
		return nil, nil, nil, ErrPauseNotFound
	}
	var pause RunPause
	query := s.db.WithContext(ctx).
		Where("conversation_id = ? AND resumed_at IS NULL", conversationID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if appID != "" {
		query = query.Where("app_id = ?", appID)
	}
	if reason != "" {
		query = query.Where("reason = ?", reason)
	}
	if err := query.Order("created_at DESC").First(&pause).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrPauseNotFound
		}
		return nil, nil, nil, fmt.Errorf("load workflow pause: %w", err)
	}

	var state State
	if err := json.Unmarshal([]byte(pause.StateJSON), &state); err != nil {
		return nil, nil, nil, fmt.Errorf("decode workflow pause state: %w", err)
	}
	var reasons []RunPauseReason
	if err := s.db.WithContext(ctx).
		Where("pause_id = ?", pause.ID).
		Order("created_at ASC").
		Find(&reasons).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("load workflow pause reasons: %w", err)
	}
	return &pause, reasons, &state, nil
}

func (s *Service) GetActiveByWorkflowRunID(ctx context.Context, workflowRunID string) (*RunPause, []RunPauseReason, *State, error) {
	if s == nil || s.db == nil {
		return nil, nil, nil, fmt.Errorf("workflow pause service is not initialized")
	}
	var pause RunPause
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND resumed_at IS NULL", workflowRunID).
		First(&pause).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrPauseNotFound
		}
		return nil, nil, nil, fmt.Errorf("load workflow pause: %w", err)
	}

	var state State
	if err := json.Unmarshal([]byte(pause.StateJSON), &state); err != nil {
		return nil, nil, nil, fmt.Errorf("decode workflow pause state: %w", err)
	}
	var reasons []RunPauseReason
	if err := s.db.WithContext(ctx).
		Where("pause_id = ?", pause.ID).
		Order("created_at ASC").
		Find(&reasons).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("load workflow pause reasons: %w", err)
	}
	return &pause, reasons, &state, nil
}

func (s *Service) MarkResumed(ctx context.Context, workflowRunID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("workflow pause service is not initialized")
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&RunPause{}).
		Where("workflow_run_id = ? AND resumed_at IS NULL", workflowRunID).
		Update("resumed_at", now).Error; err != nil {
		return fmt.Errorf("mark workflow pause resumed: %w", err)
	}
	return nil
}

func (s *Service) AppendEvent(ctx context.Context, params AppendEventParams) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("workflow pause service is not initialized")
	}
	eventJSON, err := json.Marshal(params.EventData)
	if err != nil {
		return fmt.Errorf("marshal workflow event data: %w", err)
	}
	var lastSequence int
	if err := s.db.WithContext(ctx).Model(&RunEvent{}).
		Where("workflow_run_id = ?", params.WorkflowRunID).
		Select("COALESCE(MAX(sequence), 0)").
		Scan(&lastSequence).Error; err != nil {
		return fmt.Errorf("load workflow event sequence: %w", err)
	}
	event := &RunEvent{
		ID:            uuid.NewString(),
		TenantID:      params.TenantID,
		AppID:         params.AppID,
		WorkflowRunID: params.WorkflowRunID,
		Sequence:      lastSequence + 1,
		EventType:     params.EventType,
		EventData:     string(eventJSON),
		CreatedAt:     time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("create workflow event: %w", err)
	}
	return nil
}

func (s *Service) LatestEventSequence(ctx context.Context, tenantID, workflowRunID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("workflow pause service is not initialized")
	}
	var sequence int
	query := s.db.WithContext(ctx).Model(&RunEvent{}).
		Where("workflow_run_id = ?", workflowRunID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil {
		return 0, fmt.Errorf("load workflow event sequence: %w", err)
	}
	return sequence, nil
}

func (s *Service) ListEvents(ctx context.Context, tenantID, workflowRunID string, afterSequence, limit int) (*RunEventsPayload, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if afterSequence < 0 {
		afterSequence = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	query := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND sequence > ?", workflowRunID, afterSequence)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}

	var events []RunEvent
	if err := query.Order("sequence ASC").Limit(limit).Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load workflow run events: %w", err)
	}

	payload := &RunEventsPayload{
		WorkflowRunID: workflowRunID,
		Events:        make([]RunEventPayload, 0, len(events)),
	}
	for _, event := range events {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(event.EventData), &data); err != nil {
			return nil, fmt.Errorf("decode workflow run event %s: %w", event.ID, err)
		}
		payload.Events = append(payload.Events, RunEventPayload{
			Sequence:  event.Sequence,
			Event:     event.EventType,
			Data:      data,
			CreatedAt: event.CreatedAt.Unix(),
		})
	}
	return payload, nil
}
