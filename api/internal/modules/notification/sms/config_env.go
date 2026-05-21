package sms

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const envPrefix = "NOTIFICATION_SMS_"

type lookupFunc func(string) (string, bool)

func ConfigFromLookup(lookup lookupFunc) Config {
	enabled := lookupBool(lookup, envPrefix+"ENABLED", false)
	template := lookupString(lookup, envPrefix+"TEMPLATE", "")
	aliyun := AliyunConfig{
		AccessKeyID:     lookupString(lookup, envPrefix+"ALIYUN_ACCESS_KEY_ID", ""),
		AccessKeySecret: lookupString(lookup, envPrefix+"ALIYUN_ACCESS_KEY_SECRET", ""),
		SignName:        lookupString(lookup, envPrefix+"ALIYUN_SIGN_NAME", ""),
		APIURL:          lookupString(lookup, envPrefix+"ALIYUN_API_URL", ""),
	}
	chuanglan := ChuanglanConfig{
		Account:   lookupString(lookup, envPrefix+"CHUANGLAN_ACCOUNT", ""),
		Password:  lookupString(lookup, envPrefix+"CHUANGLAN_PASSWORD", ""),
		APIURL:    lookupString(lookup, envPrefix+"CHUANGLAN_API_URL", "https://smssh.253.com/msg/sms/v2/tpl/send"),
		Signature: lookupString(lookup, envPrefix+"CHUANGLAN_SIGNATURE", ""),
		Extend:    lookupString(lookup, envPrefix+"CHUANGLAN_EXTEND", ""),
		Report:    lookupBool(lookup, envPrefix+"CHUANGLAN_REPORT", false),
	}
	templates, configError := lookupTemplates(lookup, enabled)
	return Config{
		Enabled:         enabled,
		Providers:       lookupLowerCSV(lookup, envPrefix+"PROVIDERS"),
		DefaultProvider: strings.ToLower(lookupString(lookup, envPrefix+"DEFAULT_PROVIDER", "")),
		Template:        template,
		Templates:       templates,
		ConfigError:     configError,
		Aliyun:          aliyun,
		Chuanglan:       chuanglan,
	}
}

func lookupString(lookup lookupFunc, key string, defaultValue string) string {
	if lookup == nil {
		return defaultValue
	}
	if value, ok := lookup(key); ok {
		return strings.TrimSpace(value)
	}
	return defaultValue
}

func lookupBool(lookup lookupFunc, key string, defaultValue bool) bool {
	raw := lookupString(lookup, key, "")
	if raw == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func lookupLowerCSV(lookup lookupFunc, key string) []string {
	values := lookupCSV(lookup, key)
	for i, value := range values {
		values[i] = strings.ToLower(value)
	}
	return values
}

func lookupCSV(lookup lookupFunc, key string) []string {
	raw := lookupString(lookup, key, "")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func lookupTemplates(lookup lookupFunc, enabled bool) ([]TemplateConfig, string) {
	raw := lookupString(lookup, envPrefix+"TEMPLATES_JSON", "")
	if raw == "" {
		if enabled {
			return nil, envPrefix + "TEMPLATES_JSON is required when notification sms is enabled"
		}
		return nil, ""
	}

	var templates []TemplateConfig
	if err := json.Unmarshal([]byte(raw), &templates); err != nil {
		return nil, fmt.Sprintf("%sTEMPLATES_JSON is invalid: %v", envPrefix, err)
	}
	templates, err := normalizeTemplateConfigs(templates)
	if err != nil {
		return nil, err.Error()
	}
	return templates, ""
}

func normalizeTemplateConfigs(input []TemplateConfig) ([]TemplateConfig, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("notification sms templates must not be empty")
	}

	seenTemplates := make(map[string]struct{}, len(input))
	templates := make([]TemplateConfig, 0, len(input))
	for _, template := range input {
		template.Key = strings.TrimSpace(template.Key)
		if template.Key == "" {
			return nil, fmt.Errorf("notification sms template key is required")
		}
		if _, exists := seenTemplates[template.Key]; exists {
			return nil, fmt.Errorf("notification sms template %q is duplicated", template.Key)
		}
		seenTemplates[template.Key] = struct{}{}
		template.Name = strings.TrimSpace(template.Name)
		if template.Name == "" {
			template.Name = template.Key
		}
		template.PreviewTemplate = strings.TrimSpace(template.PreviewTemplate)
		template.Aliyun = normalizeAliyunTemplate(template.Aliyun)
		template.Chuanglan = normalizeChuanglanTemplate(template.Chuanglan)

		params, err := normalizeTemplateParams(template.Params, template.Aliyun, template.Chuanglan)
		if err != nil {
			return nil, fmt.Errorf("notification sms template %q: %w", template.Key, err)
		}
		template.Params = params
		if err := validateTemplateProviderParamMappings(template); err != nil {
			return nil, fmt.Errorf("notification sms template %q: %w", template.Key, err)
		}
		templates = append(templates, template)
	}
	return templates, nil
}

func normalizeAliyunTemplate(template AliyunTemplateConfig) AliyunTemplateConfig {
	template.TemplateCode = strings.TrimSpace(template.TemplateCode)
	template.ParamMode = normalizedParamMode(template.ParamMode, ParamModeMap)
	template.ParamMap = trimStringMap(template.ParamMap)
	return template
}

func normalizeChuanglanTemplate(template ChuanglanTemplateConfig) ChuanglanTemplateConfig {
	template.TemplateID = strings.TrimSpace(template.TemplateID)
	template.TemplateText = strings.TrimSpace(template.TemplateText)
	template.ParamMode = normalizedParamMode(template.ParamMode, ParamModeOrderedParam)
	template.ParamOrder = trimStringSlice(template.ParamOrder)
	return template
}

func normalizeTemplateParams(params []TemplateParamConfig, aliyun AliyunTemplateConfig, chuanglan ChuanglanTemplateConfig) ([]TemplateParamConfig, error) {
	if len(params) == 0 {
		keys := make([]string, 0, len(aliyun.ParamMap)+len(chuanglan.ParamOrder))
		seen := make(map[string]struct{})
		for key := range aliyun.ParamMap {
			if strings.TrimSpace(key) == "" {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		for _, key := range chuanglan.ParamOrder {
			if _, ok := seen[key]; ok || strings.TrimSpace(key) == "" {
				continue
			}
			keys = append(keys, key)
		}
		params = make([]TemplateParamConfig, 0, len(keys))
		for _, key := range keys {
			params = append(params, TemplateParamConfig{Key: key, Label: key, Required: boolPtr(true)})
		}
	}

	seen := make(map[string]struct{}, len(params))
	normalized := make([]TemplateParamConfig, 0, len(params))
	for _, param := range params {
		param.Key = strings.TrimSpace(param.Key)
		if param.Key == "" {
			return nil, fmt.Errorf("template param key is required")
		}
		if _, exists := seen[param.Key]; exists {
			return nil, fmt.Errorf("template param %q is duplicated", param.Key)
		}
		seen[param.Key] = struct{}{}
		param.Label = strings.TrimSpace(param.Label)
		if param.Label == "" {
			param.Label = param.Key
		}
		param.Pattern = strings.TrimSpace(param.Pattern)
		if param.Pattern != "" {
			if _, err := regexp.Compile(param.Pattern); err != nil {
				return nil, fmt.Errorf("template param %q pattern is invalid: %w", param.Key, err)
			}
		}
		normalized = append(normalized, param)
	}
	return normalized, nil
}

func validateTemplateProviderParamMappings(template TemplateConfig) error {
	if strings.TrimSpace(template.Aliyun.TemplateCode) != "" {
		if err := validateAliyunTemplateParamMapping(template.Params, template.Aliyun.ParamMap); err != nil {
			return err
		}
	}
	if strings.TrimSpace(template.Chuanglan.TemplateID) != "" ||
		strings.TrimSpace(template.Chuanglan.TemplateText) != "" ||
		len(template.Chuanglan.ParamOrder) > 0 {
		if err := validateChuanglanTemplateParamOrder(template.Params, template.Chuanglan.ParamOrder); err != nil {
			return err
		}
	}
	return nil
}

func validateAliyunTemplateParamMapping(params []TemplateParamConfig, paramMap map[string]string) error {
	for _, param := range params {
		if strings.TrimSpace(paramMap[param.Key]) == "" {
			return fmt.Errorf("aliyun param mapping for %s is required", param.Key)
		}
	}
	return nil
}

func validateChuanglanTemplateParamOrder(params []TemplateParamConfig, paramOrder []string) error {
	ordered := make(map[string]struct{}, len(paramOrder))
	for _, key := range paramOrder {
		ordered[strings.TrimSpace(key)] = struct{}{}
	}
	for _, param := range params {
		if _, ok := ordered[param.Key]; !ok {
			return fmt.Errorf("chuanglan param order for %s is required", param.Key)
		}
	}
	return nil
}

func trimStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

func trimStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
