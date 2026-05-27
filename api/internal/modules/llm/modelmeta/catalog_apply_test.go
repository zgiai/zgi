package modelmeta

import (
	"testing"

	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func TestFeatureColumnsForPublishedModelIncludesAttachment(t *testing.T) {
	values := featureColumnsForPublishedModel(&llmmodel.ModelFeatures{
		Attachment: true,
	}, nil)

	require.True(t, values["attachment"])
}
