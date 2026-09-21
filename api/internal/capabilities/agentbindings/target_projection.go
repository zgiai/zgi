package agentbindings

import (
	"encoding/json"
	"github.com/google/uuid"
	"sort"
	"strings"
	"time"
)

// ReconcileTargetProjectionBindings preserves identity for unchanged ordinary resource selections.
func ReconcileTargetProjectionBindings(existing, desired []Binding) []Binding {
	byKey := make(map[string][]Binding, len(existing))
	for _, binding := range existing {
		key := targetProjectionLogicalKey(binding)
		byKey[key] = append(byKey[key], binding)
	}
	for key := range byKey {
		sort.SliceStable(byKey[key], func(i, j int) bool {
			return byKey[key][i].ID.String() < byKey[key][j].ID.String()
		})
	}

	result := make([]Binding, 0, len(desired))
	for _, binding := range desired {
		key := targetProjectionLogicalKey(binding)
		candidates := byKey[key]
		if len(candidates) == 0 {
			result = append(result, binding)
			continue
		}
		prior := candidates[0]
		byKey[key] = candidates[1:]

		binding.ID = prior.ID
		binding.CreatedAt = prior.CreatedAt
		result = append(result, binding)
	}
	return result
}

func targetProjectionLogicalKey(binding Binding) string {
	return strings.Join([]string{binding.AgentID.String(), string(binding.ConsumerType), binding.ConsumerID.String(), string(binding.BindingScope), uuidValue(binding.PublishedVersionUUID), string(binding.BindingType), strings.TrimSpace(binding.BindingInstanceID), strings.TrimSpace(binding.ResourceID), strings.TrimSpace(binding.ParentResourceID), strings.ToLower(strings.TrimSpace(binding.AccessMode))}, "\x00")
}
func uuidValue(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBinding(binding Binding) Binding {
	cloned := binding
	cloned.PublishedVersionUUID = cloneUUID(binding.PublishedVersionUUID)
	cloned.AuthorizedBy = cloneUUID(binding.AuthorizedBy)
	cloned.AuthorizedAt = cloneTime(binding.AuthorizedAt)
	if binding.Metadata != nil {
		encoded, err := json.Marshal(binding.Metadata)
		if err == nil {
			var metadata map[string]interface{}
			if json.Unmarshal(encoded, &metadata) == nil {
				cloned.Metadata = metadata
			}
		}
	}
	return cloned
}
