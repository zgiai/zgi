package main

import (
	"errors"
	"regexp"
)

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

// finalizeMigration is the API-owned retirement migration; the converter runs
// only while it is absent.
const finalizeMigration = "202609031410353961_finalize_connector_detachment"

var migrationIDShape = regexp.MustCompile(`^\d{14}(\d{2}|\d{4})?_[a-z0-9_]+$`)

// validateCheckpoint accepts the detach checkpoint and the supported empty
// additive stages (a failed normal up on a historical database).
func validateCheckpoint(snapshot databaseSnapshot) error {
	if err := requireSchema1(snapshot); err != nil {
		return err
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
	return requireAdditiveStages(snapshot, applied)
}

// validateAdvancedCheckpoint accepts a schema1 database whose migrations ran
// past the detach checkpoint without retiring legacy storage, as a database
// running with Connector retirement deferred does. The operator names the
// exact applied head, so a reviewed dry-run cannot silently cover a different
// history; the ledger is part of the source digest that apply re-checks.
func validateAdvancedCheckpoint(snapshot databaseSnapshot, head string) error {
	if err := requireSchema1(snapshot); err != nil {
		return err
	}
	if !migrationIDShape.MatchString(head) || head <= detachCheckpoint {
		return errors.New("--advanced-checkpoint must name an applied migration later than " + detachCheckpoint)
	}
	applied := map[string]bool{}
	latest := ""
	for _, entry := range snapshot["migrations"] {
		id := text(entry, "id")
		if id == "" || applied[id] {
			return errors.New("unsupported migration history for historical conversion; no data was changed")
		}
		applied[id] = true
		if id > latest {
			latest = id
		}
	}
	if applied[finalizeMigration] {
		return errors.New("connector retirement is already applied; the historical converter cannot run")
	}
	if latest != head {
		return errors.New("--advanced-checkpoint does not match the applied migration head; no data was changed")
	}
	return requireAdditiveStages(snapshot, applied)
}

func requireSchema1(snapshot databaseSnapshot) error {
	ready := snapshot["connector_schema_readiness"]
	if len(ready) != 1 || integer(ready[0], "schema_version") != 1 ||
		(text(ready[0], "state") != "conversion_required" && text(ready[0], "state") != "ready") {
		return errors.New("historical converter requires schema1 conversion_required or ready; use connector-finalize after retirement")
	}
	return nil
}

// requireAdditiveStages checks the additive Connector stages after the detach
// checkpoint: applied in order, their tables present exactly when applied, and
// empty, because the converter does not understand their runtime records.
func requireAdditiveStages(snapshot databaseSnapshot, applied map[string]bool) error {
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
