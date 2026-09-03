package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	workflowtoolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"gorm.io/gorm"
)

const defaultVideoPosterBackfillLimit = 50

type VideoPosterBackfillOptions struct {
	Apply          bool
	Limit          int
	TaskIDs        []string
	OrganizationID string
	AccountID      string
}

type VideoPosterBackfillResult struct {
	DryRun  bool
	Scanned int
	Updated int
	Failed  int
	Skipped int
	Items   []VideoPosterBackfillItem
}

type VideoPosterBackfillItem struct {
	TaskID    string
	Status    string
	VideoURL  string
	PosterURL string
	Error     string
}

func BackfillVideoTaskPosters(ctx context.Context, db *gorm.DB, opts VideoPosterBackfillOptions) (VideoPosterBackfillResult, error) {
	if db == nil {
		return VideoPosterBackfillResult{}, errors.New("database is nil")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultVideoPosterBackfillLimit
	}
	if limit > 500 {
		limit = 500
	}

	query := db.WithContext(ctx).
		Model(&videoTaskRecord{}).
		Where("status = ? AND video_url <> '' AND COALESCE(poster_url, '') = ''", "succeeded")

	taskIDs := normalizeBackfillTaskIDs(opts.TaskIDs)
	if len(taskIDs) > 0 {
		query = query.Where("task_id IN ?", taskIDs)
	}
	if organizationID := strings.TrimSpace(opts.OrganizationID); organizationID != "" {
		parsed, err := uuid.Parse(organizationID)
		if err != nil {
			return VideoPosterBackfillResult{}, fmt.Errorf("invalid organization id: %w", err)
		}
		query = query.Where("organization_id = ?", parsed)
	}
	if accountID := strings.TrimSpace(opts.AccountID); accountID != "" {
		parsed, err := uuid.Parse(accountID)
		if err != nil {
			return VideoPosterBackfillResult{}, fmt.Errorf("invalid account id: %w", err)
		}
		query = query.Where("account_id = ?", parsed)
	}

	var records []videoTaskRecord
	if err := query.Order("created_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return VideoPosterBackfillResult{}, fmt.Errorf("list video tasks without poster: %w", err)
	}

	result := VideoPosterBackfillResult{
		DryRun:  !opts.Apply,
		Scanned: len(records),
		Items:   make([]VideoPosterBackfillItem, 0, len(records)),
	}
	if !opts.Apply {
		result.Skipped = len(records)
		for _, record := range records {
			result.Items = append(result.Items, VideoPosterBackfillItem{
				TaskID:   record.TaskID,
				Status:   "dry_run",
				VideoURL: record.VideoURL,
			})
		}
		return result, nil
	}

	if workflowtoolfile.GlobalToolFileManager == nil {
		return result, errors.New("tool file manager is not initialized")
	}
	if workflowtoolfile.GlobalFileSignature == nil {
		return result, errors.New("tool file signature is not initialized")
	}

	backfillService := &service{
		artifactSaver:   defaultVideoArtifactSaver{},
		posterExtractor: extractVideoPoster,
	}
	for _, record := range records {
		item := VideoPosterBackfillItem{
			TaskID:   record.TaskID,
			VideoURL: record.VideoURL,
		}
		payload := mapFromJSON(record.ResponsePayload)
		scope := Scope{
			OrganizationID: record.OrganizationID,
			AccountID:      record.AccountID,
			WorkspaceID:    record.WorkspaceID,
		}
		posterURL := backfillService.generateVideoPoster(ctx, scope, record.VideoURL, payload)
		now := time.Now().UTC()
		updates := map[string]any{
			"response_payload": jsonData(payload),
			"updated_at":       now,
		}
		if posterURL == "" {
			item.Status = "failed"
			if errValue, ok := payload["poster_generation_error"].(string); ok {
				item.Error = errValue
			}
		} else {
			item.Status = "updated"
			item.PosterURL = posterURL
			updates["poster_url"] = posterURL
		}
		if err := db.WithContext(ctx).
			Model(&videoTaskRecord{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			item.Status = "failed"
			item.Error = fmt.Sprintf("update video task poster: %v", err)
		}
		if item.Status == "updated" {
			result.Updated++
		} else {
			result.Failed++
		}
		result.Items = append(result.Items, item)
	}

	return result, nil
}

func normalizeBackfillTaskIDs(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			taskID := strings.TrimSpace(item)
			if taskID == "" {
				continue
			}
			if _, ok := seen[taskID]; ok {
				continue
			}
			seen[taskID] = struct{}{}
			result = append(result, taskID)
		}
	}
	return result
}
