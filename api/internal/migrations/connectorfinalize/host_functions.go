package connectorfinalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// The digital-human channel protocol keeps one trigger function that used to
// query the retired integration_connections table from a channel_installations
// branch. Migration 202608231300000000 dropped trg_channel_installations_scope,
// so that branch is unreachable and the finalizer replaces the byte-pinned
// function body with the same body minus the dead branch.
//
// This is a closed capability: no table, column or SQL text is accepted from a
// caller, and the replacement statement is never built from database text.
const (
	channelScopeFunctionName            = "enforce_channel_scope_consistency"
	channelInstallationsTable           = "channel_installations"
	channelInstallationCredentialsTable = "channel_installation_credentials"

	// SHA-256 over the exact prosrc created by migration
	// 202608191032439914_create_digital_human_channels.
	channelScopeFunctionBeforeDigest = "c0e613d5051352509e57b8aca630bbabe91551b20a277467e6c5f67a9b540045"
	// SHA-256 over the same prosrc after the dead branch below is removed.
	channelScopeFunctionAfterDigest = "1d2b63399c9f2ed4773e5abda2408087ce5c0c04d5c04058c542e77b2f3cd4fc"

	// A dollar-quote tag that cannot appear inside the fixed body.
	channelScopeFunctionQuoteTag = "$zgi_channel_scope_function$"

	// The exact unreachable branch, including its trailing blank line. It must
	// match the accepted body exactly once.
	channelScopeFunctionDeadFragment = "\t\tIF TG_TABLE_NAME = 'channel_installations' THEN\n" +
		"\t\t\tSELECT organization_id INTO parent_org FROM public.integration_connections WHERE id = NEW.connection_id AND deleted_at IS NULL;\n" +
		"\t\t\tIF parent_org IS NULL OR parent_org <> NEW.organization_id THEN\n" +
		"\t\t\t\tRAISE EXCEPTION 'channel installation connection scope mismatch' USING ERRCODE = '23514';\n" +
		"\t\t\tEND IF;\n" +
		"\t\t\tRETURN NEW;\n" +
		"\t\tEND IF;\n\n"

	// Go discards carriage returns inside raw string literals; the explicit
	// normalization keeps the pinned digests independent of checkout settings.
	channelScopeFunctionBeforeBodyRaw = `
	DECLARE
		parent_org uuid;
		parent_workspace uuid;
		parent_provider varchar(32);
		parent_connection uuid;
	BEGIN
		IF TG_TABLE_NAME = 'channel_installations' THEN
			SELECT organization_id INTO parent_org FROM public.integration_connections WHERE id = NEW.connection_id AND deleted_at IS NULL;
			IF parent_org IS NULL OR parent_org <> NEW.organization_id THEN
				RAISE EXCEPTION 'channel installation connection scope mismatch' USING ERRCODE = '23514';
			END IF;
			RETURN NEW;
		END IF;

		SELECT organization_id, workspace_id, provider, connection_id
		INTO parent_org, parent_workspace, parent_provider, parent_connection
		FROM public.channel_installations WHERE id = NEW.installation_id AND deleted_at IS NULL;
		IF parent_org IS NULL OR parent_org <> NEW.organization_id OR parent_workspace <> NEW.workspace_id THEN
			RAISE EXCEPTION 'channel installation scope mismatch' USING ERRCODE = '23514';
		END IF;

		IF TG_TABLE_NAME = 'channel_bindings' THEN
			IF NOT EXISTS (
				SELECT 1 FROM public.external_conversation_mappings mapping
				WHERE mapping.id = NEW.conversation_mapping_id
				  AND mapping.installation_id = NEW.installation_id
				  AND mapping.organization_id = NEW.organization_id
				  AND mapping.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel binding conversation scope mismatch' USING ERRCODE = '23514';
			END IF;
			IF NEW.subject_type = 'digital_human' AND NOT EXISTS (
				SELECT 1 FROM public.digital_humans subject
				WHERE subject.id = NEW.subject_id
				  AND subject.organization_id = NEW.organization_id
				  AND subject.workspace_id = NEW.workspace_id
				  AND subject.deleted_at IS NULL
			) THEN
				RAISE EXCEPTION 'channel binding subject scope mismatch' USING ERRCODE = '23514';
			END IF;
		END IF;

		IF TG_TABLE_NAME = 'channel_inbound_events' THEN
			IF NEW.binding_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM public.channel_bindings binding
				WHERE binding.id = NEW.binding_id
				  AND binding.installation_id = NEW.installation_id
				  AND binding.organization_id = NEW.organization_id
				  AND binding.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel inbound binding scope mismatch' USING ERRCODE = '23514';
			END IF;
			IF NEW.invocation_run_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM public.digital_human_runs run
				WHERE run.id = NEW.invocation_run_id
				  AND run.organization_id = NEW.organization_id
				  AND run.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel inbound run scope mismatch' USING ERRCODE = '23514';
			END IF;
		END IF;

		IF TG_TABLE_NAME = 'channel_delivery_outbox' THEN
			IF NEW.provider <> parent_provider OR NEW.connection_id <> parent_connection THEN
				RAISE EXCEPTION 'channel delivery provider connection mismatch' USING ERRCODE = '23514';
			END IF;
			IF NOT EXISTS (
				SELECT 1 FROM public.channel_bindings binding
				WHERE binding.id = NEW.binding_id
				  AND binding.installation_id = NEW.installation_id
				  AND binding.organization_id = NEW.organization_id
				  AND binding.workspace_id = NEW.workspace_id
				  AND binding.subject_type = NEW.subject_type
				  AND binding.subject_id = NEW.subject_id
			) THEN
				RAISE EXCEPTION 'channel delivery binding scope mismatch' USING ERRCODE = '23514';
			END IF;
			IF NOT EXISTS (
				SELECT 1 FROM public.channel_inbound_events inbound
				WHERE inbound.id = NEW.inbound_event_id
				  AND inbound.installation_id = NEW.installation_id
				  AND inbound.binding_id = NEW.binding_id
				  AND inbound.invocation_run_id = NEW.run_id
				  AND inbound.organization_id = NEW.organization_id
				  AND inbound.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel delivery inbound event mismatch' USING ERRCODE = '23514';
			END IF;
			IF NOT EXISTS (
				SELECT 1 FROM public.digital_human_runs run
				WHERE run.id = NEW.run_id
				  AND run.organization_id = NEW.organization_id
				  AND run.workspace_id = NEW.workspace_id
				  AND run.digital_human_id = NEW.subject_id
			) THEN
				RAISE EXCEPTION 'channel delivery run subject mismatch' USING ERRCODE = '23514';
			END IF;
		END IF;
		RETURN NEW;
	END;
	`
)

type channelScopeFunctionRow struct {
	IdentityArguments string
	ReturnType        string
	LanguageName      string
	Prosrc            string
}

type channelScopeTriggerRow struct {
	Name      string
	TableName string
}

type channelScopeObservation struct {
	Functions     []channelScopeFunctionRow
	Triggers      []channelScopeTriggerRow
	Installations int64
	Credentials   int64
}

// channelScopeFunctionTransition is the classification of one observation. It
// never carries database text: Replace decides whether the fixed statement must
// run, and Digest is the pinned post-transition prosrc commitment.
type channelScopeFunctionTransition struct {
	Replace bool
	Digest  string
}

func channelScopeExpectedTriggers() []string {
	return []string{
		"channel_bindings:trg_channel_bindings_scope",
		"channel_inbound_events:trg_channel_inbound_events_scope",
		"external_conversation_mappings:trg_external_conversation_mappings_scope",
	}
}

func channelScopeFunctionBeforeBody() string {
	return strings.ReplaceAll(channelScopeFunctionBeforeBodyRaw, "\r\n", "\n")
}

// channelScopeFunctionAfterBody removes the fixed dead branch exactly once. It
// preserves every remaining byte of the accepted function body.
func channelScopeFunctionAfterBody(before string) (string, error) {
	if strings.Count(before, channelScopeFunctionDeadFragment) != 1 {
		return "", fmt.Errorf("channel scope dead branch must match exactly once")
	}
	return strings.Replace(before, channelScopeFunctionDeadFragment, "", 1), nil
}

// channelScopeFunctionReplaceStatement renders the closed replacement DDL. The
// body is dollar-quoted with a tag that is proven absent from it.
func channelScopeFunctionReplaceStatement(body string) (string, error) {
	if strings.Contains(body, channelScopeFunctionQuoteTag) {
		return "", fmt.Errorf("channel scope function body contains the quoting tag")
	}
	return "CREATE OR REPLACE FUNCTION public.enforce_channel_scope_consistency() RETURNS trigger AS " +
		channelScopeFunctionQuoteTag + body + channelScopeFunctionQuoteTag + " LANGUAGE plpgsql", nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// observeChannelScopeFunction reads the exact catalog state the transition
// depends on: the function signature and body, every non-internal trigger bound
// to it, and the row counts of the two channel scope tables.
func observeChannelScopeFunction(db *gorm.DB) (channelScopeObservation, error) {
	observation := channelScopeObservation{}
	if err := db.Raw(`SELECT pg_get_function_identity_arguments(p.oid) AS identity_arguments,
	p.prorettype::regtype::text AS return_type, l.lanname AS language_name, p.prosrc AS prosrc
FROM pg_proc p
JOIN pg_namespace n ON n.oid=p.pronamespace
JOIN pg_language l ON l.oid=p.prolang
WHERE n.nspname='public' AND p.proname=?`, channelScopeFunctionName).Scan(&observation.Functions).Error; err != nil {
		return observation, fmt.Errorf("inspect channel scope function: %w", err)
	}
	if err := db.Raw(`SELECT t.tgname AS name, c.relname AS table_name
FROM pg_trigger t
JOIN pg_class c ON c.oid=t.tgrelid
JOIN pg_namespace n ON n.oid=c.relnamespace
JOIN pg_proc p ON p.oid=t.tgfoid
WHERE n.nspname='public' AND NOT t.tgisinternal AND p.proname=?`, channelScopeFunctionName).Scan(&observation.Triggers).Error; err != nil {
		return observation, fmt.Errorf("inspect channel scope triggers: %w", err)
	}
	if err := db.Raw(`SELECT count(*) FROM public."` + channelInstallationsTable + `"`).Scan(&observation.Installations).Error; err != nil {
		return observation, fmt.Errorf("inspect channel installations: %w", err)
	}
	if err := db.Raw(`SELECT count(*) FROM public."` + channelInstallationCredentialsTable + `"`).Scan(&observation.Credentials).Error; err != nil {
		return observation, fmt.Errorf("inspect channel installation credentials: %w", err)
	}
	return observation, nil
}

// validateChannelScopeFunctionShape requires the exact signature and the exact
// three trigger bindings. Any drift, overload or extra (in particular a
// channel_installations) trigger fails closed.
func validateChannelScopeFunctionShape(observation channelScopeObservation) (channelScopeFunctionRow, error) {
	if len(observation.Functions) != 1 {
		return channelScopeFunctionRow{}, fmt.Errorf("channel scope function must exist exactly once")
	}
	function := observation.Functions[0]
	if function.IdentityArguments != "" || function.ReturnType != "trigger" || function.LanguageName != "plpgsql" {
		return function, fmt.Errorf("channel scope function signature changed")
	}
	triggers := make([]string, 0, len(observation.Triggers))
	for _, trigger := range observation.Triggers {
		if trigger.TableName == channelInstallationsTable {
			return function, fmt.Errorf("channel installation scope trigger remains")
		}
		triggers = append(triggers, trigger.TableName+":"+trigger.Name)
	}
	sort.Strings(triggers)
	if !slices.Equal(triggers, channelScopeExpectedTriggers()) {
		return function, fmt.Errorf("channel scope trigger binding changed")
	}
	return function, nil
}

// classifyChannelScopeFunction accepts exactly the pinned old body (a pending
// transition) or the pinned new body (already transitioned). Every other
// function reference, body drift or non-empty channel scope table fails closed.
func classifyChannelScopeFunction(observation channelScopeObservation) (channelScopeFunctionTransition, error) {
	function, err := validateChannelScopeFunctionShape(observation)
	if err != nil {
		return channelScopeFunctionTransition{}, err
	}
	if observation.Installations != 0 || observation.Credentials != 0 {
		return channelScopeFunctionTransition{}, fmt.Errorf("channel scope inventory is not empty")
	}
	transition := channelScopeFunctionTransition{Digest: channelScopeFunctionAfterDigest}
	switch sha256Hex(function.Prosrc) {
	case channelScopeFunctionBeforeDigest:
		transition.Replace = true
	case channelScopeFunctionAfterDigest:
	default:
		return channelScopeFunctionTransition{}, fmt.Errorf("channel scope function body drifted")
	}
	return transition, nil
}

// applyChannelScopeFunctionTransition re-verifies the reviewed classification
// inside the locked maintenance transaction, then replaces the exact old
// function. It is called before the retirement DDL so no surviving object ever
// names a retired relation.
func applyChannelScopeFunctionTransition(tx *gorm.DB) error {
	if err := tx.Exec(`LOCK TABLE public."` + channelInstallationsTable + `", public."` + channelInstallationCredentialsTable + `" IN ACCESS EXCLUSIVE MODE`).Error; err != nil {
		return fmt.Errorf("freeze channel scope inventory: %w", err)
	}
	observation, err := observeChannelScopeFunction(tx)
	if err != nil {
		return err
	}
	transition, err := classifyChannelScopeFunction(observation)
	if err != nil {
		return fmt.Errorf("channel scope function transition unavailable: %w", err)
	}
	if !transition.Replace {
		return nil
	}
	if len(observation.Functions) != 1 {
		return fmt.Errorf("channel scope function disappeared during maintenance")
	}
	if strings.Count(observation.Functions[0].Prosrc, channelScopeFunctionDeadFragment) != 1 {
		return fmt.Errorf("channel scope dead branch did not match exactly once in the accepted function")
	}
	after, err := channelScopeFunctionAfterBody(channelScopeFunctionBeforeBody())
	if err != nil {
		return err
	}
	if sha256Hex(after) != channelScopeFunctionAfterDigest {
		return fmt.Errorf("channel scope function replacement drifted from its pinned digest")
	}
	statement, err := channelScopeFunctionReplaceStatement(after)
	if err != nil {
		return err
	}
	if err := tx.Exec(statement).Error; err != nil {
		return fmt.Errorf("replace channel scope function: %w", err)
	}
	return nil
}

// verifyChannelScopeFunction requires the finalized body, the three surviving
// trigger bindings and the absence of every retired relation reference.
func verifyChannelScopeFunction(db *gorm.DB, report *Report) error {
	observation, err := observeChannelScopeFunction(db)
	if err != nil {
		return err
	}
	if err := finalizedChannelScopeFunction(observation); err != nil {
		addIssue(report, "retirement_schema", "channel_scope_function_not_finalized")
		return nil
	}
	report.HostFunctionDigest = channelScopeFunctionAfterDigest
	return nil
}

func finalizedChannelScopeFunction(observation channelScopeObservation) error {
	function, err := validateChannelScopeFunctionShape(observation)
	if err != nil {
		return err
	}
	if sha256Hex(function.Prosrc) != channelScopeFunctionAfterDigest {
		return fmt.Errorf("channel scope function body is not the finalized body")
	}
	for _, table := range RetiredTables() {
		if strings.Contains(function.Prosrc, table) {
			return fmt.Errorf("channel scope function still references a retired relation")
		}
	}
	return nil
}
