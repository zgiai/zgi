package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
	"gorm.io/gorm"
)

func (s *promptService) ListOptimizationRuns(
	ctx context.Context,
	organizationID,
	accountID,
	promptID string,
	req promptdto.PromptOptimizationRunListRequest,
) (*promptdto.PromptOptimizationRunListResponse, error) {
	page, limit := normalizePageLimit(req.Page, req.Limit)

	prompt, err := s.getAccessiblePrompt(
		ctx,
		organizationID,
		accountID,
		promptID,
		promptOptimizePermissionCodes()...,
	)
	if err != nil {
		return nil, err
	}

	query := s.repo.DB().
		Model(&promptmodel.PromptOptimizationRun{}).
		Where("organization_id = ? AND account_id = ? AND prompt_id = ?", organizationID, accountID, prompt.ID)

	runs, total, err := s.repo.ListOptimizationRuns(ctx, query, page, limit)
	if err != nil {
		return nil, fmt.Errorf("list prompt optimization runs: %w", err)
	}

	items := make([]promptdto.PromptOptimizationRunResponse, 0, len(runs))
	for _, run := range runs {
		items = append(items, promptdto.BuildPromptOptimizationRunResponse(run))
	}

	return &promptdto.PromptOptimizationRunListResponse{
		Data:    items,
		HasMore: int64(page*limit) < total,
		Limit:   limit,
		Page:    page,
		Total:   total,
	}, nil
}

func (s *promptService) AdoptOptimizationRun(
	ctx context.Context,
	organizationID,
	accountID,
	promptID,
	runID string,
	req promptdto.PromptOptimizationAdoptRequest,
) (*promptdto.PromptDetailResponse, error) {
	prompt, err := s.getAccessiblePrompt(ctx, organizationID, accountID, promptID, promptVersionManagePermissionCodes()...)
	if err != nil {
		return nil, err
	}
	if prompt.Source == promptmodel.PromptSourceOfficial {
		return nil, fmt.Errorf("official prompts are read only")
	}

	run, err := s.repo.FindOptimizationRunByID(ctx, runID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("optimization run not found")
		}
		return nil, fmt.Errorf("load optimization run: %w", err)
	}
	if run.AccountID != accountID || run.OrganizationID != organizationID || run.PromptID == nil || *run.PromptID != prompt.ID {
		return nil, fmt.Errorf("optimization run not found")
	}

	variant := strings.TrimSpace(req.Variant)
	contentText, err := runVariantContent(run, variant)
	if err != nil {
		return nil, err
	}
	contentJSON, err := json.Marshal(contentText)
	if err != nil {
		return nil, fmt.Errorf("marshal optimized prompt content: %w", err)
	}
	content, config, promptType, labels, err := normalizeVersionInput(
		promptdto.PromptVersionInput{
			PromptType: "text",
			Content:    json.RawMessage(contentJSON),
			Config: map[string]any{
				"optimization_run_id": run.ID,
				"optimization_goal":   run.Goal,
				"optimization_model": map[string]any{
					"provider": derefString(run.Provider),
					"model":    derefString(run.Model),
				},
			},
			Labels: []string{},
		},
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("normalize optimized prompt version: %w", err)
	}
	version := &promptmodel.PromptVersion{
		PromptID:      prompt.ID,
		Version:       prompt.LatestVersion + 1,
		PromptType:    promptType,
		Content:       content,
		Config:        config,
		Labels:        labels,
		CommitMessage: promptOptimizationCommitMessage(req.CommitMessage, variant, run),
		CreatedBy:     &accountID,
	}

	err = s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var versions []*promptmodel.PromptVersion
		if err := tx.Where("prompt_id = ?", prompt.ID).Find(&versions).Error; err != nil {
			return err
		}
		if err := reassignLabels(tx, versions, version.Version, labels, false); err != nil {
			return err
		}

		prompt.LatestVersion = version.Version
		if err := tx.Save(prompt).Error; err != nil {
			return err
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}

		now := time.Now()
		run.AdoptedVariant = &variant
		run.AdoptedPromptVersionID = &version.ID
		run.AdoptedAt = &now
		return tx.Save(run).Error
	})
	if err != nil {
		return nil, fmt.Errorf("adopt optimization run: %w", err)
	}

	return s.buildDetail(ctx, prompt, accountID)
}

func runVariantContent(run *promptmodel.PromptOptimizationRun, variant string) (string, error) {
	switch variant {
	case "safe":
		if strings.TrimSpace(run.SafeOutput) == "" {
			return "", fmt.Errorf("safe optimization result is empty")
		}
		return run.SafeOutput, nil
	case "balanced":
		if strings.TrimSpace(run.BalancedOutput) == "" {
			return "", fmt.Errorf("balanced optimization result is empty")
		}
		return run.BalancedOutput, nil
	case "advanced":
		if strings.TrimSpace(run.AdvancedOutput) == "" {
			return "", fmt.Errorf("advanced optimization result is empty")
		}
		return run.AdvancedOutput, nil
	default:
		return "", fmt.Errorf("unsupported optimization variant")
	}
}

func promptOptimizationCommitMessage(
	commitMessage *string,
	variant string,
	run *promptmodel.PromptOptimizationRun,
) *string {
	if trimmed := trimOptional(commitMessage); trimmed != nil {
		return trimmed
	}

	message := fmt.Sprintf(
		"Adopt %s optimization run (%s / %s, goal=%s)",
		variant,
		derefString(run.Provider),
		derefString(run.Model),
		run.Goal,
	)
	return &message
}
