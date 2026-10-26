package socialevents

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func fixture(t *testing.T) (*Service, *gorm.DB, connectionRow) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err = db.AutoMigrate(&connectionRow{}, &eventRow{}); err != nil {
		t.Fatal(err)
	}
	vault, err := credentials.NewStore("test", map[string]string{"test": strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(db, vault, Options{FacebookAppSecret: "secret", FacebookVerifyToken: "verify"})
	if err != nil {
		t.Fatal(err)
	}
	c := connectionRow{ID: uuid.New(), OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "facebook-social", ExternalAccountID: "page1", State: "ready"}
	if err = db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	return s, db, c
}
func sign(raw []byte) string {
	h := hmac.New(sha256.New, []byte("secret"))
	h.Write(raw)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
func deliver(t *testing.T, s *Service, body string) {
	t.Helper()
	if err := s.ReceiveFacebook(context.Background(), sign([]byte(body)), []byte(body)); err != nil {
		t.Fatal(err)
	}
}
func TestVerificationAndSignatureFailClosed(t *testing.T) {
	s, db, _ := fixture(t)
	if got, err := s.VerifyChallenge("subscribe", "verify", "123"); err != nil || got != "123" {
		t.Fatal(got, err)
	}
	for _, args := range [][3]string{{"subscribe", "bad", "123"}, {"bad", "verify", "123"}, {"subscribe", "verify", strings.Repeat("x", 513)}, {"subscribe", "verify", ""}} {
		if _, err := s.VerifyChallenge(args[0], args[1], args[2]); err == nil {
			t.Fatal("accepted invalid challenge")
		}
	}
	for _, raw := range []string{`not json`, `{"object":"page","entry":[{"id":"page1"}]}`} {
		if err := s.ReceiveFacebook(context.Background(), "sha256="+strings.Repeat("0", 64), []byte(raw)); err == nil {
			t.Fatal("accepted forged signature")
		}
	}
	for _, raw := range []string{`{"object":"user","entry":[]}`, `{"object":"page","entry":[{}]}`, `{"object":"page","entry":[{"id":"page1"}]} {}`, `{"object":"page","entry":null}`} {
		if err := s.ReceiveFacebook(context.Background(), sign([]byte(raw)), []byte(raw)); err == nil {
			t.Fatal("accepted invalid body", raw)
		}
	}
	var count int64
	db.Model(&eventRow{}).Count(&count)
	if count != 0 {
		t.Fatal(count)
	}
}
func TestEncryptedFanoutDedupeAndOwnerQueue(t *testing.T) {
	s, db, c := fixture(t)
	ctx := context.Background()
	second := c
	second.ID = uuid.New()
	second.OwnerAccountID = uuid.New()
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	other := c
	other.ID = uuid.New()
	other.ExternalAccountID = "page2"
	other.OrganizationID = uuid.New()
	other.OwnerAccountID = uuid.New()
	db.Create(&other)
	blocked := c
	blocked.ID = uuid.New()
	blocked.State = "reconnect_required"
	db.Create(&blocked)
	deliver(t, s, `{"object":"page","entry":[{"id":"page1","time":1,"changes":[{"field":"feed","value":{"text":"private feedback"}}]},{"id":"page2","time":2}]}`)
	deliver(t, s, `{"entry":[{"changes":[{"value":{"text":"private feedback"},"field":"feed"}],"time":1,"id":"page1"}],"object":"page"}`)
	var rows []eventRow
	db.Find(&rows)
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	for _, r := range rows {
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "private feedback") {
			t.Fatal("plaintext stored")
		}
	}
	p, err := s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, 0, 1)
	if err != nil || len(p.Items) != 1 {
		t.Fatal(p, err)
	}
	if !strings.Contains(string(p.Items[0].Payload), "private feedback") || p.Items[0].SourceID == "" {
		t.Fatal(p)
	}
	if _, err = s.List(ctx, c.OrganizationID, second.OwnerAccountID, c.ID, 0, 10); err == nil {
		t.Fatal("cross owner read")
	}
	if err = s.Acknowledge(ctx, c.OrganizationID, second.OwnerAccountID, c.ID, p.Items[0].ID); err == nil {
		t.Fatal("cross owner ack")
	}
	foreign, _ := s.List(ctx, other.OrganizationID, other.OwnerAccountID, other.ID, 0, 10)
	if err = s.Acknowledge(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, foreign.Items[0].ID); err == nil {
		t.Fatal("foreign cursor ack")
	}
	if err = s.Acknowledge(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, p.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Acknowledge(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, p.Items[0].ID); err != nil {
		t.Fatal("ack not idempotent", err)
	}
	p, err = s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, 0, 10)
	if err != nil || len(p.Items) != 0 {
		t.Fatal(p, err)
	}
	q, _ := s.List(ctx, second.OrganizationID, second.OwnerAccountID, second.ID, 0, 10)
	if len(q.Items) != 1 {
		t.Fatal("ack affected another owner")
	}
	db.Model(&connectionRow{}).Where("id = ?", second.ID).Update("external_account_id", "different-page")
	q, err = s.List(ctx, second.OrganizationID, second.OwnerAccountID, second.ID, 0, 10)
	if err != nil || len(q.Items) != 0 {
		t.Fatal("old identity exposed", q, err)
	}
	db.Model(&connectionRow{}).Where("id = ?", c.ID).Update("deleted_at", time.Now())
	if _, err = s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, 0, 10); err == nil {
		t.Fatal("deleted connection read")
	}
}

func TestBoundsChunkIntegrityAndPagination(t *testing.T) {
	s, db, c := fixture(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		entry := map[string]any{"id": "page1", "time": i, "changes": []any{map[string]any{"field": "feed", "value": strings.Repeat("z", 300*1024)}}}
		raw, _ := json.Marshal(map[string]any{"object": "page", "entry": []any{entry}})
		if err := s.ReceiveFacebook(ctx, sign(raw), raw); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, 0, 100)
	if err != nil || len(p.Items) != 1 {
		t.Fatalf("page payload byte bound: items=%d err=%v", len(p.Items), err)
	}
	q, err := s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, p.NextAfterID, 100)
	if err != nil || len(q.Items) != 1 || q.Items[0].ID <= p.Items[0].ID {
		t.Fatal("pagination skipped or repeated", q, err)
	}
	var row eventRow
	db.First(&row)
	var chunks []string
	json.Unmarshal([]byte(row.EncryptedPayload), &chunks)
	chunks[0], chunks[1] = chunks[1], chunks[0]
	tampered, _ := json.Marshal(chunks)
	db.Model(&row).Update("encrypted_payload", string(tampered))
	if _, err = s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, 0, 1); err == nil {
		t.Fatal("accepted reordered chunks")
	}
	for _, limit := range []int{0, -1, 101} {
		if _, err = s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, 0, limit); err == nil {
			t.Fatal("unbounded page")
		}
	}
	if _, err = s.List(ctx, c.OrganizationID, c.OwnerAccountID, c.ID, -1, 1); err == nil {
		t.Fatal("negative cursor")
	}
}
func TestDeliveryAmplificationBoundRollsBack(t *testing.T) {
	s, db, c := fixture(t)
	entries := []any{map[string]any{"id": "page1", "time": 0}}
	for i := 0; i < 100; i++ {
		entries = append(entries, map[string]any{"id": "page1", "time": i + 1})
	}
	raw, _ := json.Marshal(map[string]any{"object": "page", "entry": entries})
	if err := s.ReceiveFacebook(context.Background(), sign(raw), raw); err == nil {
		t.Fatal("unbounded entries accepted")
	}
	for i := 0; i < 100; i++ {
		copy := c
		copy.ID = uuid.New()
		db.Create(&copy)
	}
	raw = []byte(`{"object":"page","entry":[{"id":"page1"}]}`)
	if err := s.ReceiveFacebook(context.Background(), sign(raw), raw); err == nil {
		t.Fatal("unbounded fanout accepted")
	}
	var count int64
	db.Model(&eventRow{}).Count(&count)
	if count != 0 {
		t.Fatal("partial persistence", count)
	}
}

func TestDisabledConfigurationAndCurrentReadiness(t *testing.T) {
	s, db, c := fixture(t)
	if _, err := New(db, s.vault, Options{FacebookAppSecret: "secret"}); err == nil {
		t.Fatal("partial config")
	}
	disabled, err := New(db, s.vault, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = disabled.VerifyChallenge("subscribe", "", "1"); err == nil {
		t.Fatal("disabled challenge accepted")
	}
	if err = disabled.ReceiveFacebook(context.Background(), sign([]byte(`{}`)), []byte(`{}`)); err == nil {
		t.Fatal("disabled webhook accepted")
	}
	deliver(t, s, `{"object":"page","entry":[{"id":"unknown-page"}]}`)
	deliver(t, s, `{"object":"page","entry":[{"id":"page1","time":1},{"id":"page1","time":2},{"id":"page1","time":3}]}`)
	p, err := s.List(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID, 0, 3)
	if err != nil || len(p.Items) != 3 {
		t.Fatal(p, err)
	}
	if err = s.Acknowledge(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID, p.Items[1].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Acknowledge(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID, p.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	q, err := s.List(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID, 0, 3)
	if err != nil || len(q.Items) != 1 || q.Items[0].ID != p.Items[2].ID {
		t.Fatal("ack regression", q, err)
	}
	db.Model(&connectionRow{}).Where("id = ?", c.ID).Update("state", "reconnect_required")
	if _, err = s.List(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID, 0, 10); err == nil {
		t.Fatal("inactive read")
	}
	if err = s.Acknowledge(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID, p.Items[2].ID); err == nil {
		t.Fatal("inactive ack")
	}
}

func TestCanonicalPayloadBound(t *testing.T) {
	s, _, _ := fixture(t)
	// JSON HTML escaping can expand a valid raw delivery beyond the storage cap.
	raw := []byte(`{"object":"page","entry":[{"id":"page1","value":"` + strings.Repeat("<", 200000) + `"}]}`)
	if err := s.ReceiveFacebook(context.Background(), sign(raw), raw); err == nil {
		t.Fatal("accepted oversized canonical entry")
	}
}

func TestCiphertextCannotBeMovedBetweenOwnersInSameOrganization(t *testing.T) {
	s, db, c := fixture(t)
	deliver(t, s, `{"object":"page","entry":[{"id":"page1","value":"private"}]}`)
	other := c
	other.ID = uuid.New()
	other.OwnerAccountID = uuid.New()
	db.Create(&other)
	var row eventRow
	db.First(&row)
	db.Model(&row).Updates(map[string]any{"connection_id": other.ID, "owner_account_id": other.OwnerAccountID})
	if _, err := s.List(context.Background(), other.OrganizationID, other.OwnerAccountID, other.ID, 0, 1); err == nil {
		t.Fatal("ciphertext moved to another owner")
	}
}
