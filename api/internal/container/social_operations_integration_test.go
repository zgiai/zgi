package container

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type socialIntegrationDispatcher struct {
	service        *socialops.Service
	scope          socialops.Scope
	publication    map[string]uuid.UUID
	media          map[string]string
	calls          []connectordispatch.Invocation
	readFailures   map[string]int
	rejectFailures map[string]int
	sawReceipt     bool
}

func (d *socialIntegrationDispatcher) Dispatch(ctx context.Context, in connectordispatch.Invocation) (execution.ExecutionResult, error) {
	d.calls = append(d.calls, in)
	op := in.Identity.OperationID.String()
	input, _ := in.Input.(map[string]any)
	completed := func(data map[string]any) (execution.ExecutionResult, error) {
		return execution.ExecutionResult{State: execution.ExecutionStateCompleted, Value: map[string]any{"provider": "x-social", "transport": "mcp", "data": data}}, nil
	}
	switch op {
	case "x-social.post.create":
		text, _ := input["text"].(string)
		if d.rejectFailures[text] > 0 {
			d.rejectFailures[text]--
			return execution.ExecutionResult{State: execution.ExecutionStateRejected, Evidence: &execution.DispatchEvidence{Version: 1, Disposition: execution.DispatchNotSubmitted}}, connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil)
		}
		if text == "unknown" {
			return execution.ExecutionResult{State: execution.ExecutionStateUnresolved, Evidence: &execution.DispatchEvidence{Version: 1, Disposition: execution.DispatchUnknown}}, connectorcore.NewError(connectorcore.ErrorCodeOutcomeUnknown, false, nil)
		}
		id := "remote-" + text
		data := map[string]any{"data": map[string]any{"id": id, "text": text}}
		if _, hasImage := input["image_data"]; hasImage {
			mediaID := "media-" + id
			if text == "main" {
				mediaID = "media-main"
			}
			d.media[id] = mediaID
			data["media_receipts"] = []any{map[string]any{"id": mediaID}}
		}
		return completed(data)
	case "x-social.post.get":
		id, _ := input["post_id"].(string)
		if publicationID := d.publication[id]; publicationID != uuid.Nil {
			p, err := d.service.GetPublication(ctx, d.scope, publicationID)
			if err == nil && p.State == "remote_created" && p.RemoteID == id {
				d.sawReceipt = true
			}
		}
		text := strings.TrimPrefix(id, "remote-")
		if d.readFailures[id] > 0 {
			d.readFailures[id]--
			text = "mismatched"
		}
		data := map[string]any{"id": id, "text": text, "author_id": "acct-7", "url": "https://x.com/acct/status/" + id, "published_at": "2026-10-20T00:00:00Z"}
		if mediaID := d.media[id]; mediaID != "" {
			data["media"] = []any{map[string]any{"id": mediaID, "url": "https://pbs.twimg.com/media/fixture.jpg"}}
		}
		return completed(data)
	case "x-social.comment.create":
		return completed(map[string]any{"data": map[string]any{"id": "comment-supplement", "text": "supplement"}, "parent_id": input["post_id"]})
	case "x-social.comment.get":
		return completed(map[string]any{"id": "comment-supplement", "text": "supplement", "author_id": "acct-7", "url": "https://x.com/acct/status/comment-supplement", "published_at": "2026-10-20T00:01:00Z", "parent_id": "remote-main"})
	case "x-social.metrics.get":
		id, _ := input["post_id"].(string)
		return completed(map[string]any{"data": map[string]any{"id": id, "author_id": "acct-7", "public_metrics": map[string]any{"like_count": float64(9)}}})
	case "x-social.message.send":
		text, _ := input["text"].(string)
		if d.rejectFailures[text] > 0 {
			d.rejectFailures[text]--
			return execution.ExecutionResult{State: execution.ExecutionStateRejected, Evidence: &execution.DispatchEvidence{Version: 1, Disposition: execution.DispatchNotSubmitted}}, connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil)
		}
		if text == "unknown-dm" {
			return execution.ExecutionResult{State: execution.ExecutionStateUnresolved, Evidence: &execution.DispatchEvidence{Version: 1, Disposition: execution.DispatchUnknown}}, connectorcore.NewError(connectorcore.ErrorCodeOutcomeUnknown, false, nil)
		}
		return completed(map[string]any{"data": map[string]any{"dm_event_id": "dm-receipt-1"}})
	default:
		return execution.ExecutionResult{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
}

type socialIntegrationFixture struct {
	db         *gorm.DB
	service    *socialops.Service
	scope      socialops.Scope
	now        *time.Time
	gateway    *socialOperationsGateway
	dispatcher *socialIntegrationDispatcher
	connection uuid.UUID
}

func newSocialIntegrationFixture(t *testing.T) *socialIntegrationFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err = db.AutoMigrate(&socialops.Artifact{}, &socialops.Record{}, &socialops.Publication{}, &socialops.Metric{}, &socialops.Outbound{}, &socialops.ImageInspection{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC)
	service, err := socialops.New(db, t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	scope := socialops.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerID: uuid.New(), EmployeeID: uuid.New()}
	version, conversation, connection, binding := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	operations := []string{"post.create", "post.get", "comment.create", "comment.get", "metrics.get", "message.send"}
	catalogOps := make([]catalog.Operation, len(operations))
	for i, op := range operations {
		catalogOps[i].ID = "x-social." + op
	}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeDigitalHuman, ID: scope.EmployeeID.String(), Scope: connectorcore.ScopePublished, VersionID: version.String()}
	capability := runtimeauth.ChatCapability{Target: target, BindingID: binding, ConnectionID: connection, OwnerAccountID: scope.OwnerID, ConnectorID: "x-social", Operations: catalogOps}
	dispatcher := &socialIntegrationDispatcher{service: service, scope: scope, publication: map[string]uuid.UUID{}, media: map[string]string{}, readFailures: map[string]int{}, rejectFailures: map[string]int{}}
	message := uuid.New()
	restored := ActiveBackgroundTaskRun{Run: digitalhuman.Run{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: scope.EmployeeID, DigitalHumanVersionID: version, ConversationID: &conversation, MessageID: &message}, WorkspaceID: scope.WorkspaceID, OwnerID: scope.OwnerID}
	gateway, err := newSocialOperationsGatewayWithDependencies(restored, socialGatewayResolverStub{capability}, dispatcher, socialGatewayConnectionStub{account: "acct-7", id: connection, organization: scope.OrganizationID, owner: scope.OwnerID, connector: "x-social"}, service)
	if err != nil {
		t.Fatal(err)
	}
	return &socialIntegrationFixture{db: db, service: service, scope: scope, now: &now, gateway: gateway, dispatcher: dispatcher, connection: connection}
}

func (f *socialIntegrationFixture) prepare(t *testing.T, key, action, english, target string, images []uuid.UUID, parent *uuid.UUID) socialops.Publication {
	t.Helper()
	bundle := socialops.DraftBundle{Action: action, English: english, Chinese: "中文审核", TargetID: target, Images: images}
	raw, _ := json.Marshal(bundle)
	a, err := f.service.PutArtifact(t.Context(), f.scope, "application/json", raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.service.PreparePublication(t.Context(), socialops.PublicationRequest{Scope: f.scope, ConnectionID: f.connection, ContentKey: key, Platform: "x", DraftArtifactID: a.ID, ParentPublicationID: parent})
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.service.ReviewPublication(t.Context(), f.scope, p.ID, p.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSocialOperationsNativeGatewayPublicationComposition(t *testing.T) {
	f := newSocialIntegrationFixture(t)
	ctx := t.Context()
	var pngBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	imageArtifact, err := f.service.PutArtifact(ctx, f.scope, "image/png", pngBytes.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	main := f.prepare(t, "main", "post", "main", "", []uuid.UUID{imageArtifact.ID}, nil)
	f.dispatcher.publication["remote-main"] = main.ID
	main, err = f.service.Publish(ctx, f.scope, main.ID, f.gateway, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if main.State != "archived" || main.RemoteID != "remote-main" || !f.dispatcher.sawReceipt {
		t.Fatalf("main = %#v, saw receipt=%v", main, f.dispatcher.sawReceipt)
	}
	var media []socialops.RemoteMedia
	if json.Unmarshal([]byte(main.MediaJSON), &media) != nil || len(media) != 1 || media[0].ID != "media-main" || media[0].ArtifactID == nil || *media[0].ArtifactID != imageArtifact.ID {
		t.Fatalf("media = %#v", media)
	}
	if got, err := f.service.ReadArtifact(ctx, f.scope, *media[0].ArtifactID); err != nil || !bytes.Equal(got, pngBytes.Bytes()) {
		t.Fatal("published image mapping lost", err)
	}
	record, err := f.service.GetRecord(ctx, f.scope, "archive", main.ArchiveDate)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := f.service.ReadArtifact(ctx, f.scope, record.ArtifactID)
	if err != nil || !bytes.Contains(archive, []byte("remote-main")) || !bytes.Contains(archive, []byte(imageArtifact.ID.String())) {
		t.Fatalf("archive=%s err=%v", archive, err)
	}
	before := len(f.dispatcher.calls)
	if _, err = f.service.Publish(ctx, f.scope, main.ID, f.gateway, "Asia/Shanghai"); err != nil || len(f.dispatcher.calls) != before {
		t.Fatalf("repeat sent/read: calls %d -> %d, err=%v", before, len(f.dispatcher.calls), err)
	}
	supplement := f.prepare(t, "supplement", "reply", "supplement", "remote-main", nil, &main.ID)
	supplement, err = f.service.Publish(ctx, f.scope, supplement.ID, f.gateway, "UTC")
	if err != nil || supplement.ParentPublicationID == nil || supplement.ParentRemoteID != "remote-main" || supplement.RemoteID != "comment-supplement" {
		t.Fatalf("supplement=%#v err=%v", supplement, err)
	}
	if _, err = f.service.CaptureMetrics(ctx, f.scope, main.ID, f.gateway); err == nil {
		t.Fatal("captured metrics before due")
	}
	*f.now = f.now.Add(7 * 24 * time.Hour)
	metric, err := f.service.CaptureMetrics(ctx, f.scope, main.ID, f.gateway)
	if err != nil || metric.ValuesJSON != `{"public_metrics_like_count":9}` {
		t.Fatalf("metric=%#v err=%v", metric, err)
	}
	metricsCalls := countSocialOperation(f.dispatcher.calls, "x-social.metrics.get")
	again, err := f.service.CaptureMetrics(ctx, f.scope, main.ID, f.gateway)
	if err != nil || again.ID != metric.ID || countSocialOperation(f.dispatcher.calls, "x-social.metrics.get") != metricsCalls {
		t.Fatalf("metric repeated: %#v err=%v", again, err)
	}
}

func TestSocialOperationsOutboundJournalUsesNativeReceiptOnce(t *testing.T) {
	f := newSocialIntegrationFixture(t)
	requirements := []socialops.CapabilityRequirement{}
	for _, capability := range []socialops.Capability{socialops.CapabilityAccount, socialops.CapabilityPost, socialops.CapabilityReadback, socialops.CapabilityComment, socialops.CapabilityInbox, socialops.CapabilityMetrics, socialops.CapabilityImages} {
		requirements = append(requirements, socialops.CapabilityRequirement{Capability: capability, Mode: socialops.RequirementOptional})
	}
	cfg := socialops.Configuration{Scope: f.scope, EmployeeVersionID: f.gateway.restored.Run.DigitalHumanVersionID,
		Research: socialops.ResearchBinding{ConnectorID: "exa", ConnectionID: uuid.New()},
		Product:  socialops.ProductConfiguration{Name: "Product", Description: "Description"}, Audience: "Audience", PublicationLanguage: "English", InternalReviewLanguage: "Chinese",
		Sources: []socialops.SourceReference{{URL: "https://example.com/source"}}, Platforms: []socialops.PlatformBinding{{Platform: "x", ConnectorID: "x-social", ConnectionID: f.connection, AccountID: "acct-7", MaxDailyPublications: 1, Capabilities: requirements}},
		Timezone: "Asia/Shanghai", Cadence: socialops.Cadence{DailyContentCron: "0 9 * * *", InboxCron: "0 * * * *", SevenDaySampleCron: "0 10 * * *", WeeklyReviewCron: "0 11 * * 1"},
		TeamHandoff: socialops.TeamHandoff{ConnectorID: "dingtalk", ConnectionID: uuid.New(), OperationID: "dingtalk.messages.send_direct", Destination: "user-1"}}
	if _, _, err := f.service.SaveCurrentConfiguration(t.Context(), f.scope, 0, cfg); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(socialops.OutboundMessage{Text: "private reply", Chinese: "内部审阅"})
	artifact, err := f.service.PutArtifact(t.Context(), f.scope, "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.service.PrepareOutbound(t.Context(), socialops.OutboundRequest{Scope: f.scope, WorkKey: "dm-1", Kind: "dm", AccountID: "acct-7", ConversationID: "conversation-1", ConnectionID: f.connection, BodyArtifactID: artifact.ID})
	if err != nil {
		t.Fatal(err)
	}
	out, err = f.service.ReviewOutbound(t.Context(), f.scope, out.ID, out.Revision, artifact.Digest)
	if err != nil {
		t.Fatal(err)
	}
	before := len(f.dispatcher.calls)
	out, err = f.service.DispatchOutbound(t.Context(), f.scope, out.ID, f.gateway)
	if err != nil || out.State != "completed" || out.ReceiptID != "dm-receipt-1" || out.ReceiptStatus != "accepted_by_platform" {
		t.Fatalf("outbound = %#v, %v", out, err)
	}
	if len(f.dispatcher.calls) != before+1 {
		t.Fatalf("native sends = %d, want one", len(f.dispatcher.calls)-before)
	}
	if _, err = f.service.DispatchOutbound(t.Context(), f.scope, out.ID, f.gateway); err != nil || len(f.dispatcher.calls) != before+1 {
		t.Fatalf("completed outbound resent: %v calls=%d", err, len(f.dispatcher.calls)-before)
	}

	prepare := func(key, text string) socialops.Outbound {
		t.Helper()
		body, marshalErr := json.Marshal(socialops.OutboundMessage{Text: text, Chinese: "内部审阅"})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		artifact, putErr := f.service.PutArtifact(t.Context(), f.scope, "application/json", body)
		if putErr != nil {
			t.Fatal(putErr)
		}
		prepared, prepareErr := f.service.PrepareOutbound(t.Context(), socialops.OutboundRequest{Scope: f.scope, WorkKey: key, Kind: "dm", AccountID: "acct-7", ConversationID: "conversation-1", ConnectionID: f.connection, BodyArtifactID: artifact.ID})
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		reviewed, reviewErr := f.service.ReviewOutbound(t.Context(), f.scope, prepared.ID, prepared.Revision, artifact.Digest)
		if reviewErr != nil {
			t.Fatal(reviewErr)
		}
		return reviewed
	}

	rejected := prepare("dm-rejected", "reject-dm")
	f.dispatcher.rejectFailures["reject-dm"] = 1
	rejected, err = f.service.DispatchOutbound(t.Context(), f.scope, rejected.ID, f.gateway)
	var notSubmitted *socialops.NotSubmittedError
	if !errors.As(err, &notSubmitted) || rejected.State != "reviewed" {
		t.Fatalf("definitive rejection = %#v, %v", rejected, err)
	}
	if retried, retryErr := f.service.DispatchOutbound(t.Context(), f.scope, rejected.ID, f.gateway); retryErr != nil || retried.State != "completed" {
		t.Fatalf("proven-not-submitted retry = %#v, %v", retried, retryErr)
	}

	unknown := prepare("dm-unknown", "unknown-dm")
	unknownBefore := len(f.dispatcher.calls)
	unknown, err = f.service.DispatchOutbound(t.Context(), f.scope, unknown.ID, f.gateway)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeOutcomeUnknown) || unknown.State != "outcome_unknown" || len(f.dispatcher.calls) != unknownBefore+1 {
		t.Fatalf("uncertain send = %#v, %v calls=%d", unknown, err, len(f.dispatcher.calls)-unknownBefore)
	}
	if _, retryErr := f.service.DispatchOutbound(t.Context(), f.scope, unknown.ID, f.gateway); retryErr == nil || len(f.dispatcher.calls) != unknownBefore+1 {
		t.Fatalf("uncertain outbound resent: %v calls=%d", retryErr, len(f.dispatcher.calls)-unknownBefore)
	}
}

func TestSocialOperationsNativeGatewayResumeAndDispatchDisposition(t *testing.T) {
	f := newSocialIntegrationFixture(t)
	ctx := t.Context()
	retry := f.prepare(t, "retry", "post", "retry", "", nil, nil)
	f.dispatcher.publication["remote-retry"] = retry.ID
	f.dispatcher.readFailures["remote-retry"] = 1
	if _, err := f.service.Publish(ctx, f.scope, retry.ID, f.gateway, "UTC"); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("first read error=%v", err)
	}
	stored, err := f.service.GetPublication(ctx, f.scope, retry.ID)
	if err != nil || stored.State != "remote_created" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if _, err = f.service.Publish(ctx, f.scope, retry.ID, f.gateway, "UTC"); err != nil {
		t.Fatal(err)
	}
	if creates := countSocialText(f.dispatcher.calls, "x-social.post.create", "retry"); creates != 1 {
		t.Fatalf("retry sends=%d", creates)
	}
	readIDs := socialLogicalIDs(f.dispatcher.calls, "x-social.post.get", "remote-retry")
	if len(readIDs) != 2 || readIDs[0] == readIDs[1] {
		t.Fatalf("read logical IDs=%#v", readIDs)
	}
	rejected := f.prepare(t, "reject", "post", "reject", "", nil, nil)
	f.dispatcher.rejectFailures["reject"] = 1
	f.dispatcher.publication["remote-reject"] = rejected.ID
	_, err = f.service.Publish(ctx, f.scope, rejected.ID, f.gateway, "UTC")
	var marker *socialops.NotSubmittedError
	if !errors.As(err, &marker) {
		t.Fatalf("rejection=%v", err)
	}
	rejected, _ = f.service.GetPublication(ctx, f.scope, rejected.ID)
	if rejected.State != "reviewed" {
		t.Fatalf("rejected state=%s", rejected.State)
	}
	if _, err = f.service.Publish(ctx, f.scope, rejected.ID, f.gateway, "UTC"); err != nil {
		t.Fatal(err)
	}
	writeIDs := socialLogicalIDs(f.dispatcher.calls, "x-social.post.create", "")
	if len(writeIDs) < 3 || writeIDs[len(writeIDs)-1] == writeIDs[len(writeIDs)-2] {
		t.Fatalf("write logical IDs=%#v", writeIDs)
	}
	unknown := f.prepare(t, "unknown", "post", "unknown", "", nil, nil)
	_, err = f.service.Publish(ctx, f.scope, unknown.ID, f.gateway, "UTC")
	if err == nil || errors.As(err, &marker) {
		t.Fatalf("unknown error=%v", err)
	}
	unknown, _ = f.service.GetPublication(ctx, f.scope, unknown.ID)
	if unknown.State != "outcome_unknown" {
		t.Fatalf("unknown state=%s", unknown.State)
	}
}

func TestSocialOperationsGatewaySelectsConfiguredManagedLinkedInAccount(t *testing.T) {
	g, p, dispatcher := socialGatewayFixture("linkedin", "account.list")
	p.AccountID = "urn:li:organization:22"
	dispatcher.results["linkedin-social.account.list"] = map[string]any{"provider": "linkedin-social", "transport": "mcp", "data": map[string]any{"accounts": []any{map[string]any{"id": "urn:li:organization:11"}, map[string]any{"id": "urn:li:organization:22"}}}}
	if got, err := g.accountID(t.Context(), p); err != nil || got != p.AccountID {
		t.Fatalf("account=%q err=%v", got, err)
	}
	p.AccountID = ""
	if _, err := g.accountID(t.Context(), p); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("ambiguous account error=%v", err)
	}
}

func countSocialOperation(calls []connectordispatch.Invocation, operation string) int {
	n := 0
	for _, call := range calls {
		if call.Identity.OperationID.String() == operation {
			n++
		}
	}
	return n
}
func countSocialText(calls []connectordispatch.Invocation, operation, text string) int {
	n := 0
	for _, call := range calls {
		input, _ := call.Input.(map[string]any)
		if call.Identity.OperationID.String() == operation && input["text"] == text {
			n++
		}
	}
	return n
}
func socialLogicalIDs(calls []connectordispatch.Invocation, operation, postID string) []string {
	var ids []string
	for _, call := range calls {
		input, _ := call.Input.(map[string]any)
		if call.Identity.OperationID.String() == operation && (postID == "" || input["post_id"] == postID) {
			ids = append(ids, call.Identity.LogicalOperationID)
		}
	}
	return ids
}
