package connectorfinalize

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

func unorderedDigest(rows []row) string {
	commitments := make([]string, 0, len(rows))
	for _, record := range rows {
		commitments = append(commitments, digest(record))
	}
	sort.Strings(commitments)
	return digest(commitments)
}

func Verify(ctx context.Context, db *gorm.DB, options Options) (Report, error) {
	var report Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; report, err = verify(tx, options, true); return err }, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return report, err
}

func verify(db *gorm.DB, option Options, requireMarker bool) (Report, error) {
	r := Report{Version: ReportVersion, Source: []TableProof{}, Issues: []Issue{}}
	if err := db.Exec("SET LOCAL TIME ZONE 'UTC'").Error; err != nil {
		return r, err
	}
	data := snapshot{}
	size := 0
	var identity string
	if err := db.Raw("SELECT current_database() || ':' || oid::text FROM pg_database WHERE datname=current_database()").Scan(&identity).Error; err != nil {
		return r, err
	}
	r.DatabaseFingerprint = digest(identity)
	var remaining int64
	if err := db.Raw("SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN ?", RetiredTables()).Scan(&remaining).Error; err != nil {
		return r, err
	}
	if remaining != 0 {
		return r, fmt.Errorf("schema2 still contains retired relations")
	}
	var err error
	if r.Native, r.DeferredNativeTables, err = readNativeTables(db, data, &size); err != nil {
		return r, err
	}
	if r.Hosts, err = readTables(db, hostTables(), data, &size); err != nil {
		return r, err
	}
	if len(data["connector_schema_readiness"]) != 1 {
		return r, fmt.Errorf("schema2 readiness row is missing")
	}
	ready := data["connector_schema_readiness"][0]
	r.SchemaVersion, r.State = number(ready, "schema_version"), text(ready, "state")
	if r.SchemaVersion != 2 || r.State != "ready" || ready["verified_at"] == nil {
		return r, fmt.Errorf("schema2 is not ready")
	}
	if err = db.Raw("SELECT max(id) FROM public.migrations").Scan(&r.Checkpoint).Error; err != nil {
		return r, err
	}
	if requireMarker {
		var marker int64
		if err := db.Table("migrations").Where("id=?", MigrationID).Count(&marker).Error; err != nil {
			return r, err
		}
		if marker != 1 {
			return r, fmt.Errorf("retirement migration marker is missing")
		}
	}
	proof := row(nil)
	for _, record := range data["connector_conversion_records"] {
		if text(record, "source_kind") == "retirement_v2" && text(record, "source_id") == MigrationID {
			proof = record
		}
	}
	if proof == nil || text(proof, "result") != "finalized" || !sha256Digest(text(proof, "source_digest")) || text(proof, "source_digest") != text(ready, "report_digest") {
		return r, fmt.Errorf("schema2 finalization proof is missing or inconsistent")
	}
	manifest := map[string]bool{}
	sourceCount := 0
	archivedRows := 0
	archiveProof := false
	for _, record := range data["connector_conversion_records"] {
		switch text(record, "source_kind") {
		case "retired_table_v2":
			name := text(record, "source_id")
			count, err := strconv.Atoi(strings.TrimPrefix(text(record, "result"), "rows:"))
			if !slices.Contains(RetiredTables(), name) || manifest[name] || err != nil || count < 0 || !strings.HasPrefix(text(record, "result"), "rows:") || !sha256Digest(text(record, "source_digest")) {
				return r, fmt.Errorf("invalid finalized table manifest")
			}
			manifest[name] = true
			sourceCount += count
		case "retired_source_v2":
			if text(record, "result") != "archived" || !sha256Digest(text(record, "source_digest")) {
				return r, fmt.Errorf("invalid finalized source commitment")
			}
			archivedRows++
		case "retirement_archive_v2":
			archiveProof = text(record, "source_id") == MigrationID && text(record, "result") == "sealed" && sha256Digest(text(record, "source_digest"))
		}
	}
	if len(manifest) != len(RetiredTables()) || archivedRows != sourceCount || (sourceCount > 0 && !archiveProof) {
		return r, fmt.Errorf("incomplete finalization manifest or sealed source proof")
	}
	objects, err := schemaObjects(db)
	if err != nil {
		return r, err
	}
	r.SchemaDigest = digest(objects)
	typeCheck, continuationFK := false, false
	for _, schema := range objects {
		if text(schema, "kind") == "column" && text(schema, "table") == "connector_target_bindings" && slices.Contains([]string{"compatibility", "compatibility_state"}, text(schema, "name")) {
			addIssue(&r, "connector_target_bindings", "compatibility_column_present")
		}
		if text(schema, "kind") == "constraint" && text(schema, "name") == "agent_resource_bindings_type_check" {
			typeCheck = text(schema, "table") == "agent_resource_bindings" && nativeBindingTypeConstraint(text(schema, "definition")) && schema["validated"] == true
		}
		if text(schema, "kind") == "constraint" && text(schema, "name") == "connector_confirmation_continuations_native_claim_fk" {
			continuationFK = schema["validated"] == true
		}
		for _, table := range RetiredTables() {
			if strings.Contains(text(schema, "definition"), table) {
				addIssue(&r, "retirement_schema", "retired_dependency_remains")
				break
			}
		}
	}
	if !typeCheck {
		addIssue(&r, "agent_resource_bindings", "native_type_constraint_missing")
	}
	if !continuationFK {
		addIssue(&r, "connector_confirmation_continuations", "validated_native_claim_fk_missing")
	}
	if err := verifyChannelScopeFunction(db, &r); err != nil {
		return r, err
	}
	validateNative(data, &r)
	validateNativeCatalog(db, data, &r)
	validateNativeClaims(data, &r)
	validateNativeEnvelopes(db, data, &r, option.Credentials)
	changes, err := catalogChanges(data)
	if err != nil || len(changes) > 0 {
		addIssue(&r, "connector_definitions", "retired_skill_reference")
	}
	hosts, err := hostChanges(data)
	if err != nil || len(hosts) > 0 {
		addIssue(&r, "host_metadata", "host_conversion_incomplete")
	}
	r.seal()
	if len(r.Issues) > 0 {
		return r, fmt.Errorf("schema2 verification found %d issues", len(r.Issues))
	}
	return r, nil
}

func nativeBindingTypeConstraint(definition string) bool {
	// pg_get_constraintdef adds casts and redundant parentheses according to
	// the column type. Only the exact native enum expression is permitted.
	value := strings.ToLower(definition)
	for _, cast := range []string{"::character varying[]", "::text[]", "::character varying", "::text"} {
		value = strings.ReplaceAll(value, cast, "")
	}
	value = strings.NewReplacer("(", "", ")", "", " ", "", "\n", "", "\t", "").Replace(value)
	return value == "checkbinding_type=anyarray['skill','knowledge_dataset','database','database_table','workflow']"
}
