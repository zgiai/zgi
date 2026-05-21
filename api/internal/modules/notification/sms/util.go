package sms

import (
	"fmt"
	"regexp"
	"strings"
)

const maxNotificationTitleRunes = 64

var linkSuffixPattern = regexp.MustCompile(`^[A-Za-z0-9/_?=&.%+-]+$`)

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

func ValidateNotificationContent(template, notificationTitle, linkSuffix string) error {
	return ValidateNotificationTemplateParams(template, map[string]string{
		TemplateParamNotificationTitle: strings.TrimSpace(notificationTitle),
		TemplateParamLinkSuffix:        strings.TrimSpace(linkSuffix),
	})
}

func ValidateNotificationTemplateParams(template string, params map[string]string) error {
	if strings.TrimSpace(template) == "" {
		template = TemplatePendingActionNotification
	}
	if template != TemplatePendingActionNotification {
		return fmt.Errorf("unsupported notification sms template: %s", template)
	}
	notificationTitle := strings.TrimSpace(params[TemplateParamNotificationTitle])
	if strings.TrimSpace(notificationTitle) == "" {
		return fmt.Errorf("notification_title is required")
	}
	if len([]rune(notificationTitle)) > maxNotificationTitleRunes {
		return fmt.Errorf("notification_title must be at most %d characters", maxNotificationTitleRunes)
	}
	linkSuffix := params[TemplateParamLinkSuffix]
	if err := validateLinkSuffix(linkSuffix); err != nil {
		return err
	}
	return nil
}

func validateRequest(req Request) error {
	if NormalizePhoneNumbers(req.Phone) == "" {
		return fmt.Errorf("phone is required")
	}
	return ValidateNotificationTemplateParams(req.Template, normalizeTemplateParams(req))
}

func normalizeTemplateParams(req Request) map[string]string {
	params := make(map[string]string, len(req.TemplateParams)+1)
	for key, value := range req.TemplateParams {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			params[key] = value
		}
	}
	if value := strings.TrimSpace(req.NotificationTitle); value != "" && params[TemplateParamNotificationTitle] == "" {
		params[TemplateParamNotificationTitle] = value
	}
	return params
}

func validateLinkSuffix(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("link_suffix is required")
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(value, "//") {
		return fmt.Errorf("link_suffix must not be a full URL")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("link_suffix contains whitespace")
	}
	if !linkSuffixPattern.MatchString(value) {
		return fmt.Errorf("link_suffix contains unsupported characters")
	}
	return nil
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func hasRequiredNotificationParamsFromMap(params map[string]string) bool {
	return strings.TrimSpace(params[TemplateParamNotificationTitle]) != "" &&
		strings.TrimSpace(params[TemplateParamLinkSuffix]) != ""
}

func hasRequiredNotificationParamsFromList(params []string) bool {
	hasTitle := false
	hasLink := false
	for _, param := range params {
		switch strings.TrimSpace(param) {
		case TemplateParamNotificationTitle:
			hasTitle = true
		case TemplateParamLinkSuffix:
			hasLink = true
		}
	}
	return hasTitle && hasLink
}
