package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const maxOwnedOrganizationNameAttempts = 100

func ownedOrganizationName(accountName string, language *string) string {
	name := strings.TrimSpace(accountName)
	if name == "" {
		name = "User"
	}
	if language != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(*language)), "zh") {
		return fmt.Sprintf("%s 的组织", name)
	}
	return fmt.Sprintf("%s's Organization", name)
}

type organizationNameAvailabilityChecker interface {
	CheckOrganizationNameExists(ctx context.Context, name string) (bool, error)
}

func uniqueOwnedOrganizationName(ctx context.Context, organizationService organizationNameAvailabilityChecker, accountName string, language *string) (string, error) {
	baseName := ownedOrganizationName(accountName, language)
	for attempt := 0; attempt < maxOwnedOrganizationNameAttempts; attempt++ {
		candidate := baseName
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%s", baseName, uuid.New().String()[:5])
		}

		exists, err := organizationService.CheckOrganizationNameExists(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("check organization name exists: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique organization name")
}
