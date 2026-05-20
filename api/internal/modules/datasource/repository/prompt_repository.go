package repository

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"gorm.io/gorm"
)

// PromptRepository defines the interface for table prompt repository
type PromptRepository interface {
	Create(ctx context.Context, prompt *model.TablePrompt) error
	FindByTableID(ctx context.Context, tableID string) (*model.TablePrompt, error)
	Update(ctx context.Context, prompt *model.TablePrompt) error
	Delete(ctx context.Context, id string) error
	DeleteByTableID(ctx context.Context, tableID string) error
}

// PostgresPromptRepository implements PromptRepository using postgres
type PostgresPromptRepository struct {
	db *gorm.DB
}

// NewPostgresPromptRepository creates a new PostgresPromptRepository
func NewPostgresPromptRepository(db *gorm.DB) PromptRepository {
	return &PostgresPromptRepository{db: db}
}

// Create creates a new table prompt record
func (r *PostgresPromptRepository) Create(ctx context.Context, prompt *model.TablePrompt) error {
	if prompt.ID == "" {
		prompt.ID = uuid.New().String()
	}

	if prompt.CreatedAt.IsZero() {
		prompt.CreatedAt = time.Now()
	}

	prompt.UpdatedAt = time.Now()

	query := `
		INSERT INTO data_source_table_prompts (
			id, table_id, prompt, created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	err := r.db.WithContext(ctx).Exec(
		query,
		prompt.ID,
		prompt.TableID,
		prompt.Prompt,
		prompt.CreatedBy,
		prompt.UpdatedBy,
		prompt.CreatedAt,
		prompt.UpdatedAt,
	).Error
	return err
}

// FindByTableID finds a table prompt by table ID
func (r *PostgresPromptRepository) FindByTableID(ctx context.Context, tableID string) (*model.TablePrompt, error) {
	var prompt model.TablePrompt
	err := r.db.WithContext(ctx).Where("table_id = ?", tableID).First(&prompt).Error
	if err != nil {
		// Handle both gorm.ErrRecordNotFound and table not exist error
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}

		// Check if it's a table not exist error
		// This is a PostgreSQL specific error with code "42P01"
		if strings.Contains(err.Error(), "42P01") || strings.Contains(err.Error(), "does not exist") {
			// Table does not exist, treat as no record found
			return nil, nil
		}

		return nil, err
	}

	return &prompt, nil
}

// Update updates a table prompt record
func (r *PostgresPromptRepository) Update(ctx context.Context, prompt *model.TablePrompt) error {
	prompt.UpdatedAt = time.Now()

	query := `
		UPDATE data_source_table_prompts
		SET prompt = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`
	err := r.db.WithContext(ctx).Exec(
		query,
		prompt.Prompt,
		prompt.UpdatedBy,
		prompt.UpdatedAt,
		prompt.ID,
	).Error
	return err
}

// Delete deletes a table prompt record
func (r *PostgresPromptRepository) Delete(ctx context.Context, id string) error {
	query := `
		DELETE FROM data_source_table_prompts
		WHERE id = ?
	`
	err := r.db.WithContext(ctx).Exec(query, id).Error
	return err
}

// DeleteByTableID deletes a table prompt record by table ID
func (r *PostgresPromptRepository) DeleteByTableID(ctx context.Context, tableID string) error {
	query := `
		DELETE FROM data_source_table_prompts
		WHERE table_id = ?
	`
	err := r.db.WithContext(ctx).Exec(query, tableID).Error
	return err
}
