package sms

import (
	"encoding/json"
	"strconv"
	"strings"
)

const envPrefix = "NOTIFICATION_SMS_"

type lookupFunc func(string) (string, bool)

func ConfigFromLookup(lookup lookupFunc) Config {
	return Config{
		Enabled:         lookupBool(lookup, envPrefix+"ENABLED", false),
		Providers:       lookupCSV(lookup, envPrefix+"PROVIDERS"),
		DefaultProvider: strings.ToLower(lookupString(lookup, envPrefix+"DEFAULT_PROVIDER", "")),
		Template:        lookupString(lookup, envPrefix+"TEMPLATE", TemplatePendingActionNotification),
		PreviewTemplate: lookupString(lookup, envPrefix+"PREVIEW_TEMPLATE", ""),
		Aliyun: AliyunConfig{
			AccessKeyID:     lookupString(lookup, envPrefix+"ALIYUN_ACCESS_KEY_ID", ""),
			AccessKeySecret: lookupString(lookup, envPrefix+"ALIYUN_ACCESS_KEY_SECRET", ""),
			SignName:        lookupString(lookup, envPrefix+"ALIYUN_SIGN_NAME", ""),
			TemplateCode:    lookupString(lookup, envPrefix+"ALIYUN_TEMPLATE_CODE", ""),
			ParamMode:       lookupString(lookup, envPrefix+"ALIYUN_PARAM_MODE", ParamModeMap),
			ParamMap:        lookupStringMap(lookup, envPrefix+"ALIYUN_PARAM_MAP"),
			APIURL:          lookupString(lookup, envPrefix+"ALIYUN_API_URL", ""),
		},
		Chuanglan: ChuanglanConfig{
			Account:      lookupString(lookup, envPrefix+"CHUANGLAN_ACCOUNT", ""),
			Password:     lookupString(lookup, envPrefix+"CHUANGLAN_PASSWORD", ""),
			APIURL:       lookupString(lookup, envPrefix+"CHUANGLAN_API_URL", "https://smssh.253.com/msg/sms/v2/tpl/send"),
			TemplateID:   lookupString(lookup, envPrefix+"CHUANGLAN_TEMPLATE_ID", ""),
			Signature:    lookupString(lookup, envPrefix+"CHUANGLAN_SIGNATURE", ""),
			Extend:       lookupString(lookup, envPrefix+"CHUANGLAN_EXTEND", ""),
			Report:       lookupBool(lookup, envPrefix+"CHUANGLAN_REPORT", false),
			AuthMode:     lookupString(lookup, envPrefix+"CHUANGLAN_AUTH_MODE", "template"),
			TemplateText: lookupString(lookup, envPrefix+"CHUANGLAN_TEMPLATE_TEXT", ""),
			ParamMode:    lookupString(lookup, envPrefix+"CHUANGLAN_PARAM_MODE", ParamModeOrderedParam),
			ParamOrder:   lookupCSV(lookup, envPrefix+"CHUANGLAN_PARAM_ORDER"),
		},
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

func lookupCSV(lookup lookupFunc, key string) []string {
	raw := lookupString(lookup, key, "")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func lookupStringMap(lookup lookupFunc, key string) map[string]string {
	raw := lookupString(lookup, key, "")
	if raw == "" {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	for key, value := range result {
		if strings.TrimSpace(value) == "" {
			delete(result, key)
			continue
		}
		result[key] = strings.TrimSpace(value)
	}
	return result
}
