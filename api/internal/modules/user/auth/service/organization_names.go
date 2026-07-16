package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
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

func uniqueOwnedOrganizationName(ctx context.Context, organizationService interfaces.OrganizationManagementService, accountName string, language *string) (string, error) {
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
