package service

import (
	"fmt"
	"strings"
	"time"
)

const (
	presentationVersionV2 = 2

	presentationKindText  = "text"
	presentationKindEvent = "event"

	presentationPhaseProvisional = "provisional"
	presentationPhaseProcess     = "process"
	presentationPhaseFinal       = "final"

	presentationDispositionProcess = "process"
	presentationDispositionDiscard = "discard"
	presentationRoleFinalOutput    = "final_output"
)

type presentationProjection struct {
	Version      int
	LastSequence int64
	Items        []map[string]interface{}
}

func presentationProjectionFromMetadata(metadata map[string]interface{}) presentationProjection {
	projection := presentationProjection{Version: presentationVersionV2}
	raw := mapFromOperationContext(metadata["presentation"])
	if len(raw) == 0 {
		return projection
	}
	if version, ok := int64FromPresentationValue(raw["version"]); ok && version > 0 {
		projection.Version = int(version)
	}
	if sequence, ok := int64FromPresentationValue(raw["last_sequence"]); ok && sequence > 0 {
		projection.LastSequence = sequence
	}
	for _, item := range sliceFromPresentationValue(raw["items"]) {
		mapped := mapFromOperationContext(item)
		if len(mapped) == 0 {
			continue
		}
		cloned := copyStringAnyMap(mapped)
		projection.Items = append(projection.Items, cloned)
		if sequence, ok := int64FromPresentationValue(cloned["presentation_sequence"]); ok && sequence > projection.LastSequence {
			projection.LastSequence = sequence
		}
	}
	return projection
}

func (p *presentationProjection) nextSequence() int64 {
	p.LastSequence++
	return p.LastSequence
}

func (p *presentationProjection) upsert(item map[string]interface{}) {
	if p == nil || len(item) == 0 {
		return
	}
	id := strings.TrimSpace(stringFromAny(item["presentation_id"]))
	if id != "" {
		for index := range p.Items {
			if strings.TrimSpace(stringFromAny(p.Items[index]["presentation_id"])) != id {
				continue
			}
			sequence := p.Items[index]["presentation_sequence"]
			p.Items[index] = copyStringAnyMap(item)
			if _, ok := p.Items[index]["presentation_sequence"]; !ok {
				p.Items[index]["presentation_sequence"] = sequence
			}
			return
		}
	}
	p.Items = append(p.Items, copyStringAnyMap(item))
}

func (p *presentationProjection) itemByID(id string) map[string]interface{} {
	id = strings.TrimSpace(id)
	if p == nil || id == "" {
		return nil
	}
	for _, item := range p.Items {
		if strings.TrimSpace(stringFromAny(item["presentation_id"])) == id {
			return item
		}
	}
	return nil
}

func (p *presentationProjection) removeByID(id string) {
	id = strings.TrimSpace(id)
	if p == nil || id == "" {
		return
	}
	for index := range p.Items {
		if strings.TrimSpace(stringFromAny(p.Items[index]["presentation_id"])) != id {
			continue
		}
		p.Items = append(p.Items[:index], p.Items[index+1:]...)
		return
	}
}

func (p *presentationProjection) eventItemByReference(reference string) map[string]interface{} {
	reference = strings.TrimSpace(reference)
	if p == nil || reference == "" {
		return nil
	}
	for _, item := range p.Items {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(item["kind"])), presentationKindEvent) {
			continue
		}
		if strings.TrimSpace(stringFromAny(item["event_ref"])) == reference {
			return item
		}
	}
	return nil
}

func (p presentationProjection) metadataValue() map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(p.Items))
	for _, item := range p.Items {
		// Ordinary process narration is a live presentation concern. Keep it in
		// the recorder projection so SSE events remain ordered, but do not write
		// it into message history. Final answer text and durable runtime events
		// (including explicit intermediate results) remain persisted.
		if strings.EqualFold(strings.TrimSpace(stringFromAny(item["kind"])), presentationKindText) &&
			!strings.EqualFold(strings.TrimSpace(stringFromAny(item["content_phase"])), presentationPhaseFinal) {
			continue
		}
		items = append(items, copyStringAnyMap(item))
	}
	return map[string]interface{}{
		"version":       presentationVersionV2,
		"last_sequence": p.LastSequence,
		"items":         items,
	}
}

func presentationTextID(messageID string, sequence int64) string {
	return fmt.Sprintf("message:%s:text:%d", strings.TrimSpace(messageID), sequence)
}

func presentationEventID(messageID string, eventType string, reference string, sequence int64) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		reference = fmt.Sprintf("%d", sequence)
		return fmt.Sprintf("message:%s:event:%s:%s", strings.TrimSpace(messageID), strings.TrimSpace(eventType), reference)
	}
	return fmt.Sprintf("message:%s:event:%s", strings.TrimSpace(messageID), reference)
}

func presentationEventReference(payload map[string]interface{}, invocation map[string]interface{}) string {
	for _, key := range []string{"runtime_id", "event_id", "request_id", "action_id", "approval_id", "workflow_run_id", "node_execution_id"} {
		if value := strings.TrimSpace(stringFromAny(payload[key])); value != "" {
			return value
		}
	}
	for _, key := range []string{"runtime_id", "event_id"} {
		if value := strings.TrimSpace(stringFromAny(invocation[key])); value != "" {
			return value
		}
	}
	return ""
}

func annotatePresentationPayload(payload map[string]interface{}, item map[string]interface{}) {
	if len(payload) == 0 || len(item) == 0 {
		return
	}
	payload["presentation_version"] = presentationVersionV2
	payload["presentation_id"] = item["presentation_id"]
	payload["presentation_sequence"] = item["presentation_sequence"]
	if strings.EqualFold(strings.TrimSpace(stringFromAny(item["kind"])), presentationKindText) {
		payload["segment_id"] = item["segment_id"]
		payload["content_phase"] = item["content_phase"]
	}
}

func ensurePresentationEventPosition(
	metadata map[string]interface{},
	messageID string,
	eventType string,
	payload map[string]interface{},
) map[string]interface{} {
	return ensurePresentationEventPositionWithReference(
		metadata,
		messageID,
		eventType,
		presentationEventReference(payload, nil),
		payload,
	)
}

func ensurePresentationEventPositionWithReference(
	metadata map[string]interface{},
	messageID string,
	eventType string,
	reference string,
	payload map[string]interface{},
) map[string]interface{} {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	projection := presentationProjectionFromMetadata(metadata)
	reference = strings.TrimSpace(reference)
	if existing := projection.eventItemByReference(reference); len(existing) > 0 {
		annotatePresentationPayload(payload, existing)
		metadata["presentation"] = projection.metadataValue()
		metadata["presentation_version"] = presentationVersionV2
		return metadata
	}

	sequence := projection.nextSequence()
	item := map[string]interface{}{
		"presentation_id":       presentationEventID(messageID, eventType, reference, sequence),
		"presentation_sequence": sequence,
		"kind":                  presentationKindEvent,
		"event_type":            eventType,
		"event_ref":             reference,
		"created_at_ms":         time.Now().UnixMilli(),
	}
	projection.upsert(item)
	annotatePresentationPayload(payload, item)
	metadata["presentation"] = projection.metadataValue()
	metadata["presentation_version"] = presentationVersionV2
	return metadata
}

func newPresentationTextItem(messageID string, sequence int64, content string, now time.Time) map[string]interface{} {
	id := presentationTextID(messageID, sequence)
	return map[string]interface{}{
		"presentation_id":       id,
		"presentation_sequence": sequence,
		"kind":                  presentationKindText,
		"segment_id":            id,
		"content_phase":         presentationPhaseProvisional,
		"content":               content,
		"created_at_ms":         now.UnixMilli(),
	}
}

func sliceFromPresentationValue(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func int64FromPresentationValue(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	default:
		return 0, false
	}
}
