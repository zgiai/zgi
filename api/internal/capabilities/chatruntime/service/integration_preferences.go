package service

import (
	"context"
	"fmt"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

const (
	maxAIChatIntegrationPreferences       = 32
	maxAIChatSelectedConnectionsPerApp    = 20
	maxAIChatIntegrationPreferenceIDRunes = 128
)

// refreshAIChatIntegrationRunConfig replaces (rather than merges) connection
// IDs whenever an authoritative resolver is configured. This makes deletion or
// revocation effective on regenerate and continuation, and prevents stale or
// request-supplied IDs from expanding the user's current selection.
func (s *service) refreshAIChatIntegrationRunConfig(ctx context.Context, scope Scope, caller Caller, config RunConfig) (RunConfig, error) {
	if s == nil || s.integrationPrefs == nil || normalizeCallerType(caller.Type) != runtimemodel.ConversationCallerAIChat {
		return config, nil
	}
	preferences, err := s.integrationPrefs.ResolveAIChatIntegrationPreferences(ctx, scope)
	if err != nil {
		return config, fmt.Errorf("resolve current AIChat integration preferences: %w", err)
	}
	selected, preferred, err := normalizeAIChatIntegrationRuntimePreferences(preferences)
	if err != nil {
		return config, fmt.Errorf("resolve current AIChat integration preferences: %w", err)
	}
	config.IntegrationSelectedConnectionIDs = selected
	config.IntegrationConnectionIDs = preferred
	return config, nil
}

func normalizeAIChatIntegrationRuntimePreferences(preferences AIChatIntegrationRuntimePreferences) (map[string][]string, map[string]string, error) {
	if len(preferences.SelectedConnectionIDs) > maxAIChatIntegrationPreferences || len(preferences.PreferredConnectionIDs) > maxAIChatIntegrationPreferences {
		return nil, nil, fmt.Errorf("integration preference count exceeds %d", maxAIChatIntegrationPreferences)
	}

	selected := make(map[string][]string, len(preferences.SelectedConnectionIDs))
	selectedSets := make(map[string]map[string]struct{}, len(preferences.SelectedConnectionIDs))
	seenIntegrations := make(map[string]struct{}, len(preferences.SelectedConnectionIDs))
	for rawIntegrationID, rawConnectionIDs := range preferences.SelectedConnectionIDs {
		integrationID := strings.ToLower(strings.TrimSpace(rawIntegrationID))
		if integrationID == "" || len([]rune(integrationID)) > maxAIChatIntegrationPreferenceIDRunes {
			return nil, nil, fmt.Errorf("integration preference contains an invalid integration id")
		}
		if _, duplicate := seenIntegrations[integrationID]; duplicate {
			return nil, nil, fmt.Errorf("integration preference contains duplicate integration id %q", integrationID)
		}
		seenIntegrations[integrationID] = struct{}{}
		if len(rawConnectionIDs) > maxAIChatSelectedConnectionsPerApp {
			return nil, nil, fmt.Errorf("integration %q selects more than %d connections", integrationID, maxAIChatSelectedConnectionsPerApp)
		}
		connectionIDs := make([]string, 0, len(rawConnectionIDs))
		connectionSet := make(map[string]struct{}, len(rawConnectionIDs))
		for _, rawConnectionID := range rawConnectionIDs {
			connectionID := strings.TrimSpace(rawConnectionID)
			if connectionID == "" || len([]rune(connectionID)) > maxAIChatIntegrationPreferenceIDRunes {
				return nil, nil, fmt.Errorf("integration %q contains an invalid connection id", integrationID)
			}
			key := strings.ToLower(connectionID)
			if _, duplicate := connectionSet[key]; duplicate {
				continue
			}
			connectionSet[key] = struct{}{}
			connectionIDs = append(connectionIDs, connectionID)
		}
		if len(connectionIDs) == 0 {
			continue
		}
		selected[integrationID] = connectionIDs
		selectedSets[integrationID] = connectionSet
	}

	preferred := make(map[string]string, len(preferences.PreferredConnectionIDs))
	for rawIntegrationID, rawConnectionID := range preferences.PreferredConnectionIDs {
		integrationID := strings.ToLower(strings.TrimSpace(rawIntegrationID))
		connectionID := strings.TrimSpace(rawConnectionID)
		if integrationID == "" || connectionID == "" {
			return nil, nil, fmt.Errorf("preferred integration connection is invalid")
		}
		connectionSet, exists := selectedSets[integrationID]
		if !exists {
			return nil, nil, fmt.Errorf("preferred connection for integration %q is not selected", integrationID)
		}
		if _, exists := connectionSet[strings.ToLower(connectionID)]; !exists {
			return nil, nil, fmt.Errorf("preferred connection for integration %q is not selected", integrationID)
		}
		if _, duplicate := preferred[integrationID]; duplicate {
			return nil, nil, fmt.Errorf("integration preference contains duplicate preferred integration id %q", integrationID)
		}
		preferred[integrationID] = connectionID
	}

	if len(selected) == 0 {
		selected = nil
	}
	if len(preferred) == 0 {
		preferred = nil
	}
	return selected, preferred, nil
}
