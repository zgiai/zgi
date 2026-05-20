package modelmeta

import (
	"encoding/json"

	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func defaultConfigParameters() llmmodel.ConfigParameters {
	return llmmodel.ConfigParameters{}
}

func normalizeConfigParameters(raw json.RawMessage) llmmodel.ConfigParameters {
	normalized, err := llmmodel.NormalizeConfigParametersJSON(raw)
	if err != nil {
		return defaultConfigParameters()
	}
	return normalized
}

func serializeConfigParameters(raw json.RawMessage) string {
	data, err := json.Marshal(normalizeConfigParameters(raw))
	if err != nil {
		return "[]"
	}
	return string(data)
}
