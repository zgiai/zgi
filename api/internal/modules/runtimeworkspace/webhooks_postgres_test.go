package runtimeworkspace

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	"github.com/zgiai/zgi/api/internal/infra/webhookhttp"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// testWebhookCipher seals values as plain JSON bound to the AAD's runtime.
type testWebhookCipher struct{}

func (testWebhookCipher) EncryptCredentials(values map[string]string, aad credentialcrypto.CredentialAAD) (string, error) {
	raw, _ := json.Marshal(map[string]any{"aad": aad.ConnectionID.String(), "values": values})
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (testWebhookCipher) DecryptCredentials(envelope string, aad credentialcrypto.CredentialAAD) (map[string]string, error) {
	raw, err := base64.StdEncoding.DecodeString(envelope)
	if err != nil {
		return nil, err
	}
	var sealed struct {
		AAD    string            `json:"aad"`
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal(raw, &sealed); err != nil || sealed.AAD != aad.ConnectionID.String() {
		return nil, errors.New("aad mismatch")
	}
	return sealed.Values, nil
}

type receivedWebhook struct {
	event, delivery, timestamp, signature string
	body                                  []byte
}

func TestRuntimeWorkspaceWebhooksReportEndUsersPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Runtime Workspace webhook PostgreSQL integration")
	}
	t.Setenv("ZGI_RUNTIME_WORKSPACE_WEBHOOK_ALLOW_PRIVATE", "true")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Webhook test')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Webhook owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, 'Webhook workspace', ?)", workspaceID, organizationID)
	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode, policy_json) VALUES (?, ?, ?, 'active', 'attached', ?)",
		workspaceID, organizationID, accountID, `{"end_user_quota": {"limits": [{"window": "24h", "points": 10}], "plans": [{"key": "pro", "limits": []}]}}`)
	var runtime RuntimeWorkspace
	if err := tx.Where("workspace_id = ?", workspaceID).First(&runtime).Error; err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var received []receivedWebhook
	failNext := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		received = append(received, receivedWebhook{event: r.Header.Get("X-ZGI-Event"), delivery: r.Header.Get("X-ZGI-Delivery"),
			timestamp: r.Header.Get("X-ZGI-Timestamp"), signature: r.Header.Get("X-ZGI-Signature"), body: body})
		if failNext > 0 {
			failNext--
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	clock := now
	service := NewService(tx, WithWebhookCipher(testWebhookCipher{}))
	service.now = func() time.Time { return clock }
	guard := NewQuotaGuard(tx)
	guard.now = func() time.Time { return clock }
	worker := NewWebhookWorker(tx, testWebhookCipher{})
	worker.now = func() time.Time { return clock }

	enabled := true
	for name, input := range map[string]WebhookInput{
		"no events":     {URL: server.URL, Enabled: &enabled},
		"unknown event": {URL: server.URL, Events: []string{"user.created"}},
		"not http":      {URL: "ftp://example.com/hook", Events: []string{WebhookQuotaNear}},
		"with userinfo": {URL: "https://user:pass@example.com/hook", Events: []string{WebhookQuotaNear}},
	} {
		if _, err := service.SaveWebhook(t.Context(), workspaceID, input); !errors.Is(err, ErrInvalidWorkspace) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	if view, err := service.GetWebhook(t.Context(), workspaceID); err != nil || view.Configured {
		t.Fatalf("webhook before setup = %+v %v", view, err)
	}
	saved, err := service.SaveWebhook(t.Context(), workspaceID, WebhookInput{URL: server.URL, Events: WebhookEvents, CreditsLowThreshold: decimal.RequireFromString("0.5")})
	if err != nil || saved.Secret == "" || !saved.Webhook.Configured || len(saved.Webhook.Events) != 4 || saved.Webhook.CreditsLowThreshold != 0.5 {
		t.Fatalf("saved webhook = %+v %v", saved, err)
	}
	secret := saved.Secret
	if again, err := service.SaveWebhook(t.Context(), workspaceID, WebhookInput{URL: server.URL, Events: WebhookEvents, CreditsLowThreshold: decimal.RequireFromString("0.5")}); err != nil || again.Secret != "" {
		t.Fatalf("updating returned a secret: %+v %v", again, err)
	}

	alice, err := EndUserSubject(runtime.LogicalID(), "alice@example")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureEndUser(t.Context(), runtime.LogicalID(), alice, "alice@example"); err != nil {
		t.Fatal(err)
	}
	bill := func(points int64, at time.Time) {
		t.Helper()
		exec(`INSERT INTO llm_usage_bills (id, attempt_id, request_id, organization_id, api_key_id, model_id, model_name, provider_id, provider_name, status, private_points, total_points, end_user_subject_id, request_created_at, settled_at)
			VALUES (?, ?, ?, ?, ?, ?, 'model', ?, 'provider', 'success', ?, ?, ?, ?, ?)`,
			uuid.New(), uuid.NewString(), uuid.NewString(), organizationID, uuid.New(), uuid.New(), uuid.New(), points, points, alice, at, at)
	}
	check := func() error {
		return guard.CheckEndUserQuota(t.Context(), chatruntime.EndUserQuotaRef{SubjectID: alice})
	}
	queued := func(event string) int64 {
		t.Helper()
		var count int64
		if err := tx.Model(&WebhookDelivery{}).Where("runtime_workspace_id = ? AND event = ?", runtime.LogicalID(), event).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		return count
	}

	bill(8_500, now.Add(-time.Hour))
	for range 2 {
		if err := check(); err != nil {
			t.Fatalf("alice near her limit was refused: %v", err)
		}
	}
	if near := queued(WebhookQuotaNear); near != 1 {
		t.Fatalf("quota.near queued %d times", near)
	}
	bill(2_000, now.Add(-time.Minute))
	_ = check()
	_ = check()
	if exceeded := queued(WebhookQuotaExceeded); exceeded != 1 {
		t.Fatalf("quota.exceeded queued %d times", exceeded)
	}

	// The first attempt fails and is retried after the backoff.
	if err := worker.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	var pending []WebhookDelivery
	if err := tx.Where("runtime_workspace_id = ? AND status = ?", runtime.LogicalID(), WebhookDeliveryPending).Find(&pending).Error; err != nil || len(pending) != 1 ||
		pending[0].Attempts != 1 || pending[0].LastStatus != http.StatusBadGateway || !pending[0].NextAttemptAt.Equal(now.Add(webhookFirstRetry)) {
		t.Fatalf("after a failed attempt = %+v %v", pending, err)
	}
	clock = now.Add(webhookFirstRetry + time.Second)
	if err := worker.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := append([]receivedWebhook(nil), received...)
	mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("received %d webhooks", len(got))
	}
	last := got[len(got)-1]
	if last.signature != "sha256="+webhookhttp.Sign(secret, last.timestamp, last.body) {
		t.Fatalf("signature %q does not verify", last.signature)
	}
	var payload struct {
		ID    string         `json:"id"`
		Event string         `json:"event"`
		Data  map[string]any `json:"data"`
	}
	if err := json.Unmarshal(last.body, &payload); err != nil || payload.ID != last.delivery || payload.Event != last.event ||
		payload.Data["user"] != "alice@example" || payload.Data["window"] != "24h" {
		t.Fatalf("payload = %s %v", last.body, err)
	}

	// Credits falling to the threshold are reported once.
	if _, err := service.GrantCredits(t.Context(), workspaceID, alice, EndUserActor{Kind: EndUserActorAccount, ID: accountID}, CreditGrant{Points: decimal.NewFromInt(1), ExternalID: "order-1"}); err != nil {
		t.Fatal(err)
	}
	bill(600, clock.Add(time.Second))
	clock = clock.Add(time.Minute)
	_ = check()
	_ = check()
	if low := queued(WebhookCreditsLow); low != 1 {
		t.Fatalf("credits.low queued %d times", low)
	}

	// An expired plan is reported once by the worker.
	expiry := clock.Add(time.Hour)
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, alice, EndUserActor{Kind: EndUserActorAccount, ID: accountID}, EndUserPatch{Plan: &EndUserPlanAssignment{Key: "pro", ExpiresAt: &expiry}}); err != nil {
		t.Fatal(err)
	}
	clock = expiry.Add(time.Minute)
	for range 2 {
		if err := worker.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if expired := queued(WebhookPlanExpired); expired != 1 {
		t.Fatalf("plan.expired queued %d times", expired)
	}

	// A delivery that keeps failing gives up, and can be sent again.
	test, err := service.TestWebhook(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE runtime_workspace_webhook_deliveries SET attempts = ? WHERE id = ?", webhookMaxAttempts-1, test.ID)
	mu.Lock()
	failNext = 1
	mu.Unlock()
	if err := worker.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	deliveries, err := service.ListWebhookDeliveries(t.Context(), workspaceID, 50)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, delivery := range deliveries {
		status[delivery.Event] = delivery.Status
	}
	if status[WebhookTest] != WebhookDeliveryFailed || status[WebhookPlanExpired] != WebhookDeliveryDelivered || status[WebhookCreditsLow] != WebhookDeliveryDelivered {
		t.Fatalf("delivery states = %+v", status)
	}
	testID := uuid.MustParse(test.ID)
	if err := service.RetryWebhookDelivery(t.Context(), workspaceID, testID); err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	var retried WebhookDelivery
	if err := tx.Where("id = ?", testID).Take(&retried).Error; err != nil || retried.Status != WebhookDeliveryDelivered {
		t.Fatalf("retried test = %+v %v", retried, err)
	}

	if rotated, err := service.RotateWebhookSecret(t.Context(), workspaceID); err != nil || rotated == "" || rotated == secret {
		t.Fatalf("rotated secret = %q %v", rotated, err)
	}
	if err := service.EraseEndUser(t.Context(), workspaceID, alice, EndUserActor{Kind: EndUserActorAccount, ID: accountID}); err != nil {
		t.Fatal(err)
	}
	var left int64
	if err := tx.Raw(`SELECT (SELECT count(*) FROM runtime_workspace_end_user_notices WHERE subject_id = ?)
		+ (SELECT count(*) FROM runtime_workspace_webhook_deliveries WHERE subject_id = ?)`, alice, alice).Scan(&left).Error; err != nil || left != 0 {
		t.Fatalf("erased alice keeps %d webhook rows: %v", left, err)
	}
	if err := service.DeleteWebhook(t.Context(), workspaceID); err != nil {
		t.Fatal(err)
	}
	if view, err := service.GetWebhook(t.Context(), workspaceID); err != nil || view.Configured {
		t.Fatalf("webhook after removal = %+v %v", view, err)
	}
}
