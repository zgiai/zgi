package catalogsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/modelmeta"
)

func TestSynchronizerRecordsCatalogApplyReport(t *testing.T) {
	syncer := &Synchronizer{}
	syncer.recordCatalogApplyReport(context.Background(), modelmeta.CatalogApplyReport{
		Version:    3,
		Normalized: 1,
		Models: []modelmeta.CatalogModelDiff{{
			Provider:    "agicto",
			Model:       "gpt-x",
			ProviderKey: "agicto",
			VendorKey:   "agicto",
		}},
	})

	report := syncer.LastCatalogApplyReport()
	require.NotNil(t, report)
	require.Equal(t, int64(3), report.Version)
	require.Equal(t, 1, report.Normalized)
	require.Len(t, report.Models, 1)
	require.Equal(t, "agicto", report.Models[0].ProviderKey)
}
