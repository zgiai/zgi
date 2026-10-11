package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func runConnectorCheckpoint(args []string) error {
	flags := flag.NewFlagSet("connector-checkpoint", flag.ContinueOnError)
	id := flags.String("id", "", "registered historical Connector checkpoint")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	db, err := openConfiguredDB()
	if err != nil {
		return err
	}
	return migrations.RunToConnectorCheckpoint(db, *id)
}

func runConnectorFinalize(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: connector-finalize plan|apply|verify [--report PATH] [--expected-report SHA256] [--advanced-checkpoint MIGRATION_ID] [--archive PATH --archive-key-file PATH]")
	}
	command := args[0]
	if command != "plan" && command != "apply" && command != "verify" {
		return fmt.Errorf("unknown Connector finalization command")
	}
	flags := flag.NewFlagSet("connector-finalize", flag.ContinueOnError)
	reportPath := flags.String("report", "", "safe JSON report output (stdout when omitted)")
	expected := flags.String("expected-report", "", "exact clean plan digest required by apply")
	archivePath := flags.String("archive", "", "encrypted retirement archive outside the repository")
	keyPath := flags.String("archive-key-file", "", "independent binary 32-byte archive key; plan creates it exclusively when absent")
	advancedCheckpoint := flags.String("advanced-checkpoint", "", "exact applied migration ID for a schema1 head later than the registered checkpoint; valid for plan and apply only")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	// flags.Visit reports only the flags that actually appeared, so an explicit
	// empty --advanced-checkpoint is distinguishable from the flag being absent.
	advancedSet := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "advanced-checkpoint" {
			advancedSet = true
		}
	})
	if flags.NArg() != 0 || (*archivePath == "") != (*keyPath == "") {
		return fmt.Errorf("archive and archive-key-file must be supplied together")
	}
	if command == "verify" && *archivePath != "" {
		return fmt.Errorf("schema2 verification uses native data, not historical archives")
	}
	// Validate the command/flag contract before opening any database connection.
	option, err := connectorFinalizeOptions(command, advancedSet, *advancedCheckpoint)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.InitDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("open configured finalization database: %w", err)
	}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	if len(cfg.Connectors.CredentialKeys) > 0 {
		option.Credentials, err = credentials.NewStore(cfg.Connectors.CredentialActiveKeyID, cfg.Connectors.CredentialKeys)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var key []byte
	if *keyPath != "" {
		key, err = readBounded(*keyPath, 32)
		if os.IsNotExist(err) && command == "plan" {
			key = make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				return err
			}
			err = writeExclusive(*keyPath, key)
		}
		if err != nil || len(key) != 32 {
			clear(key)
			return fmt.Errorf("archive key must be a readable independent 32-byte file")
		}
		defer clear(key)
	}
	var report connectorfinalize.Report
	switch command {
	case "plan":
		report, err = connectorfinalize.Plan(ctx, db, option)
		if err == nil && *archivePath != "" {
			var encrypted []byte
			encrypted, err = connectorfinalize.CreateArchive(ctx, db, report.Digest, key, option)
			if err == nil {
				err = writeExclusive(*archivePath, encrypted)
			}
		}
	case "apply":
		var encrypted []byte
		if *archivePath != "" {
			encrypted, err = readBounded(*archivePath, 257<<20)
			if err != nil {
				return fmt.Errorf("cannot read bounded encrypted archive")
			}
		}
		report, err = connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{Options: option, ExpectedReport: *expected, Archive: encrypted, ArchiveKey: key})
	case "verify":
		report, err = connectorfinalize.Verify(ctx, db, option)
	}
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, encoded, 0600); err != nil {
			return fmt.Errorf("write safe finalization report: %w", err)
		}
	} else {
		if _, err := os.Stdout.Write(encoded); err != nil {
			return err
		}
	}
	if len(report.Issues) > 0 {
		return fmt.Errorf("report contains %d unresolved issues; no database changes applied", len(report.Issues))
	}
	return nil
}

// connectorFinalizeOptions validates the command against its flags before any
// database connection is opened. advancedSet records whether
// --advanced-checkpoint appeared at all, so an explicit empty value is rejected
// instead of being silently treated as the default. The explicit advanced
// checkpoint is a plan/apply opt-in: verify reads the applied schema2 state, so
// it rejects the flag even when its value is empty.
func connectorFinalizeOptions(command string, advancedSet bool, advancedCheckpoint string) (connectorfinalize.Options, error) {
	if advancedSet && command == "verify" {
		return connectorfinalize.Options{}, fmt.Errorf("verify reads the applied schema2 state; --advanced-checkpoint is valid for plan and apply only")
	}
	if advancedSet && advancedCheckpoint == "" {
		return connectorfinalize.Options{}, fmt.Errorf("--advanced-checkpoint requires a non-empty migration ID")
	}
	return connectorfinalize.Options{AdvancedCheckpoint: advancedCheckpoint}, nil
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		clear(data)
		return nil, fmt.Errorf("file exceeds permitted size")
	}
	return data, nil
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create recovery artifact exclusively: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
