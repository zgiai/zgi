package main

import (
	"context"
	"testing"
)

func checkpointFixture(t *testing.T, stages int) databaseSnapshot {
	t.Helper()
	snapshot := fixture(t)
	snapshot["migrations"] = []row{{"id": detachCheckpoint}}
	for _, stage := range additiveCheckpoints[:stages] {
		snapshot["migrations"] = append(snapshot["migrations"], row{"id": stage.id})
		for _, table := range stage.tables {
			snapshot[table] = []row{}
		}
	}
	return snapshot
}

func TestHistoricalCheckpointRecovery(t *testing.T) {
	for stages := 0; stages <= len(additiveCheckpoints); stages++ {
		snapshot := checkpointFixture(t, stages)
		// This legacy-runtime migration is newer than the Connector checkpoint.
		snapshot["migrations"] = append(snapshot["migrations"], row{"id": "20260903183000_add_video_runtime_task_poster_url"})
		if err := validateCheckpoint(snapshot); err != nil {
			t.Fatal(stages, err)
		}
		if err := inventorySkillFiles(snapshot, ""); err != nil {
			t.Fatal(err)
		}
		plan, err := makePlan(snapshot, testKeys())
		if err != nil || len(plan.Report.Issues) != 0 {
			t.Fatal("invalid recovery fixture", err)
		}
		db := &commandDatabase{tx: &commandTransaction{source: snapshot, converted: applyMemory(t, snapshot, plan)}}
		if err := runOnConnection(context.Background(), db, "apply", testKeys(), plan.Report.SourceDigest, "", func(report) error { return nil }); err != nil || !db.tx.committed || !db.tx.readyWritten {
			t.Fatal("recovery was not verified and committed", stages, err)
		}
		for _, mutation := range plan.Mutations {
			if mutation.Table == "migrations" {
				t.Fatal("converter rewrites migration history")
			}
			for _, stage := range additiveCheckpoints {
				for _, table := range stage.tables {
					if mutation.Table == table {
						t.Fatal("converter rewrites additive native state")
					}
				}
			}
		}
	}
}

func TestHistoricalCheckpointFailsClosed(t *testing.T) {
	for _, scenario := range []string{"missing_detach", "missing_history", "gap", "retired", "unknown", "schema2", "invalid_state", "missing_table", "unmarked_table", "native_rows"} {
		t.Run(scenario, func(t *testing.T) {
			snapshot := checkpointFixture(t, len(additiveCheckpoints))
			switch scenario {
			case "missing_detach":
				snapshot["migrations"] = snapshot["migrations"][1:]
			case "missing_history":
				delete(snapshot, "migrations")
			case "gap":
				snapshot["migrations"] = append(snapshot["migrations"][:1], snapshot["migrations"][2:]...)
			case "retired":
				snapshot["migrations"] = append(snapshot["migrations"], row{"id": "202609031410353961_finalize_connector_detachment"})
			case "unknown":
				snapshot["migrations"] = append(snapshot["migrations"], row{"id": "20260905120000_unknown"})
			case "schema2":
				snapshot["connector_schema_readiness"][0]["schema_version"] = 2
			case "invalid_state":
				snapshot["connector_schema_readiness"][0]["state"] = "unknown"
			case "missing_table":
				delete(snapshot, "connector_messaging_states")
			case "unmarked_table":
				snapshot["migrations"] = snapshot["migrations"][:3]
			case "native_rows":
				snapshot["connector_messaging_states"] = []row{{"id": "not-converter-owned"}}
			}
			if err := validateCheckpoint(snapshot); err == nil {
				t.Fatal("unsafe checkpoint accepted")
			}
		})
	}
}
