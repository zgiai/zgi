package workflowtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeFileGenerationConfigDefaultsToStandardContent(t *testing.T) {
	config := normalizeFileGenerationConfig(&FileGenerationConfig{
		Enabled: true,
	})

	require.True(t, config.Enabled)
	require.Equal(t, []string{"docx"}, config.Formats)
	require.Equal(t, 1, config.FilesPerCase)
	require.Equal(t, []string{"normal"}, config.Complexities)
}
