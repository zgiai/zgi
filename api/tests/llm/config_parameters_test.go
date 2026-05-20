package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
)

func TestConfigParametersValueAndScan(t *testing.T) {
	precision := 2
	params := llmmodel.ConfigParameters{
		{
			Name:        "temperature",
			TemplateKey: "temperature",
			Type:        "float",
			Default:     json.RawMessage("1"),
			Min:         json.RawMessage("0"),
			Max:         json.RawMessage("2"),
			Precision:   &precision,
		},
	}

	value, err := params.Value()
	require.NoError(t, err)

	var decoded llmmodel.ConfigParameters
	require.NoError(t, decoded.Scan(value))
	require.Len(t, decoded, 1)
	assert.Equal(t, "temperature", decoded[0].Name)
	require.NotNil(t, decoded[0].Precision)
	assert.Equal(t, 2, *decoded[0].Precision)
}

func TestNormalizeConfigParametersJSONEmptyFallsBackToArray(t *testing.T) {
	params, err := llmmodel.NormalizeConfigParametersJSON(nil)
	require.NoError(t, err)
	assert.Empty(t, params)

	params, err = llmmodel.NormalizeConfigParametersJSON([]byte(""))
	require.NoError(t, err)
	assert.Empty(t, params)
}

func TestValidateConfigParametersRejectsInvalidShapes(t *testing.T) {
	t.Run("duplicate name", func(t *testing.T) {
		err := llmmodel.ValidateConfigParameters(llmmodel.ConfigParameters{
			{Name: "temperature", TemplateKey: "temperature", Type: "float"},
			{Name: "temperature", TemplateKey: "temperature_alt", Type: "float"},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate name")
	})

	t.Run("missing template key", func(t *testing.T) {
		err := llmmodel.ValidateConfigParameters(llmmodel.ConfigParameters{
			{Name: "temperature", Type: "float"},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "template_key is required")
	})

	t.Run("negative precision", func(t *testing.T) {
		negative := -1
		err := llmmodel.ValidateConfigParameters(llmmodel.ConfigParameters{
			{Name: "temperature", TemplateKey: "temperature", Type: "float", Precision: &negative},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "precision must be non-negative")
	})

	t.Run("default out of range", func(t *testing.T) {
		err := llmmodel.ValidateConfigParameters(llmmodel.ConfigParameters{
			{
				Name:        "temperature",
				TemplateKey: "temperature",
				Type:        "float",
				Default:     json.RawMessage("2.5"),
				Min:         json.RawMessage("0"),
				Max:         json.RawMessage("2"),
			},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "default must be less than or equal to max")
	})
}
