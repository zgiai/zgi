package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type row map[string]any
type databaseSnapshot map[string][]row
type issue struct {
	Table string `json:"table"`
	ID    string `json:"id"`
	Code  string `json:"code"`
}
type report struct {
	State        string             `json:"state"`
	SourceDigest string             `json:"source_digest"`
	Counts       map[string]int     `json:"counts"`
	Issues       []issue            `json:"issues"`
	Results      []conversionResult `json:"results"`
}
type conversionResult struct {
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Result        string `json:"result"`
	DestinationID string `json:"destination_id,omitempty"`
}
type mutation struct {
	Table  string
	Key    row
	Values row
	Insert bool
}
type conversionPlan struct {
	Report    report
	Mutations []mutation
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var sqlNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Only these tables can be read or locked. No SQL or path comes from a source
// record or a user-supplied report. History is read but never updated.
var snapshotTables = []string{
	"aichat_integration_preferences", "agent_published_versions", "agent_resource_bindings", "agents", "agents_configs",
	"chat_runtime_conversations", "chat_runtime_custom_skills", "chat_runtime_organization_skill_configs",
	"connector_connections", "connector_confirmation_continuations", "connector_conversion_records", "connector_definition_versions", "connector_definitions",
	"connector_execution_route_claims", "connector_invocation_claims", "connector_legacy_identity_mappings", "connector_lifecycle_decisions",
	"connector_operation_outcomes", "connector_plugin_versions", "connector_reversal_baselines", "connector_schema_readiness", "connector_target_bindings", "connector_unavailable_references",
	"integration_action_policies", "integration_connection_grants", "integration_connections", "workflow_run_pauses", "workflow_runs", "workflows", "workspaces",
}
var mutableTables = map[string]bool{"connector_connections": true, "connector_target_bindings": true, "connector_unavailable_references": true, "connector_invocation_claims": true, "connector_confirmation_continuations": true, "connector_conversion_records": true, "connector_definitions": true, "connector_definition_versions": true, "connector_plugin_versions": true, "agents_configs": true, "agent_published_versions": true, "workflows": true, "workflow_runs": true, "workflow_run_pauses": true, "chat_runtime_custom_skills": true}

func tableExists(ctx context.Context, tx pgx.Tx, name string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+name).Scan(&exists)
	return exists, err
}
func lockSnapshotTables(ctx context.Context, tx pgx.Tx) error {
	for _, name := range snapshotTables {
		exists, err := tableExists(ctx, tx, name)
		if err != nil {
			return errors.New("cannot inspect conversion schema")
		}
		if exists {
			if _, err = tx.Exec(ctx, "LOCK TABLE "+pgx.Identifier{"public", name}.Sanitize()+" IN SHARE ROW EXCLUSIVE MODE"); err != nil {
				return fmt.Errorf("cannot acquire maintenance lock on %s", name)
			}
		}
	}
	return nil
}
func loadSnapshot(ctx context.Context, tx pgx.Tx) (databaseSnapshot, error) {
	snapshot := databaseSnapshot{}
	size := 0
	for _, name := range snapshotTables {
		exists, err := tableExists(ctx, tx, name)
		if err != nil {
			return nil, errors.New("cannot inspect conversion schema")
		}
		if !exists {
			continue
		}
		snapshot[name] = []row{}
		rows, err := tx.Query(ctx, "SELECT to_jsonb(t) FROM "+pgx.Identifier{"public", name}.Sanitize()+" t ORDER BY to_jsonb(t)::text")
		if err != nil {
			return nil, fmt.Errorf("cannot inventory %s", name)
		}
		for rows.Next() {
			var data []byte
			if err = rows.Scan(&data); err != nil {
				rows.Close()
				return nil, fmt.Errorf("cannot inventory %s", name)
			}
			size += len(data)
			if size > 512*1024*1024 {
				rows.Close()
				return nil, errors.New("inventory exceeds the 512 MiB maintenance limit")
			}
			var record row
			if json.Unmarshal(data, &record) != nil {
				rows.Close()
				return nil, fmt.Errorf("invalid record in %s", name)
			}
			snapshot[name] = append(snapshot[name], record)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot finish inventory of %s", name)
		}
	}
	if len(snapshot["connector_schema_readiness"]) != 1 {
		return nil, errors.New("apply the native Connector schema migration before conversion")
	}
	return snapshot, nil
}
func inventory(snapshot databaseSnapshot) report {
	r := report{State: "inventory", SourceDigest: hash(snapshot), Counts: map[string]int{}, Issues: []issue{}}
	for table, rows := range snapshot {
		r.Counts[table] = len(rows)
	}
	return r
}
func hash(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func text(r row, key string) string { value, _ := r[key].(string); return value }
func integer(r row, key string) int {
	v, _ := r[key].(float64)
	if i, ok := r[key].(int); ok {
		return i
	}
	return int(v)
}
func object(value any) row {
	if r, ok := value.(map[string]any); ok {
		return row(r)
	}
	if r, ok := value.(row); ok {
		return r
	}
	if s, ok := value.(string); ok {
		var r row
		if json.Unmarshal([]byte(s), &r) == nil {
			return r
		}
	}
	return row{}
}
func list(value any) []any {
	if r, ok := value.([]any); ok {
		return r
	}
	return []any{}
}
func stringsOf(value any) []string {
	out := []string{}
	for _, v := range list(value) {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	if v, ok := value.([]string); ok {
		return v
	}
	return out
}
func validID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func deterministicID(parts ...string) string {
	return uuid.NewSHA1(uuid.MustParse("169a7405-58ae-5a84-b7a6-2854c73043d6"), []byte(strings.Join(parts, "\x00"))).String()
}
func clone(r row) row { out := row{}; b, _ := json.Marshal(r); _ = json.Unmarshal(b, &out); return out }
func findBy(rows []row, key, value string) row {
	for _, r := range rows {
		if text(r, key) == value {
			return r
		}
	}
	return nil
}
func (p *conversionPlan) problem(table, id, code string) {
	p.Report.Issues = append(p.Report.Issues, issue{table, id, code})
}
func (p *conversionPlan) add(table string, key, values row, insert bool) {
	p.Mutations = append(p.Mutations, mutation{table, key, values, insert})
	p.Report.Counts["write:"+table]++
}
func (p *conversionPlan) record(kind, id string, source any, destination, result string) {
	p.Report.Results = append(p.Report.Results, conversionResult{kind, id, result, destination})
	values := row{"source_kind": kind, "source_id": id, "source_digest": hash(source), "result": result}
	if validID(destination) {
		values["destination_id"] = destination
	}
	p.add("connector_conversion_records", nil, values, true)
}

func applyPlan(ctx context.Context, tx pgx.Tx, p conversionPlan) error {
	for _, m := range p.Mutations {
		if !mutableTables[m.Table] {
			return errors.New("conversion plan contains an invalid destination")
		}
		cols := make([]string, 0, len(m.Values))
		for key := range m.Values {
			if !sqlNamePattern.MatchString(key) {
				return errors.New("conversion plan contains an invalid column")
			}
			cols = append(cols, key)
		}
		sort.Strings(cols)
		args := []any{}
		placeholders := []string{}
		assignments := []string{}
		quoted := []string{}
		for _, key := range cols {
			value := m.Values[key]
			switch value.(type) {
			case row, map[string]any, []any, []string:
				encoded, err := json.Marshal(value)
				if err != nil {
					return errors.New("cannot encode conversion record")
				}
				value = string(encoded)
			}
			args = append(args, value)
			placeholder := fmt.Sprintf("$%d", len(args))
			placeholders = append(placeholders, placeholder)
			quoted = append(quoted, pgx.Identifier{key}.Sanitize())
			assignments = append(assignments, pgx.Identifier{key}.Sanitize()+"="+placeholder)
		}
		table := pgx.Identifier{"public", m.Table}.Sanitize()
		var sql string
		if m.Insert {
			sql = "INSERT INTO " + table + " (" + strings.Join(quoted, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
		} else {
			if len(m.Key) == 0 {
				return errors.New("conversion update requires a stable identity")
			}
			keys := make([]string, 0, len(m.Key))
			for key := range m.Key {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			where := []string{}
			for _, key := range keys {
				if !sqlNamePattern.MatchString(key) {
					return errors.New("invalid conversion identity")
				}
				args = append(args, m.Key[key])
				where = append(where, pgx.Identifier{key}.Sanitize()+fmt.Sprintf("=$%d", len(args)))
			}
			sql = "UPDATE " + table + " SET " + strings.Join(assignments, ",") + " WHERE " + strings.Join(where, " AND ")
		}
		result, err := tx.Exec(ctx, sql, args...)
		if err != nil || result.RowsAffected() != 1 {
			return fmt.Errorf("conversion write failed in %s; transaction will roll back", m.Table)
		}
	}
	return nil
}
