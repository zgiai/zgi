package socialops

import (
	"fmt"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

// Configuration is the secret-free, persisted input to the social operations
// SOP. Connection credentials and grants are restored separately by the
// server during preflight and execution.
type Configuration struct {
	Scope                  Scope                `json:"scope"`
	EmployeeVersionID      uuid.UUID            `json:"employee_version_id"`
	Revision               int64                `json:"revision"`
	Product                ProductConfiguration `json:"product"`
	Audience               string               `json:"audience"`
	PublicationLanguage    string               `json:"publication_language"`
	InternalReviewLanguage string               `json:"internal_review_language"`
	Sources                []SourceReference    `json:"sources"`
	BrandArtifactRefs      []uuid.UUID          `json:"brand_artifact_refs,omitempty"`
	Research               ResearchBinding      `json:"research"`
	Platforms              []PlatformBinding    `json:"platforms"`
	Timezone               string               `json:"timezone"`
	Cadence                Cadence              `json:"cadence"`
	TeamHandoff            TeamHandoff          `json:"team_handoff"`
}

type ProductConfiguration struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ResearchBinding pins public research to one exact published Connector
// connection. Live operations and grants are restored separately by preflight.
type ResearchBinding struct {
	ConnectorID  string    `json:"connector_id"`
	ConnectionID uuid.UUID `json:"connection_id"`
}

// SourceReference names exactly one public HTTPS source or existing scoped
// local artifact. It never contains source credentials.
type SourceReference struct {
	URL        string    `json:"url,omitempty"`
	ArtifactID uuid.UUID `json:"artifact_id,omitempty"`
}

type PlatformBinding struct {
	Platform             string                  `json:"platform"`
	ConnectorID          string                  `json:"connector_id"`
	ConnectionID         uuid.UUID               `json:"connection_id"`
	AccountID            string                  `json:"account_id"`
	MaxDailyPublications int                     `json:"max_daily_publications"`
	Capabilities         []CapabilityRequirement `json:"capabilities"`
}

type Capability string

const (
	CapabilityAccount  Capability = "account"
	CapabilityPost     Capability = "post"
	CapabilityReadback Capability = "readback"
	CapabilityComment  Capability = "comment"
	CapabilityInbox    Capability = "inbox"
	CapabilityMetrics  Capability = "metrics"
	CapabilityImages   Capability = "images"
)

var allCapabilities = []Capability{
	CapabilityAccount,
	CapabilityPost,
	CapabilityReadback,
	CapabilityComment,
	CapabilityInbox,
	CapabilityMetrics,
	CapabilityImages,
}

type RequirementMode string

const (
	RequirementRequired RequirementMode = "required"
	RequirementOptional RequirementMode = "optional"
)

type CapabilityRequirement struct {
	Capability Capability      `json:"capability"`
	Mode       RequirementMode `json:"mode"`
}

type Cadence struct {
	DailyContentCron   string `json:"daily_content_cron"`
	InboxCron          string `json:"inbox_cron"`
	SevenDaySampleCron string `json:"seven_day_sample_cron"`
	WeeklyReviewCron   string `json:"weekly_review_cron"`
}

type TeamHandoff struct {
	ConnectorID  string    `json:"connector_id"`
	ConnectionID uuid.UUID `json:"connection_id"`
	OperationID  string    `json:"operation_id"`
	Destination  string    `json:"destination"`
}

func (c Configuration) Validate() error {
	if !validScope(c.Scope) || c.EmployeeVersionID == uuid.Nil || c.Revision < 1 {
		return fmt.Errorf("configuration scope, employee version and revision are required")
	}
	if err := boundedText("product name", c.Product.Name, 256); err != nil {
		return err
	}
	if err := boundedText("product description", c.Product.Description, 4000); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"audience", c.Audience},
		{"publication language", c.PublicationLanguage},
		{"internal review language", c.InternalReviewLanguage},
	} {
		limit := 2000
		if strings.Contains(field.name, "language") {
			limit = 64
		}
		if err := boundedText(field.name, field.value, limit); err != nil {
			return err
		}
	}
	if len(c.Sources) == 0 || len(c.Sources) > 100 {
		return fmt.Errorf("configuration requires 1 to 100 sources")
	}
	seenSources := map[string]bool{}
	for _, source := range c.Sources {
		key, err := validateSource(source)
		if err != nil {
			return err
		}
		if seenSources[key] {
			return fmt.Errorf("duplicate source %q", key)
		}
		seenSources[key] = true
	}
	seenArtifacts := map[uuid.UUID]bool{}
	for _, id := range c.BrandArtifactRefs {
		if id == uuid.Nil || seenArtifacts[id] {
			return fmt.Errorf("brand artifact references must be non-zero and unique")
		}
		seenArtifacts[id] = true
	}
	if err := validateIdentifier("research connector", c.Research.ConnectorID); err != nil {
		return err
	}
	if c.Research.ConnectionID == uuid.Nil {
		return fmt.Errorf("research connection is required")
	}
	if len(c.Platforms) == 0 || len(c.Platforms) > 10 {
		return fmt.Errorf("configuration requires 1 to 10 platform bindings")
	}
	seenBindings := map[string]bool{}
	seenConnections := map[string]bool{}
	for _, platform := range c.Platforms {
		if err := validatePlatform(platform); err != nil {
			return err
		}
		key := platform.ConnectorID + "/" + platform.ConnectionID.String() + "/" + platform.AccountID
		if seenBindings[key] {
			return fmt.Errorf("duplicate platform binding %q", key)
		}
		seenBindings[key] = true
		connectionKey := platform.ConnectorID + "/" + platform.ConnectionID.String()
		if seenConnections[connectionKey] {
			return fmt.Errorf("one social operations account is allowed per connector connection %q", connectionKey)
		}
		seenConnections[connectionKey] = true
	}
	if strings.TrimSpace(c.Timezone) != c.Timezone || c.Timezone == "" {
		return fmt.Errorf("timezone is required")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", c.Timezone, err)
	}
	for _, schedule := range []struct {
		name       string
		expression string
	}{
		{"daily content", c.Cadence.DailyContentCron},
		{"inbox", c.Cadence.InboxCron},
		{"seven day sample", c.Cadence.SevenDaySampleCron},
		{"weekly review", c.Cadence.WeeklyReviewCron},
	} {
		if strings.TrimSpace(schedule.expression) != schedule.expression || schedule.expression == "" {
			return fmt.Errorf("%s cadence is required", schedule.name)
		}
		if _, err := cron.ParseStandard("CRON_TZ=" + c.Timezone + " " + schedule.expression); err != nil {
			return fmt.Errorf("invalid %s cadence: %w", schedule.name, err)
		}
	}
	if err := validateIdentifier("handoff connector", c.TeamHandoff.ConnectorID); err != nil {
		return err
	}
	if c.TeamHandoff.ConnectionID == uuid.Nil {
		return fmt.Errorf("handoff connection is required")
	}
	if err := validateIdentifier("handoff operation", c.TeamHandoff.OperationID); err != nil {
		return err
	}
	if !strings.HasPrefix(c.TeamHandoff.OperationID, c.TeamHandoff.ConnectorID+".") {
		return fmt.Errorf("handoff operation does not belong to connector")
	}
	if err := boundedText("handoff destination", c.TeamHandoff.Destination, 512); err != nil {
		return err
	}
	if _, ok := catalog.MailDefinition(c.TeamHandoff.ConnectorID); ok {
		address, err := mail.ParseAddress(c.TeamHandoff.Destination)
		if err != nil || address.Address != c.TeamHandoff.Destination {
			return fmt.Errorf("mail handoff destination must be one exact email address")
		}
	}
	return nil
}

func boundedText(name, value string, maximum int) error {
	if strings.TrimSpace(value) != value || value == "" || len(value) > maximum || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must be a non-empty bounded value", name)
	}
	return nil
}

func validateIdentifier(name, value string) error {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is invalid", name)
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	return nil
}

func validateSource(source SourceReference) (string, error) {
	hasURL, hasArtifact := source.URL != "", source.ArtifactID != uuid.Nil
	if hasURL == hasArtifact {
		return "", fmt.Errorf("source must contain exactly one URL or artifact reference")
	}
	if hasArtifact {
		return "artifact:" + source.ArtifactID.String(), nil
	}
	if len(source.URL) > 2048 || strings.TrimSpace(source.URL) != source.URL {
		return "", fmt.Errorf("source URL is invalid")
	}
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("source URL must be an absolute credential-free HTTPS URL")
	}
	return parsed.String(), nil
}

func validatePlatform(platform PlatformBinding) error {
	if err := validateIdentifier("platform", platform.Platform); err != nil {
		return err
	}
	if err := validateIdentifier("platform connector", platform.ConnectorID); err != nil {
		return err
	}
	if !catalog.IsSocialMCPConnector(platform.ConnectorID) || platform.Platform != strings.TrimSuffix(platform.ConnectorID, "-social") {
		return fmt.Errorf("platform and social connector do not match")
	}
	if platform.ConnectionID == uuid.Nil {
		return fmt.Errorf("platform connection is required")
	}
	if err := boundedText("platform account", platform.AccountID, 256); err != nil {
		return err
	}
	if platform.MaxDailyPublications < 1 || platform.MaxDailyPublications > 100 {
		return fmt.Errorf("platform max daily publications must be between 1 and 100")
	}
	modes := make(map[Capability]RequirementMode, len(platform.Capabilities))
	for _, requirement := range platform.Capabilities {
		if requirement.Mode != RequirementRequired && requirement.Mode != RequirementOptional {
			return fmt.Errorf("capability %q has invalid requirement mode", requirement.Capability)
		}
		if _, exists := modes[requirement.Capability]; exists {
			return fmt.Errorf("capability %q is duplicated", requirement.Capability)
		}
		modes[requirement.Capability] = requirement.Mode
	}
	for _, capability := range allCapabilities {
		if _, exists := modes[capability]; !exists {
			return fmt.Errorf("capability %q must be classified as required or optional", capability)
		}
	}
	if len(modes) != len(allCapabilities) {
		return fmt.Errorf("unknown capability configured")
	}
	return nil
}

// RestoredConnection contains only server-restored state: the admitted
// definition, the current connection accounts, provider/runtime availability,
// and the current background grant allowlist. Preflight performs no network IO.
type RestoredConnection struct {
	Scope               Scope
	ConnectorID         string
	ConnectionID        uuid.UUID
	Definition          catalog.Definition
	AccountIDs          []string
	AvailableOperations []string
	GrantedOperations   []string
	KnownDestinations   []string
	OperationReasons    map[string]string
}

type PreflightInput struct {
	Scope                      Scope
	PublishedEmployeeVersionID uuid.UUID
	ConfigurationRevision      int64
	Connections                []RestoredConnection
	Prerequisites              []PrerequisiteFinding
}

type PrerequisiteFinding struct {
	Key    string `json:"key"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason"`
}

type CapabilityStatus string

const (
	CapabilityReady           CapabilityStatus = "ready"
	CapabilityMissing         CapabilityStatus = "missing"
	CapabilityUnsupported     CapabilityStatus = "unsupported"
	CapabilityOptionalSkipped CapabilityStatus = "optional_skipped"
)

type CapabilityFinding struct {
	Capability         Capability       `json:"capability"`
	Status             CapabilityStatus `json:"status"`
	Required           bool             `json:"required"`
	RequiredOperations []string         `json:"required_operations"`
	MissingOperations  []string         `json:"missing_operations,omitempty"`
	Reason             string           `json:"reason"`
}

type PlatformPreflight struct {
	Platform     string              `json:"platform"`
	ConnectorID  string              `json:"connector_id"`
	ConnectionID uuid.UUID           `json:"connection_id"`
	AccountID    string              `json:"account_id"`
	Ready        bool                `json:"ready"`
	Reason       string              `json:"reason"`
	Findings     []CapabilityFinding `json:"findings"`
}

func (p PlatformPreflight) Finding(capability Capability) CapabilityFinding {
	for _, finding := range p.Findings {
		if finding.Capability == capability {
			return finding
		}
	}
	return CapabilityFinding{Capability: capability}
}

type HandoffPreflight struct {
	ConnectorID  string    `json:"connector_id"`
	ConnectionID uuid.UUID `json:"connection_id"`
	OperationID  string    `json:"operation_id"`
	Destination  string    `json:"destination"`
	Ready        bool      `json:"ready"`
	Reason       string    `json:"reason"`
}

type PreflightReport struct {
	Scope                      Scope                 `json:"scope"`
	PublishedEmployeeVersionID uuid.UUID             `json:"published_employee_version_id"`
	ConfigurationRevision      int64                 `json:"configuration_revision"`
	Ready                      bool                  `json:"ready"`
	Platforms                  []PlatformPreflight   `json:"platforms"`
	Handoff                    HandoffPreflight      `json:"handoff"`
	Prerequisites              []PrerequisiteFinding `json:"prerequisites,omitempty"`
}

var capabilityOperations = map[Capability][]string{
	CapabilityAccount:  {"account.get", "account.list"},
	CapabilityPost:     {"post.create"},
	CapabilityReadback: {"post.get"},
	CapabilityComment:  {"comment.list", "comment.get", "comment.create", "comment.reply"},
	CapabilityInbox:    {"conversation.list", "message.list", "message.send"},
	CapabilityMetrics:  {"metrics.get"},
	CapabilityImages:   {"post.create"},
}

func Preflight(configuration Configuration, input PreflightInput) (PreflightReport, error) {
	if err := configuration.Validate(); err != nil {
		return PreflightReport{}, err
	}
	if input.Scope != configuration.Scope || input.PublishedEmployeeVersionID != configuration.EmployeeVersionID || input.ConfigurationRevision != configuration.Revision {
		return PreflightReport{}, fmt.Errorf("preflight scope, employee version or configuration revision does not match")
	}
	connections := map[string]RestoredConnection{}
	for _, connection := range input.Connections {
		if connection.Scope != input.Scope || connection.ConnectionID == uuid.Nil || connection.ConnectorID == "" || connection.Definition.ID != connection.ConnectorID {
			return PreflightReport{}, fmt.Errorf("restored connection identity or scope is invalid")
		}
		key := connection.ConnectorID + "/" + connection.ConnectionID.String()
		if _, exists := connections[key]; exists {
			return PreflightReport{}, fmt.Errorf("duplicate restored connection %q", key)
		}
		connections[key] = connection
	}
	report := PreflightReport{
		Scope: input.Scope, PublishedEmployeeVersionID: input.PublishedEmployeeVersionID,
		ConfigurationRevision: input.ConfigurationRevision, Ready: true,
		Prerequisites: append([]PrerequisiteFinding(nil), input.Prerequisites...),
	}
	seenPrerequisites := map[string]bool{}
	for _, prerequisite := range report.Prerequisites {
		if prerequisite.Key == "" || seenPrerequisites[prerequisite.Key] || strings.TrimSpace(prerequisite.Reason) == "" {
			return PreflightReport{}, fmt.Errorf("preflight prerequisite is invalid")
		}
		seenPrerequisites[prerequisite.Key] = true
		if !prerequisite.Ready {
			report.Ready = false
		}
	}
	for _, binding := range configuration.Platforms {
		key := binding.ConnectorID + "/" + binding.ConnectionID.String()
		connection, found := connections[key]
		platform := PlatformPreflight{
			Platform: binding.Platform, ConnectorID: binding.ConnectorID,
			ConnectionID: binding.ConnectionID, AccountID: binding.AccountID, Ready: true,
		}
		switch {
		case !found:
			platform.Ready = false
			platform.Reason = "configured connection was not restored by the server"
			report.Ready = false
		case !containsExact(connection.AccountIDs, binding.AccountID):
			platform.Ready = false
			platform.Reason = "configured account is not present on the restored connection"
			report.Ready = false
		case !connection.Definition.Supports(catalog.SurfaceDigitalHuman):
			platform.Ready = false
			platform.Reason = "restored connector contract is unavailable to the digital human"
			report.Ready = false
		default:
			platform.Reason = "configured connection and account are restored"
		}
		modes := map[Capability]RequirementMode{}
		for _, requirement := range binding.Capabilities {
			modes[requirement.Capability] = requirement.Mode
		}
		for _, capability := range allCapabilities {
			requirement := CapabilityRequirement{Capability: capability, Mode: modes[capability]}
			finding := evaluateCapability(binding, requirement, connection, found)
			platform.Findings = append(platform.Findings, finding)
			if requirement.Mode == RequirementRequired && finding.Status != CapabilityReady {
				platform.Ready = false
				report.Ready = false
			}
		}
		report.Platforms = append(report.Platforms, platform)
	}
	report.Handoff = evaluateHandoff(configuration.TeamHandoff, connections)
	if !report.Handoff.Ready {
		report.Ready = false
	}
	return report, nil
}

func evaluateCapability(binding PlatformBinding, requirement CapabilityRequirement, connection RestoredConnection, found bool) CapabilityFinding {
	required := requirement.Mode == RequirementRequired
	suffixes := capabilityOperationSuffixes(binding.ConnectorID, requirement.Capability)
	operations := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		operations = append(operations, binding.ConnectorID+"."+suffix)
	}
	finding := CapabilityFinding{Capability: requirement.Capability, Required: required, RequiredOperations: operations}
	finishMissing := func(status CapabilityStatus, reason string, missing []string) CapabilityFinding {
		if !required {
			status = CapabilityOptionalSkipped
		}
		finding.Status, finding.Reason, finding.MissingOperations = status, reason, missing
		return finding
	}
	if binding.ConnectorID == "linkedin-social" && requirement.Capability == CapabilityInbox {
		return finishMissing(CapabilityUnsupported, "LinkedIn company messaging requires an approved partner API and is unavailable", operations)
	}
	if !found {
		return finishMissing(CapabilityMissing, "configured connection was not restored by the server", operations)
	}
	if !connection.Definition.Supports(catalog.SurfaceDigitalHuman) {
		return finishMissing(CapabilityUnsupported, "connector contract is unavailable to the digital human", operations)
	}
	if !containsExact(connection.AccountIDs, binding.AccountID) {
		return finishMissing(CapabilityMissing, "configured account is not present on the restored connection", operations)
	}
	defined := operationSet(connection.Definition.Operations)
	available := stringSet(connection.AvailableOperations)
	granted := stringSet(connection.GrantedOperations)
	var unsupported, unavailable, ungranted []string
	for _, operation := range operations {
		switch {
		case !defined[operation]:
			unsupported = append(unsupported, operation)
		case !available[operation]:
			unavailable = append(unavailable, operation)
		case !granted[operation]:
			ungranted = append(ungranted, operation)
		}
	}
	if requirement.Capability == CapabilityImages && defined[binding.ConnectorID+".post.create"] && !operationHasInputProperty(connection.Definition.Operations, binding.ConnectorID+".post.create", "image_data") {
		unsupported = append(unsupported, binding.ConnectorID+".post.create:image_data")
	}
	if len(unsupported) > 0 {
		sort.Strings(unsupported)
		return finishMissing(CapabilityUnsupported, "connector definition does not support required operations or fields", unsupported)
	}
	if len(unavailable) > 0 {
		sort.Strings(unavailable)
		return finishMissing(CapabilityMissing, readinessReason("operation is not currently available", unavailable, connection.OperationReasons), unavailable)
	}
	if len(ungranted) > 0 {
		sort.Strings(ungranted)
		return finishMissing(CapabilityMissing, "background grant does not allow all required operations", ungranted)
	}
	finding.Status = CapabilityReady
	if required {
		finding.Reason = "all required operations are available and granted"
	} else {
		finding.Reason = "optional capability is available and granted"
	}
	return finding
}

func capabilityOperationSuffixes(connectorID string, capability Capability) []string {
	operations := append([]string(nil), capabilityOperations[capability]...)
	if capability != CapabilityInbox {
		return operations
	}
	switch connectorID {
	case "x-social", "facebook-social":
		return append(operations, "mention.list")
	case "linkedin-social":
		return append(operations, "notification.list")
	default:
		return operations
	}
}

func evaluateHandoff(handoff TeamHandoff, connections map[string]RestoredConnection) HandoffPreflight {
	result := HandoffPreflight{
		ConnectorID: handoff.ConnectorID, ConnectionID: handoff.ConnectionID,
		OperationID: handoff.OperationID, Destination: handoff.Destination,
	}
	connection, found := connections[handoff.ConnectorID+"/"+handoff.ConnectionID.String()]
	if !found {
		result.Reason = "configured handoff connection was not restored by the server"
		return result
	}
	if !connection.Definition.Supports(catalog.SurfaceDigitalHuman) {
		result.Reason = "handoff connector contract is unavailable to the digital human"
		return result
	}
	operation, supported := findOperation(connection.Definition.Operations, handoff.OperationID)
	if !supported {
		result.Reason = "handoff operation is unsupported by the connector definition"
		return result
	}
	if operation.Effect != catalog.OperationEffectExternalSend {
		result.Reason = "handoff operation is not an external send operation"
		return result
	}
	if !stringSet(connection.AvailableOperations)[handoff.OperationID] {
		result.Reason = readinessReason("handoff operation is not currently available", []string{handoff.OperationID}, connection.OperationReasons)
		return result
	}
	if !stringSet(connection.GrantedOperations)[handoff.OperationID] {
		result.Reason = "background grant does not allow the handoff operation"
		return result
	}
	if !containsExact(connection.KnownDestinations, handoff.Destination) {
		result.Reason = "background grant does not pin the configured handoff destination"
		return result
	}
	result.Ready = true
	result.Reason = "operation, grant and destination are ready"
	return result
}

func operationSet(operations []catalog.Operation) map[string]bool {
	set := make(map[string]bool, len(operations))
	for _, operation := range operations {
		set[operation.ID] = true
	}
	return set
}

func findOperation(operations []catalog.Operation, operationID string) (catalog.Operation, bool) {
	for _, operation := range operations {
		if operation.ID == operationID {
			return operation, true
		}
	}
	return catalog.Operation{}, false
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func operationHasInputProperty(operations []catalog.Operation, operationID, property string) bool {
	for _, operation := range operations {
		if operation.ID != operationID {
			continue
		}
		properties, ok := operation.InputSchema["properties"].(map[string]any)
		return ok && properties[property] != nil
	}
	return false
}

func readinessReason(fallback string, operations []string, reasons map[string]string) string {
	for _, operation := range operations {
		if reason := strings.TrimSpace(reasons[operation]); reason != "" {
			return fallback + ": " + reason
		}
	}
	return fallback
}
