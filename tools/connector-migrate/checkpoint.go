package main

import "errors"

const detachCheckpoint = "20260902190000_detach_connector_runtime"

// These additive stages can precede a failed normal up on a historical database.
// The converter does not understand their runtime records: recovery is supported
// only while the new tables are empty. Do not infer compatibility from a prefix.
var additiveCheckpoints = []struct {
	id     string
	tables []string
}{
	{"20260903100000_create_connector_chat_preferences", []string{"connector_chat_preferences", "connector_chat_preference_mutations"}},
	{"20260903110000_create_connector_onboarding", []string{"connector_onboarding_flows", "connector_onboarding_secrets", "connector_connection_provider_evidence"}},
	{"20260903120000_messaging_connector_protocol", []string{"connector_messaging_states", "connector_messaging_contexts"}},
}

func validateCheckpoint(snapshot databaseSnapshot) error {
	ready := snapshot["connector_schema_readiness"]
	if len(ready) != 1 || integer(ready[0], "schema_version") != 1 ||
		(text(ready[0], "state") != "conversion_required" && text(ready[0], "state") != "ready") {
		return errors.New("historical converter requires schema1 conversion_required or ready; use connector-finalize after retirement")
	}
	allowed := map[string]bool{detachCheckpoint: true, "20260903183000_add_video_runtime_task_poster_url": true}
	for _, stage := range additiveCheckpoints {
		allowed[stage.id] = true
	}
	applied := map[string]bool{}
	for _, entry := range snapshot["migrations"] {
		id := text(entry, "id")
		if id == "" || applied[id] || (id > detachCheckpoint && !allowed[id]) {
			return errors.New("unsupported migration history for historical conversion; no data was changed")
		}
		applied[id] = true
	}
	if !applied[detachCheckpoint] {
		return errors.New("historical converter requires the detach Connector checkpoint")
	}
	previous := detachCheckpoint
	for _, stage := range additiveCheckpoints {
		if applied[stage.id] && !applied[previous] {
			return errors.New("incomplete pre-retirement Connector migration history")
		}
		for _, table := range stage.tables {
			rows, exists := snapshot[table]
			if exists != applied[stage.id] {
				return errors.New("pre-retirement Connector tables and migration markers disagree")
			}
			if len(rows) != 0 {
				return errors.New("historical recovery requires empty additive Connector tables; stop writers and review native state")
			}
		}
		previous = stage.id
	}
	return nil
}
