package service

import (
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/routing"
	"github.com/zgiai/ginext/internal/contracts"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
)

type DatasetParseRunBuildInput struct {
	WorkspaceID    *uuid.UUID
	DatasetID      *uuid.UUID
	DocumentID     *uuid.UUID
	FileID         *uuid.UUID
	ArtifactID     *uuid.UUID
	Request        contracts.ParseRequest
	RoutePlan      *routing.RoutePlan
	Artifact       *contracts.ParseArtifact
	Summary        map[string]interface{}
	OrganizationID string
}

func BuildDatasetParseRun(input DatasetParseRunBuildInput) *model.ParseRun {
	summary := cloneStringAnyMap(input.Summary)
	if summary == nil {
		summary = map[string]interface{}{}
	}
	if input.OrganizationID != "" {
		summary["organization_id"] = input.OrganizationID
	}
	return &model.ParseRun{
		WorkspaceID:            input.WorkspaceID,
		DatasetID:              input.DatasetID,
		DocumentID:             input.DocumentID,
		FileID:                 input.FileID,
		ArtifactID:             input.ArtifactID,
		SourceType:             string(input.Request.SourceType),
		SourceRef:              input.Request.SourceRef,
		FileName:               input.Request.FileName,
		Intent:                 string(input.Request.Intent),
		Profile:                string(input.Request.Profile),
		PolicyKey:              RoutePolicyKey(input.RoutePlan),
		RequestedProviderKey:   RequestedProviderKey(input.Request),
		PlannedProviderOrder:   PlannedProviderOrder(input.RoutePlan),
		AttemptedProviderOrder: AttemptedProviderOrder(input.RoutePlan, input.Artifact),
		FinalProviderKey:       FinalProviderKey(input.RoutePlan, input.Artifact),
		AdapterName:            FinalAdapterName(input.RoutePlan, input.Artifact),
		EngineName:             string(FinalEngineName(input.RoutePlan, input.Artifact)),
		Status:                 StatusString(summary, input.Artifact),
		QualityLevel:           QualityLevelString(summary, input.Artifact),
		FallbackUsed:           FallbackUsedBool(summary, input.Artifact),
		DurationMS:             OptionalInt(summary["duration_ms"]),
		ArtifactStorageKey:     "",
		DiagnosticsStorageKey:  "",
		SummaryJSON:            summary,
	}
}

func RoutePolicyKey(plan *routing.RoutePlan) string {
	if plan == nil {
		return ""
	}
	return string(plan.Mode)
}

func RequestedProviderKey(req contracts.ParseRequest) string {
	if req.EngineHint == "" {
		return ""
	}
	return string(req.EngineHint)
}

func StatusString(summary map[string]interface{}, artifact *contracts.ParseArtifact) string {
	if artifact != nil && artifact.Status != "" {
		return string(artifact.Status)
	}
	return ReadStringMap(summary, "status")
}

func QualityLevelString(summary map[string]interface{}, artifact *contracts.ParseArtifact) string {
	if artifact != nil && artifact.QualityLevel != "" {
		return string(artifact.QualityLevel)
	}
	return ReadStringMap(summary, "quality_level")
}

func FallbackUsedBool(summary map[string]interface{}, artifact *contracts.ParseArtifact) bool {
	if artifact != nil {
		return artifact.FallbackUsed
	}
	value, _ := summary["fallback_used"].(bool)
	return value
}

func ReadStringMap(summary map[string]interface{}, key string) string {
	if len(summary) == 0 {
		return ""
	}
	value, ok := summary[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func OptionalInt(value interface{}) *int {
	switch typed := value.(type) {
	case int:
		return &typed
	case int32:
		converted := int(typed)
		return &converted
	case int64:
		converted := int(typed)
		return &converted
	case float64:
		converted := int(typed)
		return &converted
	default:
		return nil
	}
}
