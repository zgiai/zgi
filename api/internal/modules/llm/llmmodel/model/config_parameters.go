package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

type ConfigParameter struct {
	Name        string          `json:"name"`
	TemplateKey string          `json:"template_key"`
	Label       json.RawMessage `json:"label,omitempty"`
	Type        string          `json:"type"`
	Help        json.RawMessage `json:"help,omitempty"`
	Required    bool            `json:"required"`
	Default     json.RawMessage `json:"default,omitempty"`
	Min         json.RawMessage `json:"min,omitempty"`
	Max         json.RawMessage `json:"max,omitempty"`
	Precision   *int            `json:"precision,omitempty"`
	Options     []string        `json:"options,omitempty"`
}

type ConfigParameters []ConfigParameter

func (p *ConfigParameter) UnmarshalJSON(data []byte) error {
	type configParameterAlias ConfigParameter
	raw := struct {
		configParameterAlias
		UseTemplate string `json:"use_template"`
	}{}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*p = ConfigParameter(raw.configParameterAlias)
	if strings.TrimSpace(p.TemplateKey) == "" {
		p.TemplateKey = raw.UseTemplate
	}
	return nil
}

func (p *ConfigParameters) Scan(value interface{}) error {
	if p == nil {
		return fmt.Errorf("failed to scan ConfigParameters: destination is nil")
	}

	switch v := value.(type) {
	case nil:
		*p = ConfigParameters{}
		return nil
	case []byte:
		return p.scanBytes(v)
	case string:
		return p.scanBytes([]byte(v))
	default:
		return fmt.Errorf("failed to scan ConfigParameters: expected []byte or string, got %T", value)
	}
}

func (p *ConfigParameters) scanBytes(raw []byte) error {
	normalized, err := NormalizeConfigParametersJSON(raw)
	if err != nil {
		return err
	}
	*p = normalized
	return nil
}

func (p ConfigParameters) Value() (driver.Value, error) {
	normalized := NormalizeConfigParameters(p)
	if err := ValidateConfigParameters(normalized); err != nil {
		return nil, err
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func NormalizeConfigParameters(params ConfigParameters) ConfigParameters {
	if params == nil {
		return ConfigParameters{}
	}

	normalized := make(ConfigParameters, 0, len(params))
	for _, param := range params {
		param.Name = strings.TrimSpace(param.Name)
		param.TemplateKey = strings.TrimSpace(param.TemplateKey)
		param.Type = strings.TrimSpace(param.Type)
		if param.TemplateKey == "" {
			param.TemplateKey = param.Name
		}
		normalized = append(normalized, param)
	}

	return normalized
}

func NormalizeConfigParametersJSON(raw []byte) (ConfigParameters, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ConfigParameters{}, nil
	}

	var params ConfigParameters
	if err := json.Unmarshal(trimmed, &params); err != nil {
		return nil, err
	}

	params = NormalizeConfigParameters(params)
	if err := ValidateConfigParameters(params); err != nil {
		return nil, err
	}

	return params, nil
}

func ValidateConfigParameters(params ConfigParameters) error {
	if len(params) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(params))
	for i, param := range params {
		index := i + 1
		if param.Name == "" {
			return fmt.Errorf("config_parameters[%d].name is required", index)
		}
		if param.TemplateKey == "" {
			return fmt.Errorf("config_parameters[%d].template_key is required", index)
		}
		if param.Type == "" {
			return fmt.Errorf("config_parameters[%d].type is required", index)
		}
		if _, exists := seen[param.Name]; exists {
			return fmt.Errorf("config_parameters contains duplicate name %q", param.Name)
		}
		seen[param.Name] = struct{}{}

		if param.Precision != nil && *param.Precision < 0 {
			return fmt.Errorf("config_parameters[%d].precision must be non-negative", index)
		}

		min, hasMin, err := parseNumericConfigValue(param.Min)
		if err != nil {
			return fmt.Errorf("config_parameters[%d].min must be numeric", index)
		}
		max, hasMax, err := parseNumericConfigValue(param.Max)
		if err != nil {
			return fmt.Errorf("config_parameters[%d].max must be numeric", index)
		}
		if hasMin && hasMax && min > max {
			return fmt.Errorf("config_parameters[%d].min must be less than or equal to max", index)
		}

		def, hasDefault, err := parseNumericConfigValue(param.Default)
		if err != nil {
			hasDefault = false
		}
		if hasDefault && hasMin && def < min {
			return fmt.Errorf("config_parameters[%d].default must be greater than or equal to min", index)
		}
		if hasDefault && hasMax && def > max {
			return fmt.Errorf("config_parameters[%d].default must be less than or equal to max", index)
		}
	}

	return nil
}

func parseNumericConfigValue(raw json.RawMessage) (float64, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, false, nil
	}

	var number float64
	if err := json.Unmarshal(trimmed, &number); err != nil {
		return 0, false, err
	}

	return number, true, nil
}
