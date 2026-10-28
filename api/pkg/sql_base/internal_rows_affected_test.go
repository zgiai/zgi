package sql_base

import (
	"context"
	"testing"

	metaTypes "github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

type stubQueryService struct {
	resp *metaTypes.QueryResponse
}

func (s stubQueryService) Execute(context.Context, string, metaTypes.QueryOptions) (*metaTypes.QueryResponse, error) {
	return s.resp, nil
}

func TestInternalExecuteSQLReportsCommandTagRowsAffected(t *testing.T) {
	cases := []struct {
		name string
		resp *metaTypes.QueryResponse
		want int64
	}{
		{
			name: "insert without returning",
			resp: &metaTypes.QueryResponse{Command: "INSERT", RowsAffected: 3},
			want: 3,
		},
		{
			name: "update with returning",
			resp: &metaTypes.QueryResponse{Command: "UPDATE", RowsAffected: 2, Data: []metaTypes.QueryRow{{"id": 1}, {"id": 2}}},
			want: 2,
		},
		{
			name: "delete matching nothing",
			resp: &metaTypes.QueryResponse{Command: "DELETE", RowsAffected: 0},
			want: 0,
		},
		{
			name: "no command tag falls back to row count",
			resp: &metaTypes.QueryResponse{Data: []metaTypes.QueryRow{{"n": 1}}},
			want: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &internalClient{queryService: stubQueryService{resp: tc.resp}}
			result, err := client.executeSQL(context.Background(), "SELECT 1", nil)
			if err != nil {
				t.Fatalf("executeSQL: %v", err)
			}
			if result.RowsAffected != tc.want {
				t.Fatalf("RowsAffected = %d, want %d", result.RowsAffected, tc.want)
			}
		})
	}
}
