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
	migrations                                     []string
	retiredTables, bindingColumns                  []string
	constraintNames                                map[string][]string
	events                                         []string
	reads, writes, failWrite                       int
	commitError, invalidConversion, failLock       bool
	committed, rolledBack, validated, readyWritten bool
}

func (tx *commandTransaction) current() databaseSnapshot {
	if tx.reads >= 2 && !tx.invalidConversion {
		return tx.converted
	}
	return tx.source
}

type existsRow bool

func (r existsRow) Scan(dest ...any) error { *dest[0].(*bool) = bool(r); return nil }

type stringListRow []string

func (r stringListRow) Scan(dest ...any) error {
	*dest[0].(*[]string) = append([]string(nil), r...)
	return nil
}

func (tx *commandTransaction) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	switch query {
	case "SELECT to_regclass($1) IS NOT NULL":
		tx.events = append(tx.events, "select:to_regclass")
		name := strings.TrimPrefix(args[0].(string), "public.")
		_, exists := tx.current()[name]
		return existsRow(exists)
	case migrationHistoryQuery:
		tx.events = append(tx.events, "select:migration-ledger")
		return stringListRow(tx.migrations)
	case advancedTableQuery:
		tx.events = append(tx.events, "select:catalog")
		return stringListRow(tx.retiredTables)
	case advancedColumnQuery:
		tx.events = append(tx.events, "select:catalog")
		return stringListRow(tx.bindingColumns)
	case advancedConstraintQuery:
		tx.events = append(tx.events, "select:catalog")
		return stringListRow(tx.constraintNames[args[0].(string)])
	}
	panic("unexpected command query")
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
	tx.events = append(tx.events, "select:snapshot")
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
	case query == migrationLedgerLock:
		tx.events = append(tx.events, "lock:migration-ledger")
		if tx.failLock {
			return pgconn.CommandTag{}, errors.New(secretMarker)
		}
	case strings.HasPrefix(query, "SELECT pg_advisory_xact_lock"):
		tx.events = append(tx.events, "lock:advisory")
	case strings.HasPrefix(query, "ALTER TABLE connector_confirmation_continuations"):
		tx.events = append(tx.events, "write:validate")
		tx.validated = true
	case strings.HasPrefix(query, "UPDATE connector_schema_readiness"):
		tx.events = append(tx.events, "write:readiness")
		if !tx.validated {
			return pgconn.CommandTag{}, errors.New("readiness before ownership validation")
		}
		tx.readyWritten = true
	case strings.HasPrefix(query, "INSERT "), strings.HasPrefix(query, "UPDATE "):
		tx.events = append(tx.events, "write:plan")
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
	return commandFixtureAt(t, testHistoricalMigrations)
}

func commandFixtureAt(t *testing.T, history []string) (*commandDatabase, string) {
	t.Helper()
	source := fixture(t)
	source[migrationHistoryKey] = migrationRows(history)
	if err := inventorySkillFiles(source, ""); err != nil {
		t.Fatal(err)
	}
	plan, err := makePlan(source, testKeys())
	if err != nil || len(plan.Report.Issues) != 0 {
		t.Fatal("invalid command fixture", err)
	}
	tx := &commandTransaction{
		source:          source,
		converted:       applyMemory(t, source, plan),
		migrations:      append([]string(nil), history...),
		retiredTables:   append([]string(nil), advancedRetiredTables...),
		bindingColumns:  append([]string(nil), advancedBindingColumns...),
		constraintNames: advancedConstraintFixture(),
	}
	return &commandDatabase{tx: tx}, plan.Report.SourceDigest
}

// advancedConstraintFixture returns the retained constraint names an advanced
// checkpoint database carries, including the not-yet-validated native claim FK.
func advancedConstraintFixture() map[string][]string {
	names := map[string][]string{}
	for _, group := range advancedConstraintGroups {
		names["public."+group.table] = append([]string(nil), group.names...)
	}
	return names
}

func eventIndex(events []string, want string) int {
	for i, event := range events {
		if event == want {
			return i
		}
	}
	return -1
}

func firstEventWithPrefix(events []string, prefix string) int {
	for i, event := range events {
		if strings.HasPrefix(event, prefix) {
			return i
		}
	}
	return -1
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
			err := runOnConnection(context.Background(), db, command, testKeys(), checkpointChoice{}, digest, "", func(report) error { return nil })
			if err != nil || db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.Serializable || db.tx.writes != 0 || db.tx.committed || !db.tx.rolledBack || hash(db.tx.source) != before {
				t.Fatal("read-only command mutated or committed", err)
			}
		})
	}
	for _, scenario := range []string{"write_failure", "verification_failure", "commit_failure", "stale_report", "success"} {
		t.Run(scenario, func(t *testing.T) {
			db, digest := commandFixture(t)
			switch scenario {
			case "write_failure":
				db.tx.failWrite = 2
			case "verification_failure":
				db.tx.invalidConversion = true
			case "commit_failure":
				db.tx.commitError = true
			case "stale_report":
				digest = strings.Repeat("0", 64)
			}
			var output report
			err := runOnConnection(context.Background(), db, "apply", testKeys(), checkpointChoice{}, digest, "", func(r report) error { output = r; return nil })
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
			if scenario == "stale_report" && db.tx.writes != 0 {
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
