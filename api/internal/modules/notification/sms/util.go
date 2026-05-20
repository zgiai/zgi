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

func ValidateNotificationContent(template, notificationTitle, linkCode string) error {
	if strings.TrimSpace(template) == "" {
		template = TemplatePendingActionNotification
	}
	if template != TemplatePendingActionNotification {
		return fmt.Errorf("unsupported notification sms template: %s", template)
	}
	if strings.TrimSpace(notificationTitle) == "" {
		return fmt.Errorf("notification_title is required")
	}
	if len([]rune(notificationTitle)) > maxNotificationTitleRunes {
		return fmt.Errorf("notification_title must be at most %d characters", maxNotificationTitleRunes)
	}
	linkCode = strings.TrimSpace(linkCode)
	if linkCode == "" {
		return fmt.Errorf("link_code is required")
	}
	if !linkCodePattern.MatchString(linkCode) {
		return fmt.Errorf("link_code contains unsupported characters")
	}
	return nil
}

func validateRequest(req Request) error {
	if NormalizePhoneNumbers(req.Phone) == "" {
		return fmt.Errorf("phone is required")
	}
	return ValidateNotificationContent(req.Template, req.NotificationTitle, req.LinkCode)
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
