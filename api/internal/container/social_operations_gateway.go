package container

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/socialmcp"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
)

type socialOperationsRuntimeResolver interface {
	ResolveDigitalHumanCapabilities(context.Context, runtimeauth.DigitalHumanCapabilityRequest) (runtimeauth.ChatCapabilitySnapshot, error)
	ResolveConnectorIdentity(context.Context, dispatch.IdentityResolutionRequest) (dispatch.Identity, error)
}
type socialOperationsDispatcher interface {
	Dispatch(context.Context, dispatch.Invocation) (execution.ExecutionResult, error)
}
type socialOperationsConnectionReader interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error)
}
type socialOperationsArtifactReader interface {
	ReadArtifact(context.Context, socialops.Scope, uuid.UUID) ([]byte, error)
}

type socialOperationsGateway struct {
	restored    ActiveBackgroundTaskRun
	resolver    socialOperationsRuntimeResolver
	dispatcher  socialOperationsDispatcher
	connections socialOperationsConnectionReader
	artifacts   socialOperationsArtifactReader
}

func (c *ServiceContainer) newSocialOperationsGateway(restored ActiveBackgroundTaskRun, store *socialops.Service) (socialops.PublicationGateway, error) {
	if c == nil || store == nil {
		return nil, socialGatewayError(core.ErrorCodeInternal)
	}
	return newSocialOperationsGatewayWithDependencies(restored, c.GetConnectorDigitalHumanRuntimeResolver(), c.GetConnectorSurfaceDispatcher(), c.GetConnectorConnectionService(), store)
}

func newSocialOperationsGatewayWithDependencies(restored ActiveBackgroundTaskRun, resolver socialOperationsRuntimeResolver, dispatcher socialOperationsDispatcher, connections socialOperationsConnectionReader, artifacts socialOperationsArtifactReader) (*socialOperationsGateway, error) {
	r := restored.Run
	if resolver == nil || dispatcher == nil || connections == nil || restored.OwnerID == uuid.Nil || restored.WorkspaceID == uuid.Nil ||
		r.ID == uuid.Nil || r.OrganizationID == uuid.Nil || r.WorkspaceID != restored.WorkspaceID || r.DigitalHumanID == uuid.Nil || r.DigitalHumanVersionID == uuid.Nil || r.ConversationID == nil || *r.ConversationID == uuid.Nil || r.MessageID == nil || *r.MessageID == uuid.Nil {
		return nil, socialGatewayError(core.ErrorCodeAccessDenied)
	}
	return &socialOperationsGateway{restored: restored, resolver: resolver, dispatcher: dispatcher, connections: connections, artifacts: artifacts}, nil
}

func (g *socialOperationsGateway) Send(ctx context.Context, p socialops.Publication, bundle socialops.DraftBundle) (socialops.RemotePost, error) {
	if err := g.validatePublication(p); err != nil {
		return socialops.RemotePost{}, &socialops.NotSubmittedError{Cause: err}
	}
	operation := ""
	input := map[string]any{}
	switch bundle.Action {
	case "post":
		operation, input["text"] = "post.create", bundle.English
	case "reply":
		operation, input["post_id"], input["text"] = "comment.create", bundle.TargetID, bundle.English
	case "quote":
		operation, input["post_id"], input["text"] = "post.quote", bundle.TargetID, bundle.English
	case "repost":
		operation, input["post_id"] = "post.repost", bundle.TargetID
	default:
		return socialops.RemotePost{}, &socialops.NotSubmittedError{Cause: socialGatewayError(core.ErrorCodeInvalidInput)}
	}
	if len(bundle.Images) > 1 || len(bundle.Images) == 1 && bundle.Action != "post" {
		return socialops.RemotePost{}, &socialops.NotSubmittedError{Cause: socialGatewayError(core.ErrorCodeInvalidInput)}
	}
	if len(bundle.Images) == 1 {
		if g.artifacts == nil {
			return socialops.RemotePost{}, &socialops.NotSubmittedError{Cause: socialGatewayError(core.ErrorCodeInternal)}
		}
		raw, err := g.artifacts.ReadArtifact(ctx, p.Scope, bundle.Images[0])
		if err != nil {
			return socialops.RemotePost{}, &socialops.NotSubmittedError{Cause: err}
		}
		image, err := socialmcp.PrepareImage(raw)
		if err != nil {
			return socialops.RemotePost{}, &socialops.NotSubmittedError{Cause: socialGatewayError(core.ErrorCodeInvalidInput)}
		}
		input["image_data"], input["mime_type"] = image.ImageData, image.MIMEType
	}
	value, err := g.invoke(ctx, p, operation, input, true)
	if err != nil {
		return socialops.RemotePost{}, err
	}
	remote, err := normalizeSocialReceipt(value, p, bundle)
	if err != nil {
		return remote, err
	}
	if len(bundle.Images) == 1 {
		if len(remote.Media) != 1 {
			return remote, socialGatewayError(core.ErrorCodeOutcomeUnknown)
		}
		local := bundle.Images[0]
		remote.Media[0].ArtifactID = &local
	}
	return remote, nil
}

func (g *socialOperationsGateway) Read(ctx context.Context, p socialops.Publication) (socialops.RemotePost, error) {
	if err := g.validatePublication(p); err != nil || strings.TrimSpace(p.RemoteID) == "" {
		if err != nil {
			return socialops.RemotePost{}, err
		}
		return socialops.RemotePost{}, socialGatewayError(core.ErrorCodeInvalidInput)
	}
	operation, key := "post.get", "post_id"
	if p.ParentRemoteID != "" {
		operation, key = "comment.get", "comment_id"
	}
	input := map[string]any{key: p.RemoteID}
	if p.Platform == "linkedin" && operation == "comment.get" {
		input["post_id"] = p.ParentRemoteID
	}
	value, err := g.invoke(ctx, p, operation, input, false)
	if err != nil {
		return socialops.RemotePost{}, err
	}
	accountID, err := g.accountID(ctx, p)
	if err != nil {
		return socialops.RemotePost{}, err
	}
	remote, author, err := normalizeSocialRead(value)
	if err != nil {
		return socialops.RemotePost{}, err
	}
	if remote.ID != p.RemoteID || author != accountID || remote.ParentID != p.ParentRemoteID {
		return socialops.RemotePost{}, socialGatewayError(core.ErrorCodeConflict)
	}
	return remote, nil
}

func (g *socialOperationsGateway) ValidateTarget(ctx context.Context, p socialops.Publication, bundle socialops.DraftBundle) error {
	if err := g.validatePublication(p); err != nil {
		return err
	}
	if bundle.TargetID == "" {
		return socialGatewayError(core.ErrorCodeInvalidInput)
	}
	value, err := g.invoke(ctx, p, "post.get", map[string]any{"post_id": bundle.TargetID}, false)
	if err != nil {
		return err
	}
	remote, author, err := normalizeSocialRead(value)
	if err != nil {
		return err
	}
	if remote.ID != bundle.TargetID || author == "" {
		return socialGatewayError(core.ErrorCodeConflict)
	}
	return nil
}

func (g *socialOperationsGateway) ObserveMetrics(ctx context.Context, p socialops.Publication) (map[string]*int64, error) {
	if err := g.validatePublication(p); err != nil || strings.TrimSpace(p.RemoteID) == "" {
		if err != nil {
			return nil, err
		}
		return nil, socialGatewayError(core.ErrorCodeInvalidInput)
	}
	value, err := g.invoke(ctx, p, "metrics.get", map[string]any{"post_id": p.RemoteID}, false)
	if err != nil {
		return nil, err
	}
	data, ok := value["data"].(map[string]any)
	if !ok {
		return nil, socialGatewayError(core.ErrorCodeProviderUnavailable)
	}
	accountID, err := g.accountID(ctx, p)
	if err != nil {
		return nil, err
	}
	metrics := map[string]*int64{}
	switch p.Platform {
	case "x":
		post, ok := data["data"].(map[string]any)
		if !ok || firstString(post, "id") != p.RemoteID || firstString(post, "author_id") != accountID {
			return nil, socialGatewayError(core.ErrorCodeConflict)
		}
		for _, group := range []string{"public_metrics", "non_public_metrics", "organic_metrics", "promoted_metrics"} {
			if counters, ok := post[group].(map[string]any); ok {
				appendMetricCounters(metrics, group, counters)
			}
		}
	case "facebook":
		if firstString(data, "id") != p.RemoteID || (!strings.HasPrefix(p.RemoteID, accountID+"_") && p.RemoteID != accountID) {
			return nil, socialGatewayError(core.ErrorCodeConflict)
		}
		if reactions, ok := data["reactions"].(map[string]any); ok {
			appendSummaryMetric(metrics, "reactions", reactions)
		}
		if comments, ok := data["comments"].(map[string]any); ok {
			appendSummaryMetric(metrics, "comments", comments)
		}
		if shares, ok := data["shares"].(map[string]any); ok {
			appendNamedMetric(metrics, "shares", shares["count"])
		}
		for _, raw := range anySlice(data["data"]) {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name := firstString(item, "name")
			values := anySlice(item["values"])
			if name == "" || len(values) != 1 {
				continue
			}
			sample, _ := values[0].(map[string]any)
			appendNamedMetric(metrics, name, sample["value"])
		}
	case "linkedin":
		if firstString(data, "post_id") != p.RemoteID || firstString(data, "account_id") != accountID {
			return nil, socialGatewayError(core.ErrorCodeConflict)
		}
		elements := anySlice(data["elements"])
		if len(elements) != 1 {
			return nil, socialGatewayError(core.ErrorCodeProviderUnavailable)
		}
		element, ok := elements[0].(map[string]any)
		if !ok {
			return nil, socialGatewayError(core.ErrorCodeProviderUnavailable)
		}
		if stats, ok := element["totalShareStatistics"].(map[string]any); ok {
			appendMetricCounters(metrics, "total_share_statistics", stats)
		}
	default:
		return nil, socialGatewayError(core.ErrorCodeInvalidInput)
	}
	return metrics, nil
}

func appendSummaryMetric(out map[string]*int64, name string, value map[string]any) {
	if summary, ok := value["summary"].(map[string]any); ok {
		appendNamedMetric(out, name, summary["total_count"])
	}
}
func appendMetricCounters(out map[string]*int64, prefix string, values map[string]any) {
	for name, value := range values {
		appendNamedMetric(out, prefix+"_"+name, value)
	}
}
func appendNamedMetric(out map[string]*int64, name string, value any) {
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	case float64:
		if v < 0 || v > 9007199254740991 || v != float64(int64(v)) {
			return
		}
		n = int64(v)
	case json.Number:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return
		}
		n = parsed
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return
		}
		n = parsed
	default:
		return
	}
	if n < 0 {
		return
	}
	copy := n
	out[name] = &copy
}
func anySlice(value any) []any { values, _ := value.([]any); return values }

func (g *socialOperationsGateway) validatePublication(p socialops.Publication) error {
	r := g.restored.Run
	if p.ID == uuid.Nil || p.ConnectionID == uuid.Nil || p.OrganizationID != r.OrganizationID || p.WorkspaceID != g.restored.WorkspaceID ||
		p.OwnerID != g.restored.OwnerID || p.EmployeeID != r.DigitalHumanID || connectorForPlatform(p.Platform) == "" {
		return socialGatewayError(core.ErrorCodeAccessDenied)
	}
	return nil
}

func connectorForPlatform(platform string) core.Identifier {
	switch platform {
	case "x":
		return "x-social"
	case "linkedin":
		return "linkedin-social"
	case "facebook":
		return "facebook-social"
	}
	return ""
}

func (g *socialOperationsGateway) invoke(ctx context.Context, p socialops.Publication, shortOperation string, input map[string]any, write bool) (map[string]any, error) {
	fail := func(err error) (map[string]any, error) {
		if write {
			return nil, &socialops.NotSubmittedError{Cause: err}
		}
		return nil, err
	}
	connector := connectorForPlatform(p.Platform)
	operation := core.Identifier(connector.String() + "." + shortOperation)
	request := runtimeauth.DigitalHumanCapabilityRequest{OrganizationID: p.OrganizationID, OwnerAccountID: p.OwnerID, DigitalHumanID: p.EmployeeID.String(), Scope: core.ScopePublished, VersionID: g.restored.Run.DigitalHumanVersionID.String()}
	target := core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: p.EmployeeID.String(), Scope: core.ScopePublished, VersionID: g.restored.Run.DigitalHumanVersionID.String()}
	snapshot, err := g.resolver.ResolveDigitalHumanCapabilities(ctx, request)
	if err != nil {
		return fail(err)
	}
	var found bool
	for _, capability := range snapshot.Capabilities {
		if capability.ConnectionID != p.ConnectionID || capability.OwnerAccountID != p.OwnerID || capability.ConnectorID != connector ||
			capability.Target.Type != target.Type || capability.Target.ID != target.ID || capability.Target.Scope != target.Scope || capability.Target.VersionID != target.VersionID {
			continue
		}
		for _, candidate := range capability.Operations {
			if candidate.ID == operation.String() {
				found = true
				break
			}
		}
	}
	if !found {
		return fail(socialGatewayError(core.ErrorCodeTargetNotReady))
	}
	connection, err := g.connections.GetConnection(ctx, p.OrganizationID, p.OwnerID, p.ConnectionID)
	if err != nil {
		return fail(err)
	}
	if connection.ID != p.ConnectionID || connection.OrganizationID != p.OrganizationID || connection.OwnerAccountID != p.OwnerID || connection.ConnectorID != connector {
		return fail(socialGatewayError(core.ErrorCodeAccessDenied))
	}
	if connection.ExternalAccountID == nil || strings.TrimSpace(*connection.ExternalAccountID) == "" {
		return fail(socialGatewayError(core.ErrorCodeConnectionNotReady))
	}
	if p.Platform == "linkedin" && shortOperation != "account.list" {
		accountID, accountErr := g.accountID(ctx, p)
		if accountErr != nil {
			return fail(accountErr)
		}
		input["account_id"] = accountID
	} else if shortOperation != "account.list" {
		verified := strings.TrimSpace(*connection.ExternalAccountID)
		if p.AccountID != "" && p.AccountID != verified {
			return fail(socialGatewayError(core.ErrorCodeConflict))
		}
		input["account_id"] = verified
	}
	logical := fmt.Sprintf("socialops:%s:%s", uuid.New(), shortOperation)
	if write {
		logical = fmt.Sprintf("socialops:%s:%d:%s", p.ID, p.Revision, shortOperation)
	}
	identity, err := g.resolver.ResolveConnectorIdentity(ctx, dispatch.IdentityResolutionRequest{ConversationID: g.restored.Run.ConversationID.String(), OrganizationID: p.OrganizationID,
		OwnerAccountID: p.OwnerID, ActingPrincipalID: p.OwnerID, ConnectorID: connector, Target: target,
		OperationID: operation, LogicalOperationID: logical, CorrelationID: logical, Interactive: false})
	if err != nil {
		return fail(err)
	}
	dispatchCtx := execution.WithBackgroundIdentity(ctx, g.restored.Run.ID, *g.restored.Run.MessageID)
	if write {
		dispatchCtx = withSocialPublicationClaim(dispatchCtx, p.ID, p.Revision, input)
	}
	result, err := g.dispatcher.Dispatch(dispatchCtx, dispatch.Invocation{Identity: identity, Input: input})
	if result.Evidence != nil && result.Evidence.Disposition == execution.DispatchNotSubmitted {
		if err == nil {
			err = socialGatewayError(core.ErrorCodeConflict)
		}
		return nil, &socialops.NotSubmittedError{Cause: err}
	}
	if err != nil {
		return nil, err
	}
	if result.State != execution.ExecutionStateCompleted {
		return nil, socialGatewayError(core.ErrorCodeOutcomeUnknown)
	}
	value, ok := result.Value.(map[string]any)
	if !ok || value["provider"] != connector.String() || value["transport"] != "mcp" {
		return nil, socialGatewayError(core.ErrorCodeProviderUnavailable)
	}
	return value, nil
}

func (g *socialOperationsGateway) accountID(ctx context.Context, p socialops.Publication) (string, error) {
	if p.Platform != "linkedin" {
		connection, err := g.connections.GetConnection(ctx, p.OrganizationID, p.OwnerID, p.ConnectionID)
		if err != nil {
			return "", err
		}
		if connection.ExternalAccountID == nil || strings.TrimSpace(*connection.ExternalAccountID) == "" {
			return "", socialGatewayError(core.ErrorCodeConnectionNotReady)
		}
		verified := strings.TrimSpace(*connection.ExternalAccountID)
		if p.AccountID != "" && p.AccountID != verified {
			return "", socialGatewayError(core.ErrorCodeConflict)
		}
		return verified, nil
	}
	desired := strings.TrimSpace(p.AccountID)
	if desired != "" && !strings.HasPrefix(desired, "urn:li:organization:") {
		allDigits := true
		for _, char := range desired {
			if char < '0' || char > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			desired = "urn:li:organization:" + desired
		}
	}
	cursor, only, count, found := "", "", 0, false
	for page := 0; page < 20; page++ {
		input := map[string]any{"limit": 50}
		if cursor != "" {
			input["cursor"] = cursor
		}
		value, err := g.invoke(ctx, p, "account.list", input, false)
		if err != nil {
			return "", err
		}
		data, ok := value["data"].(map[string]any)
		if !ok {
			return "", socialGatewayError(core.ErrorCodeProviderUnavailable)
		}
		accounts, ok := data["accounts"].([]any)
		if !ok {
			return "", socialGatewayError(core.ErrorCodeProviderUnavailable)
		}
		for _, raw := range accounts {
			account, ok := raw.(map[string]any)
			if !ok {
				return "", socialGatewayError(core.ErrorCodeProviderUnavailable)
			}
			id := firstString(account, "id")
			if id == "" {
				return "", socialGatewayError(core.ErrorCodeProviderUnavailable)
			}
			count++
			only = id
			if id == desired {
				found = true
			}
		}
		cursor = firstString(data, "cursor")
		if cursor == "" {
			if desired != "" && found {
				return desired, nil
			}
			if desired == "" && count == 1 {
				return only, nil
			}
			return "", socialGatewayError(core.ErrorCodeConflict)
		}
	}
	return "", socialGatewayError(core.ErrorCodeConflict)
}

func normalizeSocialReceipt(value map[string]any, p socialops.Publication, bundle socialops.DraftBundle) (socialops.RemotePost, error) {
	data, ok := value["data"].(map[string]any)
	if !ok {
		return socialops.RemotePost{}, socialGatewayError(core.ErrorCodeProviderUnavailable)
	}
	receipt := data
	if nested, nestedOK := data["data"].(map[string]any); nestedOK && firstString(nested, "id", "post_id") != "" {
		receipt = nested
	}
	id := firstString(receipt, "id", "post_id")
	if id == "" {
		return socialops.RemotePost{}, socialGatewayError(core.ErrorCodeOutcomeUnknown)
	}
	r := socialops.RemotePost{ID: id, URL: firstString(receipt, "url", "permalink_url"), Text: firstString(receipt, "text", "message", "commentary"), ParentID: firstString(data, "parent_id", "in_reply_to_id")}
	r.QuotedID, r.RepostedID = firstString(data, "quoted_id"), firstString(data, "reposted_id")
	if r.QuotedID == "" {
		r.QuotedID = firstString(receipt, "quoted_id")
	}
	if r.RepostedID == "" {
		r.RepostedID = firstString(receipt, "reposted_id")
	}
	if stamp := firstString(receipt, "published_at", "created_at", "created_time", "createdAt"); stamp != "" {
		r.PublishedAt, _ = time.Parse(time.RFC3339, stamp)
	}
	if receipts, ok := data["media_receipts"].([]any); ok {
		r.Media = normalizeMedia(receipts)
	}
	if r.ParentID != "" && bundle.Action == "reply" && r.ParentID != bundle.TargetID {
		return r, socialGatewayError(core.ErrorCodeConflict)
	}
	return r, nil
}

func normalizeSocialRead(value map[string]any) (socialops.RemotePost, string, error) {
	data, ok := value["data"].(map[string]any)
	if !ok {
		return socialops.RemotePost{}, "", socialGatewayError(core.ErrorCodeProviderUnavailable)
	}
	r := socialops.RemotePost{ID: firstString(data, "id", "post_id"), URL: firstString(data, "url", "permalink_url"), Text: firstString(data, "text", "message", "commentary"), ParentID: firstString(data, "parent_post_id", "parent_id", "in_reply_to_id")}
	r.QuotedID, r.RepostedID = firstString(data, "quoted_id"), firstString(data, "reposted_id")
	stamp := firstString(data, "published_at", "created_at", "created_time", "createdAt")
	parsed, err := time.Parse(time.RFC3339, stamp)
	author := firstString(data, "author_id", "author")
	if from, ok := data["from"].(map[string]any); ok && author == "" {
		author = firstString(from, "id")
	}
	if media, ok := data["media"].([]any); ok {
		r.Media = normalizeMedia(media)
	}
	if r.ID == "" || r.URL == "" || author == "" || err != nil {
		return socialops.RemotePost{}, "", socialGatewayError(core.ErrorCodeProviderUnavailable)
	}
	r.PublishedAt = parsed.UTC()
	return r, author, nil
}

func normalizeMedia(values []any) []socialops.RemoteMedia {
	result := make([]socialops.RemoteMedia, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id, url := firstString(item, "id"), firstString(item, "url")
		if id != "" {
			result = append(result, socialops.RemoteMedia{ID: id, URL: url})
		}
	}
	return result
}
func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func socialGatewayError(code core.ErrorCode) error { return core.NewError(code, false, nil) }

var _ socialops.PublicationGateway = (*socialOperationsGateway)(nil)
var _ socialops.TargetValidator = (*socialOperationsGateway)(nil)
var _ socialops.MetricsGateway = (*socialOperationsGateway)(nil)
