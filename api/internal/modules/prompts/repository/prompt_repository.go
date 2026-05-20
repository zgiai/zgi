package repository

import (
	"context"

	promptmodel "github.com/zgiai/ginext/internal/modules/prompts/model"
	"gorm.io/gorm"
)

type PromptRepository interface {
	DB() *gorm.DB
	List(ctx context.Context, query *gorm.DB, page, limit int) ([]*promptmodel.Prompt, int64, error)
	FindByID(ctx context.Context, id string) (*promptmodel.Prompt, error)
	FindLatestVersions(ctx context.Context, promptIDs []string) (map[string]*promptmodel.PromptVersion, error)
	FindVersions(ctx context.Context, promptID string) ([]*promptmodel.PromptVersion, error)
	ListOptimizationRuns(ctx context.Context, query *gorm.DB, page, limit int) ([]*promptmodel.PromptOptimizationRun, int64, error)
	FindOptimizationRunByID(ctx context.Context, id string) (*promptmodel.PromptOptimizationRun, error)
	Create(ctx context.Context, prompt *promptmodel.Prompt) error
	CreateVersion(ctx context.Context, version *promptmodel.PromptVersion) error
	CreateOptimizationRun(ctx context.Context, run *promptmodel.PromptOptimizationRun) error
	Update(ctx context.Context, prompt *promptmodel.Prompt) error
	UpdateOptimizationRun(ctx context.Context, run *promptmodel.PromptOptimizationRun) error
	UpdateVersionLabels(ctx context.Context, versionID string, labels []string) error
}

type promptRepository struct {
	db *gorm.DB
}

func NewPromptRepository(db *gorm.DB) PromptRepository {
	return &promptRepository{db: db}
}

func (r *promptRepository) DB() *gorm.DB {
	return r.db
}

func (r *promptRepository) List(ctx context.Context, query *gorm.DB, page, limit int) ([]*promptmodel.Prompt, int64, error) {
	var prompts []*promptmodel.Prompt
	var total int64
	if err := query.WithContext(ctx).Model(&promptmodel.Prompt{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	err := query.WithContext(ctx).
		Order("updated_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&prompts).Error
	return prompts, total, err
}

func (r *promptRepository) FindByID(ctx context.Context, id string) (*promptmodel.Prompt, error) {
	var prompt promptmodel.Prompt
	if err := r.db.WithContext(ctx).First(&prompt, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &prompt, nil
}

func (r *promptRepository) FindLatestVersions(ctx context.Context, promptIDs []string) (map[string]*promptmodel.PromptVersion, error) {
	result := make(map[string]*promptmodel.PromptVersion, len(promptIDs))
	if len(promptIDs) == 0 {
		return result, nil
	}
	var versions []*promptmodel.PromptVersion
	err := r.db.WithContext(ctx).
		Table("app_prompt_versions AS pv").
		Select("pv.*").
		Joins("JOIN app_prompts AS p ON p.id = pv.prompt_id AND p.latest_version = pv.version").
		Where("pv.prompt_id IN ?", promptIDs).
		Find(&versions).Error
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		result[version.PromptID] = version
	}
	return result, nil
}

func (r *promptRepository) FindVersions(ctx context.Context, promptID string) ([]*promptmodel.PromptVersion, error) {
	var versions []*promptmodel.PromptVersion
	err := r.db.WithContext(ctx).
		Where("prompt_id = ?", promptID).
		Order("version DESC").
		Find(&versions).Error
	return versions, err
}

func (r *promptRepository) ListOptimizationRuns(ctx context.Context, query *gorm.DB, page, limit int) ([]*promptmodel.PromptOptimizationRun, int64, error) {
	var runs []*promptmodel.PromptOptimizationRun
	var total int64
	if err := query.WithContext(ctx).Model(&promptmodel.PromptOptimizationRun{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	err := query.WithContext(ctx).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&runs).Error
	return runs, total, err
}

func (r *promptRepository) FindOptimizationRunByID(ctx context.Context, id string) (*promptmodel.PromptOptimizationRun, error) {
	var run promptmodel.PromptOptimizationRun
	if err := r.db.WithContext(ctx).First(&run, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *promptRepository) Create(ctx context.Context, prompt *promptmodel.Prompt) error {
	return r.db.WithContext(ctx).Create(prompt).Error
}

func (r *promptRepository) CreateVersion(ctx context.Context, version *promptmodel.PromptVersion) error {
	return r.db.WithContext(ctx).Create(version).Error
}

func (r *promptRepository) CreateOptimizationRun(ctx context.Context, run *promptmodel.PromptOptimizationRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *promptRepository) Update(ctx context.Context, prompt *promptmodel.Prompt) error {
	return r.db.WithContext(ctx).Save(prompt).Error
}

func (r *promptRepository) UpdateOptimizationRun(ctx context.Context, run *promptmodel.PromptOptimizationRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

func (r *promptRepository) UpdateVersionLabels(ctx context.Context, versionID string, labels []string) error {
	return r.db.WithContext(ctx).
		Model(&promptmodel.PromptVersion{}).
		Where("id = ?", versionID).
		Update("labels", labels).Error
}
