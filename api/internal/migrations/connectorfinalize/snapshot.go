package connectorfinalize

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"gorm.io/gorm"
)

const maxRowsPerTable = 10000
const maxSnapshotBytes = 128 << 20

// Explicit host projections exclude message bodies and uploaded/custom Skill text.
var hostColumns = map[string]string{
	"agent_resource_bindings":                 "to_jsonb(t)",
	"agents_configs":                          "jsonb_build_object('id',id,'agent_mode',agent_mode)",
	"agent_published_versions":                "jsonb_build_object('id',id,'config_snapshot',config_snapshot)",
	"workflows":                               "jsonb_build_object('id',id,'graph',graph)",
	"workflow_runs":                           "jsonb_build_object('id',id,'graph',graph)",
	"workflow_run_logs":                       "jsonb_build_object('id',id,'workflow_id',workflow_id,'graph',graph,'features',features,'status',status,'runtime_protocol_version',runtime_protocol_version,'execution_generation',execution_generation,'active_execution_id',active_execution_id)",
	"workflow_node_runtime_logs":              "jsonb_build_object('id',id,'workflow_run_id',workflow_run_id,'node_id',node_id,'graph',graph,'features',features,'execution_metadata',execution_metadata,'status',status)",
	"workflow_run_pauses":                     "to_jsonb(t)",
	"chat_runtime_messages":                   "jsonb_build_object('id',id,'metadata',metadata)",
	"chat_runtime_conversations":              "to_jsonb(t) - 'name' - 'title' - 'description'",
	"chat_runtime_organization_skill_configs": "to_jsonb(t)",
	"chat_runtime_account_skill_preferences":  "to_jsonb(t)",
	"chat_runtime_custom_skills":              "jsonb_build_object('id',id,'organization_id',organization_id,'skill_id',skill_id,'runtime_type',runtime_type,'manifest',manifest,'status',status,'validation_error',validation_error)",
}

func hostTables() []string {
	result := make([]string, 0, len(hostColumns))
	for table := range hostColumns {
		result = append(result, table)
	}
	sort.Strings(result)
	return result
}

func readTables(db *gorm.DB, tables []string, data snapshot, bytesRead *int) ([]TableProof, error) {
	proofs := make([]TableProof, 0, len(tables))
	for _, table := range tables {
		if !slices.Contains(RetiredTables(), table) && !slices.Contains(NativeTables(), table) && !slices.Contains(hostTables(), table) {
			return nil, fmt.Errorf("table outside retirement inventory")
		}
		projection := "to_jsonb(t)"
		if columns, ok := hostColumns[table]; ok {
			projection = columns
		}
		rows, err := db.Raw("SELECT "+projection+" FROM public.\""+table+"\" t LIMIT ?", maxRowsPerTable+1).Rows()
		if err != nil {
			return nil, fmt.Errorf("read retirement inventory %s: %w", table, err)
		}
		entries, err := readRows(rows, bytesRead)
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", table, err)
		}
		data[table] = entries
		proofs = append(proofs, TableProof{Table: table, Rows: len(entries), Digest: digest(entries)})
	}
	return proofs, nil
}

func readRows(rows *sql.Rows, bytesRead *int) ([]row, error) {
	result := []row{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		*bytesRead += len(raw)
		if len(result) >= maxRowsPerTable || *bytesRead > maxSnapshotBytes {
			return nil, fmt.Errorf("bounded inventory exceeded; explicit backfill required")
		}
		var entry row
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&entry); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return digest(result[i]) < digest(result[j]) })
	return result, nil
}

// Plan is read-only, repeatable and bounded. A partial schema fails; it is never
// interpreted as a successful repeat. Verify handles an already-finalized schema.
type Options struct {
	Credentials *credentials.Store

	// allowDiscardInspect is used only by the explicit discard maintenance
	// command. It lets that command inventory a schema1 database whose
	// readiness row is conversion_required without treating that state as a
	// successful conversion.
	allowDiscardInspect bool
}

func Plan(ctx context.Context, db *gorm.DB, options ...Options) (Report, error) {
	var report Report
	var option Options
	if len(options) > 0 {
		option = options[0]
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		report, _, err = inspect(tx, option)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return report, err
}

func inspect(db *gorm.DB, option Options) (Report, snapshot, error) {
	r := Report{Version: ReportVersion, Issues: []Issue{}}
	if err := db.Exec("SET LOCAL TIME ZONE 'UTC'").Error; err != nil {
		return r, nil, err
	}
	data := snapshot{}
	var identity string
	if err := db.Raw("SELECT current_database() || ':' || oid::text FROM pg_database WHERE datname=current_database()").Scan(&identity).Error; err != nil {
		return r, nil, err
	}
	r.DatabaseFingerprint = digest(identity)
	if err := db.Raw("SELECT max(id) FROM public.migrations").Scan(&r.Checkpoint).Error; err != nil {
		return r, nil, err
	}
	var version struct {
		SchemaVersion int
		State         string
	}
	if err := db.Table("public.connector_schema_readiness").Where("id=1").Take(&version).Error; err != nil {
		return r, nil, err
	}
	r.SchemaVersion, r.State = version.SchemaVersion, version.State
	if err := validateSchema1Readiness(version.SchemaVersion, version.State, option.allowDiscardInspect); err != nil {
		return r, nil, err
	}
	bytesRead := 0
	var err error
	if r.Source, err = readTables(db, RetiredTables(), data, &bytesRead); err != nil {
		return r, nil, err
	}
	native, err := nativeTablesAtCheckpoint(db)
	if err != nil {
		return r, nil, err
	}
	if r.Native, err = readTables(db, native, data, &bytesRead); err != nil {
		return r, nil, err
	}
	if r.Hosts, err = readTables(db, hostTables(), data, &bytesRead); err != nil {
		return r, nil, err
	}
	objects, err := schemaObjects(db)
	if err != nil {
		return r, nil, err
	}
	dependencies, err := retirementDependencies(db)
	if err != nil {
		return r, nil, err
	}
	r.SchemaDigest = digest([]any{objects, dependencies})
	validateRetirementDependencies(dependencies, &r)
	for _, object := range objects {
		if text(object, "kind") != "function" && text(object, "kind") != "view" {
			continue
		}
		for _, table := range RetiredTables() {
			if strings.Contains(text(object, "definition"), table) {
				r.Issues = append(r.Issues, Issue{Table: table, Code: "dependent_" + text(object, "kind")})
			}
		}
	}
	validateSource(data, &r)
	validateNativeCatalog(db, data, &r)
	validateNativeClaims(data, &r)
	validateNativeEnvelopes(db, data, &r, option.Credentials)
	transitions, transitionErr := catalogChanges(data)
	if transitionErr != nil {
		addIssue(&r, "connector_definitions", "maintenance_contract_transition_unavailable")
	} else {
		r.CatalogTransitions = len(transitions)
	}
	hosts, hostErr := hostChanges(data)
	if hostErr != nil {
		addIssue(&r, "host_metadata", "bounded_host_conversion_unavailable")
	} else {
		r.HostTransitions = len(hosts)
	}
	r.seal()
	return r, data, nil
}

func validateSchema1Readiness(schemaVersion int, state string, discard bool) error {
	if schemaVersion == 1 && (state == "ready" || (discard && state == "conversion_required")) {
		return nil
	}
	if discard {
		return fmt.Errorf("Connector discard requires schema_version=1 and state=ready or conversion_required; found schema_version=%d state=%q", schemaVersion, state)
	}
	return fmt.Errorf("ready schema1 checkpoint required; found schema_version=%d state=%q (use connector-finalize discard for conversion_required)", schemaVersion, state)
}

func schemaObjects(db *gorm.DB) ([]row, error) {
	rows, err := db.Raw(`
SELECT jsonb_build_object('kind','constraint','table',c.conrelid::regclass::text,'name',c.conname,'definition',pg_get_constraintdef(c.oid),'validated',c.convalidated)
FROM pg_constraint c JOIN pg_namespace n ON n.oid=c.connamespace WHERE n.nspname='public'
UNION ALL SELECT jsonb_build_object('kind','index','table',tablename,'name',indexname,'definition',indexdef) FROM pg_indexes WHERE schemaname='public'
UNION ALL SELECT jsonb_build_object('kind','function','name',p.proname,'definition',p.prosrc) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.prokind IN ('f','p')
UNION ALL SELECT jsonb_build_object('kind','view','name',c.relname,'definition',pg_get_viewdef(c.oid)) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('v','m')
UNION ALL SELECT jsonb_build_object('kind','trigger','table',t.tgrelid::regclass::text,'name',t.tgname,'definition',pg_get_triggerdef(t.oid)) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND NOT t.tgisinternal
UNION ALL SELECT jsonb_build_object('kind','column','table',table_name,'name',column_name,'type',udt_name,'nullable',is_nullable,'default',column_default) FROM information_schema.columns WHERE table_schema='public'
`).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bytesRead := 0
	return readRows(rows, &bytesRead)
}
