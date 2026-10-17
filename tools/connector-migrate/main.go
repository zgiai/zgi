// connector-migrate is a maintenance-window program. It imports neither the
// API runtime nor a provider implementation and has no provider HTTP client.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: connector-migrate inventory|dry-run|apply|verify --keys-file PATH [--expected-report SHA256]; DSN comes from CONNECTOR_MIGRATE_DSN")
	}
	command := args[0]
	if command != "inventory" && command != "dry-run" && command != "apply" && command != "verify" {
		return errors.New("unknown command")
	}
	flags := flag.NewFlagSet("connector-migrate", flag.ContinueOnError)
	keyFile := flags.String("keys-file", "", "private JSON file containing active/native/previous keyrings")
	expected := flags.String("expected-report", "", "source digest from dry-run (required by apply)")
	skillsRoot := flags.String("skills-root", "", "explicit root for read-only verification of stored Skill manifests")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("unexpected positional arguments")
	}
	var keys keyrings
	if command != "inventory" {
		bytes, err := os.ReadFile(*keyFile)
		if err != nil {
			return errors.New("cannot read private keyring file")
		}
		defer clear(bytes)
		if json.Unmarshal(bytes, &keys) != nil || !validKeyrings(keys) {
			return errors.New("invalid private keyring file")
		}
	}
	dsn := os.Getenv("CONNECTOR_MIGRATE_DSN")
	if dsn == "" {
		return errors.New("CONNECTOR_MIGRATE_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errors.New("cannot connect to migration database")
	}
	defer conn.Close(ctx)
	return runOnConnection(ctx, conn, command, keys, *expected, *skillsRoot, writeReport)
}

type transactionSource interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

func runOnConnection(ctx context.Context, conn transactionSource, command string, keys keyrings, expected, skillsRoot string, output func(report) error) error {
	options := pgx.TxOptions{IsoLevel: pgx.Serializable}
	if command == "inventory" || command == "dry-run" || command == "verify" {
		options.AccessMode = pgx.ReadOnly
	}
	tx, err := conn.BeginTx(ctx, options)
	if err != nil {
		return errors.New("cannot begin migration transaction")
	}
	defer tx.Rollback(ctx)
	// Source commitments must match the finalizer's UTC snapshots regardless
	// of the database/session timezone. This changes only this transaction's
	// timestamp rendering, not persisted values or database configuration.
	if _, err = tx.Exec(ctx, "SET LOCAL TIME ZONE 'UTC'"); err != nil {
		return errors.New("cannot configure migration snapshot timezone")
	}
	if command == "apply" {
		if !digestPattern.MatchString(expected) {
			return errors.New("apply requires --expected-report from a reviewed dry-run")
		}
		var locked bool
		if tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtext('zgi:migrations'))").Scan(&locked) != nil || !locked {
			return errors.New("another migration owns the maintenance lock")
		}
		if _, err = tx.Exec(ctx, "SET LOCAL lock_timeout = '10s'"); err != nil {
			return errors.New("cannot configure maintenance lock timeout")
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(726914054)"); err != nil {
			return errors.New("cannot acquire conversion lock")
		}
		if err = lockSnapshotTables(ctx, tx); err != nil {
			return err
		}
	}
	snapshot, err := loadSnapshot(ctx, tx)
	if err != nil {
		return err
	}
	if err = validateCheckpoint(snapshot); err != nil {
		return err
	}
	if err = inventorySkillFiles(snapshot, skillsRoot); err != nil {
		return err
	}
	if command == "inventory" {
		return output(inventory(snapshot))
	}
	if command == "verify" {
		report := verifySnapshot(snapshot, keys)
		if err := output(report); err != nil {
			return err
		}
		if len(report.Issues) > 0 {
			return errors.New("native verification failed")
		}
		return nil
	}
	plan, err := makePlan(snapshot, keys)
	if err != nil {
		return err
	}
	if command == "dry-run" {
		return output(plan.Report)
	}
	if plan.Report.SourceDigest != expected {
		return errors.New("source changed after dry-run; run and review dry-run again")
	}
	if len(plan.Report.Issues) > 0 {
		_ = output(plan.Report)
		return errors.New("conversion has unresolved issues; nothing was applied")
	}
	if err = applyPlan(ctx, tx, plan); err != nil {
		return err
	}
	converted, err := loadSnapshot(ctx, tx)
	if err != nil {
		return err
	}
	if err = inventorySkillFiles(converted, skillsRoot); err != nil {
		return err
	}
	verified := verifySnapshot(converted, keys)
	if len(verified.Issues) > 0 {
		_ = output(verified)
		return errors.New("post-conversion verification failed; transaction rolled back")
	}
	if _, err = tx.Exec(ctx, "ALTER TABLE connector_confirmation_continuations VALIDATE CONSTRAINT connector_confirmation_continuations_native_claim_fk"); err != nil {
		return errors.New("native continuation ownership validation failed")
	}
	if _, err = tx.Exec(ctx, "UPDATE connector_schema_readiness SET state='ready',report_digest=$1,verified_at=now() WHERE id=1 AND schema_version=1", plan.Report.SourceDigest); err != nil {
		return errors.New("cannot attest native schema")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("conversion commit failed; verify database state before retry")
	}
	plan.Report.State = "applied"
	return output(plan.Report)
}
func writeReport(value report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
func validKeyrings(keys keyrings) bool {
	if !keyIDPattern.MatchString(keys.Active) || len(keys.Native[keys.Active]) != 32 || len(keys.Native) > 32 || len(keys.Previous) > 32 {
		return false
	}
	for _, ring := range []map[string]string{keys.Native, keys.Previous} {
		for id, key := range ring {
			if !keyIDPattern.MatchString(id) || len(key) != 32 {
				return false
			}
		}
	}
	return true
}
