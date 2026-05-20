package dataset_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/indexing"
)

func TestRuntimeRouterPhaseOneTableRouting(t *testing.T) {
	router := indexing.NewRuntimeRouter(context.Background(), nil, nil, "")

	testCases := []struct {
		name        string
		input       indexing.RouterInput
		wantMatched bool
		wantRoute   string
		wantDocForm string
		wantMode    string
	}{
		{
			name: "xlsx upload file should match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "xlsx",
				ExtractedOutput: &dto.ExtractOutput{
					Elements: []dto.ExtractElement{
						{Type: "table", Content: `"Name":"Alice";"Score":"90"`},
					},
				},
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "xlsx with dot should match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         ".xlsx",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "xls upload file should match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         ".xls",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "uppercase xlsx should match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         ".XLSX",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "uppercase xls should match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "REPORT.XLS",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "file name fallback should match by xlsx ext",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "jilu.xlsx",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "file name fallback should match by xls ext",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "jilu.xls",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "file path fallback should match by xlsx ext",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "upload_files/team-1/jilu.xlsx",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "file path fallback should match by xls ext",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "upload_files/team-1/jilu.xls",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "docx should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         ".docx",
			},
			wantMatched: false,
		},
		{
			name: "docx without dot should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "docx",
			},
			wantMatched: false,
		},
		{
			name: "file name docx should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "report.docx",
			},
			wantMatched: false,
		},
		{
			name: "pdf should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         ".pdf",
			},
			wantMatched: false,
		},
		{
			name: "pdf without dot should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "pdf",
			},
			wantMatched: false,
		},
		{
			name: "file name pdf should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "report.pdf",
			},
			wantMatched: false,
		},
		{
			name: "csv should match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         ".csv",
			},
			wantMatched: true,
			wantRoute:   "table_model",
			wantDocForm: "table_model",
			wantMode:    "table",
		},
		{
			name: "no extension should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "report",
			},
			wantMatched: false,
		},
		{
			name: "empty extension should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "",
			},
			wantMatched: false,
		},
		{
			name: "whitespace extension should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         "   ",
			},
			wantMatched: false,
		},
		{
			name: "similar extension should not match",
			input: indexing.RouterInput{
				DataSourceType: "upload_file",
				DocExt:         ".xlsx.tmp",
			},
			wantMatched: false,
		},
		{
			name: "non upload file should not match xlsx",
			input: indexing.RouterInput{
				DataSourceType: "reading",
				DocExt:         ".xlsx",
			},
			wantMatched: false,
		},
		{
			name: "non upload file should not match xls path",
			input: indexing.RouterInput{
				DataSourceType: "reading",
				DocExt:         "upload_files/team-1/jilu.xls",
			},
			wantMatched: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			decision, err := router.Route(tc.input)

			require.NoError(t, err)
			require.NotNil(t, decision)
			require.Equal(t, tc.wantMatched, decision.Matched)

			if tc.wantMatched {
				require.Equal(t, tc.wantRoute, decision.RouteName)
				require.Equal(t, tc.wantDocForm, decision.TargetDocForm)
				require.Equal(t, tc.wantMode, decision.TargetMode)
				require.NotNil(t, decision.RouteMeta)
			} else {
				require.Empty(t, decision.RouteName)
				require.Empty(t, decision.TargetDocForm)
				require.Empty(t, decision.TargetMode)
			}
		})
	}
}
