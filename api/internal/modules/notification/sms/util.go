package sms

import (
	"fmt"
	"regexp"
	"strings"
)

const maxNotificationTitleRunes = 64

var linkCodePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func MaskPhone(phone string) string {
	phones := splitPhoneNumbers(phone)
	if len(phones) > 1 {
		masked := make([]string, 0, len(phones))
		for _, item := range phones {
			masked = append(masked, MaskPhone(item))
		}
		return strings.Join(masked, ",")
	}
	if len(phones) == 1 {
		phone = phones[0]
	} else {
		phone = strings.TrimSpace(phone)
	}
	if len(phone) < 7 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

func NormalizePhoneNumbers(phone string) string {
	return strings.Join(splitPhoneNumbers(phone), ",")
}

func NormalizeTemplateParams(params map[string]string) map[string]string {
	normalized := make(map[string]string, len(params))
	for key, value := range params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			normalized[key] = value
		}
	}
	return normalized
}

func ValidateTemplateParams(template TemplateConfig, params map[string]string) error {
	if strings.TrimSpace(template.Key) == "" {
		return fmt.Errorf("notification sms template key is required")
	}

	known := make(map[string]TemplateParamConfig, len(template.Params))
	for _, param := range template.Params {
		known[param.Key] = param
		value := strings.TrimSpace(params[param.Key])
		if param.Required && value == "" {
			return fmt.Errorf("template param %s is required", param.Key)
		}
		if value == "" {
			continue
		}
		if param.MaxLength > 0 && len([]rune(value)) > param.MaxLength {
			return fmt.Errorf("template param %s must be at most %d characters", param.Key, param.MaxLength)
		}
		if param.Pattern != "" {
			pattern, err := regexp.Compile(param.Pattern)
			if err != nil {
				return fmt.Errorf("template param %s pattern is invalid: %w", param.Key, err)
			}
			if !pattern.MatchString(value) {
				return fmt.Errorf("template param %s contains unsupported characters", param.Key)
			}
		}
	}

	for key, value := range params {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := known[key]; !ok {
			return fmt.Errorf("template param %s is not defined by template %s", key, template.Key)
		}
	}
	return nil
}

func validateRequest(req Request, template TemplateConfig) error {
	if NormalizePhoneNumbers(req.Phone) == "" {
		return fmt.Errorf("phone is required")
	}
	if strings.TrimSpace(req.Template) == "" {
		return fmt.Errorf("template is required")
	}
	return ValidateTemplateParams(template, req.TemplateParams)
}

func splitPhoneNumbers(phone string) []string {
	normalized := strings.ReplaceAll(phone, "，", ",")
	parts := strings.Split(normalized, ",")
	phones := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			phones = append(phones, item)
		}
	}
	return phones
}
