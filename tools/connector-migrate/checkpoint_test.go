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
		// These legacy-runtime migrations sit next to the Connector checkpoint;
		// the API upgrade chain writes the external-user marker itself.
		for _, id := range compatibleCheckpointMigrations {
			snapshot["migrations"] = append(snapshot["migrations"], row{"id": id})
		}
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
		if err := runOnConnection(context.Background(), db, "apply", testKeys(), plan.Report.SourceDigest, "", "", func(report) error { return nil }); err != nil || !db.tx.committed || !db.tx.readyWritten {
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

func TestAdvancedCheckpointAcceptsOnlyTheNamedHeadOfAnUnretiredDatabase(t *testing.T) {
	const head = "20261016110000_add_tool_usage_upstream_scope"
	deferred := func(t *testing.T) databaseSnapshot {
		snapshot := checkpointFixture(t, len(additiveCheckpoints))
		// A deferred run applies every later migration but never retirement.
		snapshot["migrations"] = append(snapshot["migrations"],
			row{"id": "20260904160000_connector_pending_setups"}, row{"id": head})
		return snapshot
	}
	if err := validateCheckpoint(deferred(t)); err == nil {
		t.Fatal("the default checkpoint accepted a database past the additive stages")
	}
	if err := validateAdvancedCheckpoint(deferred(t), head); err != nil {
		t.Fatalf("advanced checkpoint refused the named head: %v", err)
	}
	for name, mutate := range map[string]func(databaseSnapshot) string{
		"stale head":        func(databaseSnapshot) string { return "20261001000000_older_migration" },
		"checkpoint itself": func(databaseSnapshot) string { return detachCheckpoint },
		"malformed id":      func(databaseSnapshot) string { return "latest" },
		"retirement applied": func(snapshot databaseSnapshot) string {
			snapshot["migrations"] = append(snapshot["migrations"], row{"id": finalizeMigration})
			return head
		},
		"schema2": func(snapshot databaseSnapshot) string {
			snapshot["connector_schema_readiness"] = []row{{"schema_version": 2, "state": "ready"}}
			return head
		},
		"additive stage table without its marker": func(snapshot databaseSnapshot) string {
			snapshot["migrations"] = []row{{"id": detachCheckpoint}, {"id": head}}
			return head
		},
		"missing detach checkpoint": func(snapshot databaseSnapshot) string {
			snapshot["migrations"] = snapshot["migrations"][1:]
			return head
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := deferred(t)
			if err := validateAdvancedCheckpoint(snapshot, mutate(snapshot)); err == nil {
				t.Fatal("advanced checkpoint accepted an unsupported database")
			}
		})
	}
}

func TestAdvancedCheckpointKeepsNativeRuntimeRecordsOfADeferredDatabase(t *testing.T) {
	const head = "20261016110000_add_tool_usage_upstream_scope"
	snapshot := checkpointFixture(t, len(additiveCheckpoints))
	snapshot["migrations"] = append(snapshot["migrations"], row{"id": head})
	for _, stage := range additiveCheckpoints {
		snapshot[stage.tables[0]] = []row{{"id": "native-runtime-record"}}
	}
	if err := validateAdvancedCheckpoint(snapshot, head); err != nil {
		t.Fatalf("deferred database with native runtime records refused: %v", err)
	}
	recovery := checkpointFixture(t, len(additiveCheckpoints))
	recovery[additiveCheckpoints[0].tables[0]] = []row{{"id": "unexpected"}}
	if err := validateCheckpoint(recovery); err == nil {
		t.Fatal("historical recovery accepted a populated additive stage")
	}
}
