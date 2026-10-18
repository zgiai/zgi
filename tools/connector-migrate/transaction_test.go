package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// This fixture tests the real CLI transaction orchestration. PostgreSQL SQL,
// constraints and rollback durability still require the isolated database gate.
type commandDatabase struct {
	tx      *commandTransaction
	options pgx.TxOptions
}

func (d *commandDatabase) BeginTx(_ context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	d.options = options
	return d.tx, nil
}

type commandTransaction struct {
	pgx.Tx
	source, converted                              databaseSnapshot
	reads, writes, failWrite                       int
	commitError, invalidConversion                 bool
	committed, rolledBack, validated, readyWritten bool
	maintenanceBusy, maintenanceLocked             bool
	timezoneConfigured, timezoneError              bool
}

func (tx *commandTransaction) current() databaseSnapshot {
	if tx.reads >= 2 && !tx.invalidConversion {
		return tx.converted
	}
	return tx.source
}

type existsRow bool

func (r existsRow) Scan(dest ...any) error { *dest[0].(*bool) = bool(r); return nil }

func (tx *commandTransaction) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	if query == "SELECT pg_try_advisory_xact_lock(hashtext('zgi:migrations'))" {
		tx.maintenanceLocked = !tx.maintenanceBusy
		return existsRow(tx.maintenanceLocked)
	}
	if query != "SELECT to_regclass($1) IS NOT NULL" {
		panic("unexpected command query")
	}
	name := strings.TrimPrefix(args[0].(string), "public.")
	_, exists := tx.current()[name]
	return existsRow(exists)
}

type snapshotRows struct {
	pgx.Rows
	values []row
	index  int
}

func (r *snapshotRows) Next() bool { r.index++; return r.index < len(r.values) }
func (r *snapshotRows) Close()     {}
func (r *snapshotRows) Err() error { return nil }
func (r *snapshotRows) Scan(dest ...any) error {
	data, err := json.Marshal(r.values[r.index])
	*dest[0].(*[]byte) = data
	return err
}

func (tx *commandTransaction) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	if !tx.timezoneConfigured {
		return nil, errors.New("snapshot read without canonical timezone")
	}
	parts := strings.Split(query, `"`)
	if len(parts) < 4 {
		return nil, errors.New("unexpected snapshot query")
	}
	name := parts[3]
	// The first fixture table identifies each complete loadSnapshot pass.
	if name == "agent_resource_bindings" {
		tx.reads++
	}
	return &snapshotRows{values: tx.current()[name], index: -1}, nil
}

func (tx *commandTransaction) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	switch {
	case query == "SET LOCAL TIME ZONE 'UTC'":
		if tx.timezoneError {
			return pgconn.CommandTag{}, errors.New(secretMarker)
		}
		tx.timezoneConfigured = true
	case strings.HasPrefix(query, "ALTER TABLE connector_confirmation_continuations"):
		tx.validated = true
	case strings.HasPrefix(query, "UPDATE connector_schema_readiness"):
		if !tx.validated {
			return pgconn.CommandTag{}, errors.New("readiness before ownership validation")
		}
		tx.readyWritten = true
	case strings.HasPrefix(query, "INSERT "), strings.HasPrefix(query, "UPDATE "):
		if !tx.maintenanceLocked {
			return pgconn.CommandTag{}, errors.New("write without migration lock")
		}
		tx.writes++
		if tx.writes == tx.failWrite {
			return pgconn.CommandTag{}, errors.New(secretMarker)
		}
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (tx *commandTransaction) Commit(context.Context) error {
	if tx.commitError {
		return errors.New(secretMarker)
	}
	tx.committed = true
	return nil
}

func (tx *commandTransaction) Rollback(context.Context) error {
	if !tx.committed {
		tx.rolledBack = true
	}
	return nil
}

func commandFixture(t *testing.T) (*commandDatabase, string) {
	t.Helper()
	source := fixture(t)
	source["migrations"] = []row{{"id": detachCheckpoint}}
	if err := inventorySkillFiles(source, ""); err != nil {
		t.Fatal(err)
	}
	plan, err := makePlan(source, testKeys())
	if err != nil || len(plan.Report.Issues) != 0 {
		t.Fatal("invalid command fixture", err)
	}
	tx := &commandTransaction{source: source, converted: applyMemory(t, source, plan)}
	return &commandDatabase{tx: tx}, plan.Report.SourceDigest
}

func TestConversionCommandsKeepReadOnlyRunsAndFailedWritesUncommitted(t *testing.T) {
	for _, command := range []string{"inventory", "dry-run", "verify"} {
		t.Run(command, func(t *testing.T) {
			db, digest := commandFixture(t)
			before := hash(db.tx.source)
			if command == "verify" {
				db.tx.source = db.tx.converted
				before = hash(db.tx.source)
			}
			err := runOnConnection(context.Background(), db, command, testKeys(), digest, "", "", func(report) error { return nil })
			if err != nil || db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.Serializable || db.tx.writes != 0 || db.tx.committed || !db.tx.rolledBack || hash(db.tx.source) != before {
				t.Fatal("read-only command mutated or committed", err)
			}
		})
	}
	for _, scenario := range []string{"timezone_failure", "write_failure", "verification_failure", "commit_failure", "stale_report", "changed_history", "migration_busy", "unsupported_history", "success"} {
		t.Run(scenario, func(t *testing.T) {
			db, digest := commandFixture(t)
			switch scenario {
			case "timezone_failure":
				db.tx.timezoneError = true
			case "write_failure":
				db.tx.failWrite = 2
			case "verification_failure":
				db.tx.invalidConversion = true
			case "commit_failure":
				db.tx.commitError = true
			case "stale_report":
				digest = strings.Repeat("0", 64)
			case "changed_history":
				db.tx.source["migrations"] = append(db.tx.source["migrations"], row{"id": "20260903183000_add_video_runtime_task_poster_url"})
			case "migration_busy":
				db.tx.maintenanceBusy = true
			case "unsupported_history":
				db.tx.source["migrations"] = append(db.tx.source["migrations"], row{"id": "202609031410353961_finalize_connector_detachment"})
			}
			var output report
			err := runOnConnection(context.Background(), db, "apply", testKeys(), digest, "", "", func(r report) error { output = r; return nil })
			if db.options.IsoLevel != pgx.Serializable || db.options.AccessMode == pgx.ReadOnly {
				t.Fatal("apply did not use a writable serializable transaction")
			}
			if scenario == "success" {
				if err != nil || !db.tx.committed || !db.tx.validated || !db.tx.readyWritten || output.State != "applied" {
					t.Fatal("successful conversion was not verified and committed", err)
				}
				return
			}
			if err == nil || strings.Contains(err.Error(), secretMarker) || db.tx.committed || !db.tx.rolledBack || output.State == "applied" {
				t.Fatal("failed conversion committed, reported success or exposed driver data", err)
			}
			if (scenario == "timezone_failure" || scenario == "stale_report" || scenario == "changed_history" || scenario == "migration_busy" || scenario == "unsupported_history") && db.tx.writes != 0 {
				t.Fatal("changed source was written")
			}
			if scenario == "write_failure" && db.tx.writes != 2 {
				t.Fatal("write failure did not stop the transaction immediately")
			}
			if scenario != "commit_failure" && db.tx.readyWritten {
				t.Fatal("failed conversion was marked ready")
			}
		})
	}
}
