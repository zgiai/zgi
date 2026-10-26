package socialops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

func testScope() Scope {
	return Scope{
		OrganizationID: uuid.MustParse("10000000-0000-0000-0000-000000000001"),
		WorkspaceID:    uuid.MustParse("20000000-0000-0000-0000-000000000002"),
		OwnerID:        uuid.MustParse("30000000-0000-0000-0000-000000000003"),
		EmployeeID:     uuid.MustParse("40000000-0000-0000-0000-000000000004"),
	}
}

func allCapabilityRequirements(inbox RequirementMode) []CapabilityRequirement {
	return []CapabilityRequirement{
		{Capability: CapabilityAccount, Mode: RequirementRequired},
		{Capability: CapabilityPost, Mode: RequirementRequired},
		{Capability: CapabilityReadback, Mode: RequirementRequired},
		{Capability: CapabilityComment, Mode: RequirementRequired},
		{Capability: CapabilityInbox, Mode: inbox},
		{Capability: CapabilityMetrics, Mode: RequirementRequired},
		{Capability: CapabilityImages, Mode: RequirementRequired},
	}
}

func validConfiguration(connectorID string, inbox RequirementMode) Configuration {
	return Configuration{
		Scope:                  testScope(),
		EmployeeVersionID:      uuid.MustParse("50000000-0000-0000-0000-000000000005"),
		Revision:               7,
		Product:                ProductConfiguration{Name: "Acme Analytics", Description: "Released analytics software"},
		Audience:               "Data engineering teams",
		PublicationLanguage:    "en-US",
		InternalReviewLanguage: "zh-Hans",
		Sources: []SourceReference{
			{URL: "https://example.com/product"},
			{ArtifactID: uuid.MustParse("60000000-0000-0000-0000-000000000006")},
		},
		BrandArtifactRefs: []uuid.UUID{uuid.MustParse("70000000-0000-0000-0000-000000000007")},
		Research: ResearchBinding{
			ConnectorID:  "web-search",
			ConnectionID: uuid.MustParse("a0000000-0000-0000-0000-00000000000a"),
		},
		Platforms: []PlatformBinding{{
			Platform:             strings.TrimSuffix(connectorID, "-social"),
			ConnectorID:          connectorID,
			ConnectionID:         uuid.MustParse("80000000-0000-0000-0000-000000000008"),
			AccountID:            "account-42",
			MaxDailyPublications: 2,
			Capabilities:         allCapabilityRequirements(inbox),
		}},
		Timezone: "Asia/Shanghai",
		Cadence: Cadence{
			DailyContentCron:   "0 9 * * *",
			InboxCron:          "*/20 * * * *",
			SevenDaySampleCron: "15 10 * * *",
			WeeklyReviewCron:   "0 16 * * 5",
		},
		TeamHandoff: TeamHandoff{
			ConnectorID:  "team-mail",
			ConnectionID: uuid.MustParse("90000000-0000-0000-0000-000000000009"),
			OperationID:  "team-mail.message.send",
			Destination:  "support@example.com",
		},
	}
}

func definition(t *testing.T, id string) catalog.Definition {
	t.Helper()
	for _, candidate := range append(catalog.SocialMCPDefinitions(), catalog.Definition{
		ID: id, Surfaces: []catalog.Surface{catalog.SurfaceDigitalHuman},
		Operations: []catalog.Operation{{ID: id + ".message.send", Effect: catalog.OperationEffectExternalSend}},
	}) {
		if candidate.ID == id {
			return candidate
		}
	}
	t.Fatalf("missing definition %q", id)
	return catalog.Definition{}
}

func restoredConnection(t *testing.T, scope Scope, id string, connectionID uuid.UUID, accounts ...string) RestoredConnection {
	t.Helper()
	d := definition(t, id)
	available := make([]string, 0, len(d.Operations))
	for _, operation := range d.Operations {
		available = append(available, operation.ID)
	}
	return RestoredConnection{
		Scope:               scope,
		ConnectorID:         id,
		ConnectionID:        connectionID,
		Definition:          d,
		AccountIDs:          accounts,
		AvailableOperations: available,
		GrantedOperations:   append([]string(nil), available...),
	}
}

func TestConfigurationValidationRejectsAmbiguousOrIncompleteInput(t *testing.T) {
	base := validConfiguration("x-social", RequirementOptional)
	tests := []struct {
		name   string
		mutate func(*Configuration)
	}{
		{"missing publication language", func(c *Configuration) { c.PublicationLanguage = "" }},
		{"missing research connector", func(c *Configuration) { c.Research.ConnectorID = "" }},
		{"missing research connection", func(c *Configuration) { c.Research.ConnectionID = uuid.Nil }},
		{"non HTTPS source", func(c *Configuration) { c.Sources = []SourceReference{{URL: "http://example.com"}} }},
		{"ambiguous source", func(c *Configuration) {
			c.Sources = []SourceReference{{URL: "https://example.com", ArtifactID: uuid.New()}}
		}},
		{"missing exact account", func(c *Configuration) { c.Platforms[0].AccountID = "" }},
		{"unbounded publication count", func(c *Configuration) { c.Platforms[0].MaxDailyPublications = 0 }},
		{"unclassified capability", func(c *Configuration) { c.Platforms[0].Capabilities = c.Platforms[0].Capabilities[:6] }},
		{"invalid timezone", func(c *Configuration) { c.Timezone = "Mars/Base" }},
		{"missing escalation destination", func(c *Configuration) { c.TeamHandoff.Destination = "" }},
		{"multiple accounts on one connection", func(c *Configuration) {
			second := c.Platforms[0]
			second.AccountID = "another-account"
			c.Platforms = append(c.Platforms, second)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := base
			got.Sources = append([]SourceReference(nil), base.Sources...)
			got.Platforms = append([]PlatformBinding(nil), base.Platforms...)
			got.Platforms[0].Capabilities = append([]CapabilityRequirement(nil), base.Platforms[0].Capabilities...)
			tc.mutate(&got)
			if err := got.Validate(); err == nil {
				t.Fatal("Validate() succeeded")
			}
		})
	}
}

func TestConfigurationJSONContainsReferencesButNoCredentialFields(t *testing.T) {
	encoded, err := json.Marshal(validConfiguration("x-social", RequirementOptional))
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	for _, forbidden := range []string{"access_token", "refresh_token", "client_secret", "password"} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("serialized configuration contains credential field %q: %s", forbidden, value)
		}
	}
	for _, required := range []string{"research", "web-search", "connection_id", "account_id", "artifact_id"} {
		if !strings.Contains(value, required) {
			t.Fatalf("serialized configuration missing reference %q", required)
		}
	}
}

func TestPreflightUsesRestoredOperationsGrantsAccountAndDestination(t *testing.T) {
	cfg := validConfiguration("x-social", RequirementRequired)
	social := restoredConnection(t, cfg.Scope, "x-social", cfg.Platforms[0].ConnectionID, cfg.Platforms[0].AccountID)
	team := restoredConnection(t, cfg.Scope, "team-mail", cfg.TeamHandoff.ConnectionID)
	team.KnownDestinations = []string{cfg.TeamHandoff.Destination}
	report, err := Preflight(cfg, PreflightInput{
		Scope:                      cfg.Scope,
		PublishedEmployeeVersionID: cfg.EmployeeVersionID,
		ConfigurationRevision:      cfg.Revision,
		Connections:                []RestoredConnection{social, team},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready || !report.Platforms[0].Ready || !report.Handoff.Ready {
		t.Fatalf("unexpected preflight: %+v", report)
	}

	for i, operation := range social.GrantedOperations {
		if operation == "x-social.metrics.get" {
			social.GrantedOperations = append(social.GrantedOperations[:i], social.GrantedOperations[i+1:]...)
			break
		}
	}
	report, err = Preflight(cfg, PreflightInput{
		Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision,
		Connections: []RestoredConnection{social, team},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready {
		t.Fatal("missing required grant reported ready")
	}
	finding := report.Platforms[0].Finding(CapabilityMetrics)
	if finding.Status != CapabilityMissing || !strings.Contains(finding.Reason, "grant") {
		t.Fatalf("metrics finding = %+v", finding)
	}

	team.KnownDestinations = []string{"another@example.com"}
	report, err = Preflight(cfg, PreflightInput{
		Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision,
		Connections: []RestoredConnection{restoredConnection(t, cfg.Scope, "x-social", cfg.Platforms[0].ConnectionID, cfg.Platforms[0].AccountID), team},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Handoff.Ready || !strings.Contains(report.Handoff.Reason, "destination") {
		t.Fatalf("handoff = %+v", report.Handoff)
	}
}

func TestPreflightNeverReportsLinkedInCompanyInboxReady(t *testing.T) {
	for _, mode := range []RequirementMode{RequirementRequired, RequirementOptional} {
		cfg := validConfiguration("linkedin-social", mode)
		social := restoredConnection(t, cfg.Scope, "linkedin-social", cfg.Platforms[0].ConnectionID, cfg.Platforms[0].AccountID)
		team := restoredConnection(t, cfg.Scope, "team-mail", cfg.TeamHandoff.ConnectionID)
		team.KnownDestinations = []string{cfg.TeamHandoff.Destination}
		report, err := Preflight(cfg, PreflightInput{
			Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision,
			Connections: []RestoredConnection{social, team},
		})
		if err != nil {
			t.Fatal(err)
		}
		inbox := report.Platforms[0].Finding(CapabilityInbox)
		if inbox.Status == CapabilityReady || !strings.Contains(inbox.Reason, "partner API") {
			t.Fatalf("mode %q inbox = %+v", mode, inbox)
		}
		if got, want := report.Ready, mode == RequirementOptional; got != want {
			t.Fatalf("mode %q Ready = %v, want %v", mode, got, want)
		}
	}
}

func TestPreflightChecksEveryRequiredOperationAndImageField(t *testing.T) {
	cfg := validConfiguration("x-social", RequirementRequired)
	team := restoredConnection(t, cfg.Scope, "team-mail", cfg.TeamHandoff.ConnectionID)
	team.KnownDestinations = []string{cfg.TeamHandoff.Destination}
	for capability, suffixes := range capabilityOperations {
		for _, suffix := range suffixes {
			capability, operationID := capability, "x-social."+suffix
			t.Run(string(capability)+"/"+suffix, func(t *testing.T) {
				social := restoredConnection(t, cfg.Scope, "x-social", cfg.Platforms[0].ConnectionID, cfg.Platforms[0].AccountID)
				for i, operation := range social.AvailableOperations {
					if operation == operationID {
						social.AvailableOperations = append(social.AvailableOperations[:i], social.AvailableOperations[i+1:]...)
						break
					}
				}
				report, err := Preflight(cfg, PreflightInput{
					Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision,
					Connections: []RestoredConnection{social, team},
				})
				if err != nil {
					t.Fatal(err)
				}
				finding := report.Platforms[0].Finding(capability)
				if finding.Status == CapabilityReady || !containsExact(finding.MissingOperations, operationID) {
					t.Fatalf("finding = %+v", finding)
				}
			})
		}
	}

	t.Run("image field schema", func(t *testing.T) {
		social := restoredConnection(t, cfg.Scope, "x-social", cfg.Platforms[0].ConnectionID, cfg.Platforms[0].AccountID)
		for i := range social.Definition.Operations {
			operation := &social.Definition.Operations[i]
			if operation.ID != "x-social.post.create" {
				continue
			}
			properties := operation.InputSchema["properties"].(map[string]any)
			delete(properties, "image_data")
		}
		report, err := Preflight(cfg, PreflightInput{
			Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision,
			Connections: []RestoredConnection{social, team},
		})
		if err != nil {
			t.Fatal(err)
		}
		finding := report.Platforms[0].Finding(CapabilityImages)
		if finding.Status != CapabilityUnsupported || !containsExact(finding.MissingOperations, "x-social.post.create:image_data") {
			t.Fatalf("image finding = %+v", finding)
		}
	})
}

func TestPreflightRejectsCrossScopeOrStaleInputs(t *testing.T) {
	cfg := validConfiguration("x-social", RequirementOptional)
	valid := PreflightInput{Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision}
	tests := []struct {
		name   string
		mutate func(*PreflightInput)
	}{
		{"scope", func(in *PreflightInput) { in.Scope.OwnerID = uuid.New() }},
		{"employee version", func(in *PreflightInput) { in.PublishedEmployeeVersionID = uuid.New() }},
		{"configuration revision", func(in *PreflightInput) { in.ConfigurationRevision++ }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.mutate(&in)
			if _, err := Preflight(cfg, in); err == nil {
				t.Fatal("Preflight() succeeded")
			}
		})
	}

	t.Run("restored connection scope", func(t *testing.T) {
		social := restoredConnection(t, cfg.Scope, "x-social", cfg.Platforms[0].ConnectionID, cfg.Platforms[0].AccountID)
		social.Scope.OwnerID = uuid.New()
		if _, err := Preflight(cfg, PreflightInput{
			Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision,
			Connections: []RestoredConnection{social},
		}); err == nil {
			t.Fatal("Preflight() accepted a cross-scope restored connection")
		}
	})
}

func TestPreflightDoesNotTreatMissingOptionalPlatformAsReady(t *testing.T) {
	cfg := validConfiguration("x-social", RequirementOptional)
	for i := range cfg.Platforms[0].Capabilities {
		cfg.Platforms[0].Capabilities[i].Mode = RequirementOptional
	}
	team := restoredConnection(t, cfg.Scope, "team-mail", cfg.TeamHandoff.ConnectionID)
	team.KnownDestinations = []string{cfg.TeamHandoff.Destination}
	report, err := Preflight(cfg, PreflightInput{
		Scope: cfg.Scope, PublishedEmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision,
		Connections: []RestoredConnection{team},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || report.Platforms[0].Ready || !strings.Contains(report.Platforms[0].Reason, "not restored") {
		t.Fatalf("missing platform binding = %+v", report.Platforms[0])
	}
}
