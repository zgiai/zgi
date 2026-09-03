package integrations

import (
	"context"

	"github.com/google/uuid"
)

type ExecutionRepository interface {
	Create(ctx context.Context, record *ExecutionRecord) error
	Complete(ctx context.Context, id uuid.UUID, completion ExecutionCompletion) error
}

type ExecutionListFilter struct {
	OrganizationID uuid.UUID
	IntegrationID  string
	ActionID       string
	Status         string
	ConnectionID   *uuid.UUID
	Page           int
	PageSize       int
}

type ExecutionListPage struct {
	Items    []ExecutionRecord `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type ExecutionQueryRepository interface {
	List(ctx context.Context, filter ExecutionListFilter) (ExecutionListPage, error)
}
