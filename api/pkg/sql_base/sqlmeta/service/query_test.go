package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

type stubQueryRepository struct {
	result *types.QueryResult
}

func (r stubQueryRepository) Execute(context.Context, string, types.QueryOptions) (*types.QueryResult, error) {
	return r.result, nil
}

func TestQueryServiceCarriesCommandTag(t *testing.T) {
	svc := NewQueryService(stubQueryRepository{result: &types.QueryResult{Command: "DELETE", RowsAffected: 4}})
	resp, err := svc.Execute(context.Background(), "DELETE FROM t", types.QueryOptions{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.Command != "DELETE" || resp.RowsAffected != 4 {
		t.Fatalf("response = %+v, want DELETE / 4", resp)
	}
}
