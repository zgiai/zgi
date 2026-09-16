package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

const platformLifecycleSubjectType = "connector_plugin"

type pluginVersionEvidence struct {
	PluginVersion string `gorm:"column:plugin_version"`
	PluginDigest  string `gorm:"column:plugin_digest"`
}

func recordPluginAdmissionAudit(
	ctx context.Context,
	tx *gorm.DB,
	pluginID string,
	ordered []Definition,
	current map[string]Definition,
	now time.Time,
) error {
	actor, ok := connectoraudit.PlatformActorFromContext(ctx)
	if !ok || tx == nil || len(ordered) == 0 {
		return fmt.Errorf("record Connector plugin admission audit: %w", ErrPluginAdmissionUnavailable)
	}
	first, err := definitionAdmissionRecord(ordered[0], now)
	if err != nil {
		return err
	}
	decision, fromStatus := "plugin_admitted", "not_admitted"
	if len(current) > 0 {
		decision, fromStatus = "plugin_updated", "active"
	}
	connectorIDs := make([]string, 0, len(ordered))
	contractDigests := make([]string, 0, len(ordered))
	for _, definition := range ordered {
		connectorIDs = append(connectorIDs, definition.ID)
		contractDigest, digestErr := DefinitionContractDigest(definition)
		if digestErr != nil {
			return digestErr
		}
		contractDigests = append(contractDigests, contractDigest)
	}
	correlation := pluginLifecycleCorrelation(
		pluginID, first.PluginVersion, first.PluginDigest, decision, "active", false,
	)
	event := connectoraudit.PlatformLifecycleEvent{
		ID:          uuid.NewSHA1(uuid.NameSpaceOID, []byte(decision+"\x00"+correlation)),
		SubjectType: platformLifecycleSubjectType, SubjectID: pluginID, Actor: actor,
		Decision: decision, Status: connectorcore.DecisionStatusSuccessful, CorrelationID: correlation,
		Details: map[string]any{
			"from_status": fromStatus, "to_status": "active", "forced": false,
			"plugin_version": first.PluginVersion, "plugin_digest": first.PluginDigest,
			"connector_ids": connectorIDs, "contract_digests": contractDigests,
		},
		CreatedAt: now.UTC(),
	}
	if err := connectoraudit.NewGormRepository(tx).CreatePlatformEvent(ctx, event); err != nil {
		return fmt.Errorf("record Connector plugin admission audit: %w", err)
	}
	return nil
}

func recordPluginTransitionAudit(
	ctx context.Context,
	tx *gorm.DB,
	pluginID string,
	fromStatus, toStatus PluginDefinitionStatus,
	force bool,
	impact PluginImpact,
	version pluginVersionEvidence,
	now time.Time,
) error {
	actor, ok := connectoraudit.PlatformActorFromContext(ctx)
	if !ok || tx == nil || version.PluginVersion == "" || !sha256HexPattern.MatchString(version.PluginDigest) {
		return fmt.Errorf("record Connector plugin transition audit: %w", ErrPluginAdmissionUnavailable)
	}
	decision := "plugin_disabled"
	if toStatus == PluginDefinitionStatusUnavailable {
		decision = "plugin_removed"
	}
	connectorIDs := make([]string, 0, len(impact.Definitions))
	for _, definition := range impact.Definitions {
		connectorIDs = append(connectorIDs, definition.ConnectorID)
	}
	sort.Strings(connectorIDs)
	correlation := pluginLifecycleCorrelation(
		pluginID, version.PluginVersion, version.PluginDigest, decision, string(toStatus), force,
	)
	event := connectoraudit.PlatformLifecycleEvent{
		ID:          uuid.NewSHA1(uuid.NameSpaceOID, []byte(decision+"\x00"+correlation)),
		SubjectType: platformLifecycleSubjectType, SubjectID: pluginID, Actor: actor,
		Decision: decision, Status: connectorcore.DecisionStatusSuccessful, CorrelationID: correlation,
		Details: map[string]any{
			"from_status": string(fromStatus), "to_status": string(toStatus), "forced": force,
			"plugin_version": version.PluginVersion, "plugin_digest": version.PluginDigest,
			"connector_ids": connectorIDs, "connection_count": len(impact.Connections),
			"binding_count": len(impact.Bindings),
		},
		CreatedAt: now.UTC(),
	}
	if err := connectoraudit.NewGormRepository(tx).CreatePlatformEvent(ctx, event); err != nil {
		return fmt.Errorf("record Connector plugin transition audit: %w", err)
	}
	return nil
}

func pluginLifecycleCorrelation(
	pluginID, pluginVersion, pluginDigest, decision, toStatus string,
	force bool,
) string {
	encoded, _ := json.Marshal([]any{
		pluginID, pluginVersion, strings.ToLower(pluginDigest), decision, toStatus, force,
	})
	digest := sha256.Sum256(encoded)
	return "plugin-lifecycle-" + hex.EncodeToString(digest[:])
}

func sameStoredPluginPackage(current map[string]Definition, candidates map[string]Definition) bool {
	if len(current) == 0 || len(current) != len(candidates) {
		return false
	}
	for connectorID, existing := range current {
		candidate, ok := candidates[connectorID]
		if !ok {
			return false
		}
		left, leftErr := json.Marshal(existing)
		right, rightErr := json.Marshal(candidate)
		if leftErr != nil || rightErr != nil || string(left) != string(right) {
			return false
		}
	}
	return true
}

func pluginImpactStatus(impact PluginImpact) (PluginDefinitionStatus, bool) {
	if len(impact.Definitions) == 0 {
		return "", false
	}
	status := impact.Definitions[0].Status
	for _, definition := range impact.Definitions[1:] {
		if definition.Status != status {
			return "", false
		}
	}
	return status, true
}
