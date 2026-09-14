package music

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	workflowtoolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const defaultMusicWaveformBackfillLimit = 50

type WaveformBackfillOptions struct {
	Apply          bool
	Limit          int
	TaskIDs        []string
	OrganizationID string
	AccountID      string
}

type WaveformBackfillResult struct {
	DryRun  bool
	Scanned int
	Updated int
	Failed  int
	Skipped int
	Items   []WaveformBackfillItem
}

type WaveformBackfillItem struct {
	TaskID     string
	FileID     string
	Status     string
	DurationMS int64
	PeakCount  int
	Error      string
}

func BackfillMusicTaskWaveforms(ctx context.Context, db *gorm.DB, opts WaveformBackfillOptions) (WaveformBackfillResult, error) {
	if db == nil {
		return WaveformBackfillResult{}, errors.New("database is nil")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultMusicWaveformBackfillLimit
	}
	if limit > 500 {
		limit = 500
	}

	query := db.WithContext(ctx).
		Model(&Task{}).
		Where("status = ? AND file_id IS NOT NULL", StatusSucceeded).
		Where("duration_ms <= 0 OR COALESCE(jsonb_array_length(waveform_peaks), 0) = 0")

	taskIDs := normalizeMusicBackfillTaskIDs(opts.TaskIDs)
	if len(taskIDs) > 0 {
		query = query.Where("id IN ?", taskIDs)
	}
	if organizationID := strings.TrimSpace(opts.OrganizationID); organizationID != "" {
		parsed, err := uuid.Parse(organizationID)
		if err != nil {
			return WaveformBackfillResult{}, fmt.Errorf("invalid organization id: %w", err)
		}
		query = query.Where("organization_id = ?", parsed)
	}
	if accountID := strings.TrimSpace(opts.AccountID); accountID != "" {
		parsed, err := uuid.Parse(accountID)
		if err != nil {
			return WaveformBackfillResult{}, fmt.Errorf("invalid account id: %w", err)
		}
		query = query.Where("account_id = ?", parsed)
	}

	var tasks []Task
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&tasks).Error; err != nil {
		return WaveformBackfillResult{}, fmt.Errorf("list music tasks without waveform: %w", err)
	}

	result := WaveformBackfillResult{
		DryRun:  !opts.Apply,
		Scanned: len(tasks),
		Items:   make([]WaveformBackfillItem, 0, len(tasks)),
	}
	if !opts.Apply {
		result.Skipped = len(tasks)
		for _, task := range tasks {
			result.Items = append(result.Items, WaveformBackfillItem{
				TaskID: task.ID.String(),
				FileID: task.FileID.String(),
				Status: "dry_run",
			})
		}
		return result, nil
	}

	if workflowtoolfile.GlobalToolFileManager == nil {
		return result, errors.New("tool file manager is not initialized")
	}

	for _, task := range tasks {
		item := WaveformBackfillItem{
			TaskID: task.ID.String(),
			FileID: task.FileID.String(),
		}
		audio, _, err := workflowtoolfile.GetFileBinaryGlobal(ctx, task.FileID.String())
		if err != nil {
			item.Status = "failed"
			item.Error = fmt.Sprintf("load music file: %v", err)
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		metadata, err := extractGeneratedMusicPlaybackMetadata(ctx, audio)
		if err != nil {
			item.Error = err.Error()
		}
		updates := map[string]any{
			"updated_at": time.Now().UTC(),
		}
		if metadata.DurationMS > 0 {
			item.DurationMS = metadata.DurationMS
			updates["duration_ms"] = metadata.DurationMS
		}
		if len(metadata.WaveformPeaks) > 0 {
			item.PeakCount = len(metadata.WaveformPeaks)
			updates["waveform_peaks"] = datatypes.NewJSONSlice(metadata.WaveformPeaks)
		}
		if len(updates) == 1 {
			item.Status = "failed"
			if item.Error == "" {
				item.Error = "music playback metadata is empty"
			}
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if err := db.WithContext(ctx).
			Model(&Task{}).
			Where("id = ?", task.ID).
			Updates(updates).Error; err != nil {
			item.Status = "failed"
			item.Error = fmt.Sprintf("update music task waveform: %v", err)
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		item.Status = "updated"
		result.Updated++
		result.Items = append(result.Items, item)
	}

	return result, nil
}

func normalizeMusicBackfillTaskIDs(values []string) []string {
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
