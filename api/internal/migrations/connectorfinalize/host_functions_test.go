package connectorfinalize

import (
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func channelScopeFunctionObservation(prosrc string) channelScopeObservation {
	return channelScopeObservation{
		Functions: []channelScopeFunctionRow{{IdentityArguments: "", ReturnType: "trigger", LanguageName: "plpgsql", Prosrc: prosrc}},
		Triggers: []channelScopeTriggerRow{
			{Name: "trg_channel_bindings_scope", TableName: "channel_bindings"},
			{Name: "trg_channel_inbound_events_scope", TableName: "channel_inbound_events"},
			{Name: "trg_external_conversation_mappings_scope", TableName: "external_conversation_mappings"},
		},
	}
}

// The transition capability is only sound while the accepted body, the dead
// branch and the replacement commitment stay byte-pinned. This test re-derives
// every one of them without a database.
func TestChannelScopeFunctionBodyAndDeadBranchArePinned(t *testing.T) {
	before := channelScopeFunctionBeforeBody()
	if sha256Hex(before) != channelScopeFunctionBeforeDigest {
		t.Fatalf("accepted prosrc digest = %s", sha256Hex(before))
	}
	if strings.Count(before, "integration_connections") != 1 {
		t.Fatal("accepted prosrc must name the retired relation exactly once")
	}
	if strings.Count(before, channelScopeFunctionDeadFragment) != 1 {
		t.Fatal("dead branch must occur exactly once in the accepted body")
	}
	after, err := channelScopeFunctionAfterBody(before)
	if err != nil {
		t.Fatal(err)
	}
	if sha256Hex(after) != channelScopeFunctionAfterDigest {
		t.Fatalf("finalized prosrc digest = %s", sha256Hex(after))
	}
	if len(after) != len(before)-len(channelScopeFunctionDeadFragment) {
		t.Fatal("transition changed more than the dead branch")
	}
	if strings.Contains(after, channelScopeFunctionDeadFragment) {
		t.Fatal("dead branch survived the transition")
	}
	for _, table := range RetiredTables() {
		if strings.Contains(after, table) {
			t.Fatalf("finalized prosrc still references retired relation %s", table)
		}
	}
	for _, surviving := range []string{"channel_bindings", "channel_inbound_events", "channel_delivery_outbox", "RETURN NEW", "END;"} {
		if !strings.Contains(after, surviving) {
			t.Fatalf("transition removed the surviving branch %q", surviving)
		}
	}
}

func TestChannelScopeFunctionTransitionClassification(t *testing.T) {
	before := channelScopeFunctionBeforeBody()
	after, err := channelScopeFunctionAfterBody(before)
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(apply func(*channelScopeObservation)) channelScopeObservation {
		observation := channelScopeFunctionObservation(before)
		apply(&observation)
		return observation
	}
	for _, testCase := range []struct {
		name        string
		observation channelScopeObservation
		wantReplace bool
		wantErr     bool
	}{
		{name: "pinned_before_requires_transition", observation: channelScopeFunctionObservation(before), wantReplace: true},
		{name: "pinned_after_is_already_finalized", observation: mutate(func(o *channelScopeObservation) { o.Functions[0].Prosrc = after })},
		{name: "missing_function", observation: mutate(func(o *channelScopeObservation) { o.Functions = nil }), wantErr: true},
		{name: "overloaded_function", observation: mutate(func(o *channelScopeObservation) {
			o.Functions = append(o.Functions, o.Functions[0])
		}), wantErr: true},
		{name: "changed_arguments", observation: mutate(func(o *channelScopeObservation) { o.Functions[0].IdentityArguments = "integer" }), wantErr: true},
		{name: "changed_return_type", observation: mutate(func(o *channelScopeObservation) { o.Functions[0].ReturnType = "text" }), wantErr: true},
		{name: "changed_language", observation: mutate(func(o *channelScopeObservation) { o.Functions[0].LanguageName = "sql" }), wantErr: true},
		{name: "extra_trigger", observation: mutate(func(o *channelScopeObservation) {
			o.Triggers = append(o.Triggers, channelScopeTriggerRow{Name: "trg_extra_scope", TableName: "channel_delivery_outbox"})
		}), wantErr: true},
		{name: "missing_trigger", observation: mutate(func(o *channelScopeObservation) { o.Triggers = o.Triggers[:2] }), wantErr: true},
		{name: "renamed_trigger", observation: mutate(func(o *channelScopeObservation) { o.Triggers[0].Name = "trg_channel_bindings_other" }), wantErr: true},
		{name: "reinstated_channel_installation_trigger", observation: mutate(func(o *channelScopeObservation) {
			o.Triggers = append(o.Triggers, channelScopeTriggerRow{Name: "trg_channel_installations_scope", TableName: channelInstallationsTable})
		}), wantErr: true},
		{name: "non_empty_installations", observation: mutate(func(o *channelScopeObservation) { o.Installations = 1 }), wantErr: true},
		{name: "non_empty_credentials", observation: mutate(func(o *channelScopeObservation) { o.Credentials = 1 }), wantErr: true},
		{name: "drifted_body", observation: mutate(func(o *channelScopeObservation) { o.Functions[0].Prosrc = before + "\n\t-- drift\n\t" }), wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			transition, err := classifyChannelScopeFunction(testCase.observation)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("drift accepted: %#v", transition)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if transition.Replace != testCase.wantReplace || transition.Digest != channelScopeFunctionAfterDigest {
				t.Fatalf("transition = %#v, want replace=%v digest=%s", transition, testCase.wantReplace, channelScopeFunctionAfterDigest)
			}
		})
	}
}

func TestChannelScopeFunctionAfterBodyRequiresExactlyOneFragment(t *testing.T) {
	if _, err := channelScopeFunctionAfterBody("body without the branch"); err == nil {
		t.Fatal("missing branch accepted")
	}
	if _, err := channelScopeFunctionAfterBody(channelScopeFunctionDeadFragment + channelScopeFunctionDeadFragment); err == nil {
		t.Fatal("duplicated branch accepted")
	}
	if _, err := channelScopeFunctionAfterBody(channelScopeFunctionBeforeBody() + channelScopeFunctionDeadFragment); err == nil {
		t.Fatal("body with two branches accepted")
	}
}

func TestChannelScopeFunctionReplaceStatementQuotesBodyExactlyOnce(t *testing.T) {
	after, err := channelScopeFunctionAfterBody(channelScopeFunctionBeforeBody())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(after, "$") || strings.Contains(after, channelScopeFunctionQuoteTag) {
		t.Fatal("body cannot be dollar-quoted safely")
	}
	statement, err := channelScopeFunctionReplaceStatement(after)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(statement, channelScopeFunctionQuoteTag) != 2 || strings.Count(statement, after) != 1 {
		t.Fatal("replacement body is not quoted exactly once")
	}
	if !strings.HasPrefix(statement, "CREATE OR REPLACE FUNCTION public.enforce_channel_scope_consistency() RETURNS trigger AS ") || !strings.HasSuffix(statement, " LANGUAGE plpgsql") {
		t.Fatalf("unexpected replacement statement: %s", statement[:40])
	}
	if _, err := channelScopeFunctionReplaceStatement("body " + channelScopeFunctionQuoteTag + " body"); err == nil {
		t.Fatal("body containing the quoting tag accepted")
	}
}

func TestChannelScopeFunctionFinalizedState(t *testing.T) {
	after, err := channelScopeFunctionAfterBody(channelScopeFunctionBeforeBody())
	if err != nil {
		t.Fatal(err)
	}
	if err := finalizedChannelScopeFunction(channelScopeFunctionObservation(after)); err != nil {
		t.Fatal(err)
	}
	if err := finalizedChannelScopeFunction(channelScopeFunctionObservation(channelScopeFunctionBeforeBody())); err == nil {
		t.Fatal("unconverted body accepted as finalized")
	}
	extraTrigger := channelScopeFunctionObservation(after)
	extraTrigger.Triggers = append(extraTrigger.Triggers, channelScopeTriggerRow{Name: "trg_channel_installations_scope", TableName: channelInstallationsTable})
	if err := finalizedChannelScopeFunction(extraTrigger); err == nil {
		t.Fatal("extra trigger accepted as finalized")
	}
}

// The report must commit the host-function transition, not merely describe it.
func TestHostFunctionReportFieldsAreCommittedByDigest(t *testing.T) {
	baseline := Report{Version: ReportVersion, Issues: []Issue{}}
	baseline.seal()
	withTransition := Report{Version: ReportVersion, Issues: []Issue{}, HostFunctionTransitions: 1, HostFunctionDigest: channelScopeFunctionAfterDigest}
	withTransition.seal()
	withDigestOnly := Report{Version: ReportVersion, Issues: []Issue{}, HostFunctionDigest: channelScopeFunctionAfterDigest}
	withDigestOnly.seal()
	if baseline.Digest == withTransition.Digest || baseline.Digest == withDigestOnly.Digest || withTransition.Digest == withDigestOnly.Digest {
		t.Fatal("sealed digest does not commit the host-function transition")
	}
}

// TestChannelScopeFunctionPostgresTransition proves the catalog observation, the
// dollar-quoted replacement and the surviving trigger bindings against a real
// PostgreSQL catalog. It requires an explicitly isolated database and rolls back
// every change.
func TestChannelScopeFunctionPostgresTransition(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicit isolated ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated PostgreSQL database")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE public.channel_installations (id uuid PRIMARY KEY, connection_id uuid, organization_id uuid, workspace_id uuid, provider varchar(32), deleted_at timestamptz)`,
		`CREATE TABLE public.channel_installation_credentials (installation_id uuid PRIMARY KEY)`,
		`CREATE TABLE public.channel_bindings (id uuid PRIMARY KEY)`,
		`CREATE TABLE public.channel_inbound_events (id uuid PRIMARY KEY)`,
		`CREATE TABLE public.external_conversation_mappings (id uuid PRIMARY KEY)`,
	} {
		if err := tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	create, err := channelScopeFunctionReplaceStatement(channelScopeFunctionBeforeBody())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(create).Error; err != nil {
		t.Fatal(err)
	}
	for _, trigger := range []struct{ name, table string }{
		{"trg_channel_bindings_scope", "channel_bindings"},
		{"trg_channel_inbound_events_scope", "channel_inbound_events"},
		{"trg_external_conversation_mappings_scope", "external_conversation_mappings"},
	} {
		if err := tx.Exec(`CREATE TRIGGER ` + trigger.name + ` BEFORE INSERT OR UPDATE ON public.` + trigger.table + ` FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency()`).Error; err != nil {
			t.Fatal(err)
		}
	}
	before, err := observeChannelScopeFunction(tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Functions) != 1 || sha256Hex(before.Functions[0].Prosrc) != channelScopeFunctionBeforeDigest {
		t.Fatal("PostgreSQL did not store the pinned accepted prosrc")
	}
	transition, err := classifyChannelScopeFunction(before)
	if err != nil || !transition.Replace {
		t.Fatalf("planning transition = %#v, err = %v", transition, err)
	}
	if err := applyChannelScopeFunctionTransition(tx); err != nil {
		t.Fatal(err)
	}
	after, err := observeChannelScopeFunction(tx)
	if err != nil {
		t.Fatal(err)
	}
	if err := finalizedChannelScopeFunction(after); err != nil {
		t.Fatal(err)
	}
	if sha256Hex(after.Functions[0].Prosrc) != channelScopeFunctionAfterDigest {
		t.Fatal("PostgreSQL did not store the pinned finalized prosrc")
	}
	if len(after.Triggers) != 3 {
		t.Fatalf("trigger bindings after replacement = %d", len(after.Triggers))
	}
	if strings.Contains(after.Functions[0].Prosrc, "integration_connections") {
		t.Fatal("retired relation reference survived the transition")
	}
}
