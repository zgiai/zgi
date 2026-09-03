package onboarding

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"strings"
	"testing"
	"time"
)

type protocolMemory struct{ memoryFlows }

func (m *protocolMemory) Claim(ctx context.Context, id uuid.UUID) (Flow, bool, error) {
	if m.flow.Step == "awaiting_code" {
		return Flow{}, false, nil
	}
	f, ok, err := m.memoryFlows.Claim(ctx, id)
	m.flow.StepDispatched = true
	f.StepDispatched = true
	return f, ok, err
}
func (m *protocolMemory) SaveProtocolStep(_ context.Context, f Flow, d DeviceRecord, next ProtocolResult) error {
	if !m.flow.State.Active() || m.flow.Fencing != f.Fencing || !validProtocolTransition(f, next.Step) {
		return registrationError(core.ErrorCodeConflict)
	}
	m.flow.State = Awaiting
	m.flow.Step = next.Step
	m.flow.StepDispatched = false
	m.flow.Revision++
	m.flow.LeaseOwner = nil
	m.flow.LeaseUntil = nil
	m.device = d
	if next.Step == "awaiting_code" {
		m.flow.ChallengeRevision++
	}
	return nil
}
func (m *protocolMemory) SubmitChallenge(_ context.Context, f Flow, d DeviceRecord, rev, challenge int64) (Flow, error) {
	if rev != m.flow.Revision || challenge != m.flow.ChallengeRevision || m.flow.Step != "awaiting_code" {
		return Flow{}, registrationError(core.ErrorCodeConflict)
	}
	m.device = d
	m.flow.Step = "poll"
	m.flow.Revision++
	m.flow.ChallengeAttempts++
	return m.flow, nil
}

type adapterFixture struct {
	calls    int
	id       string
	identity string
}

func (a *adapterFixture) ConnectorID() string { return a.id }
func (a *adapterFixture) Advance(_ context.Context, f Flow, s ProtocolSecret) (ProtocolResult, error) {
	a.calls++
	switch f.Step {
	case "init":
		return ProtocolResult{Step: "begin", Secret: ProtocolSecret{Nonce: "PRIVATE_NONCE"}, Interval: time.Second}, nil
	case "begin":
		return ProtocolResult{Step: "poll", Secret: ProtocolSecret{DeviceCode: "PRIVATE_DEVICE", QRPayload: "https://login.dingtalk.com/qr"}, Interval: time.Second}, nil
	case "poll":
		return ProtocolResult{Values: map[string]string{"client_id": a.identity, "client_secret": "PRIVATE_CREDENTIAL"}}, nil
	}
	return ProtocolResult{}, registrationError(core.ErrorCodeInvalidInput)
}
func TestMessagingDurableVerificationAndReadOnlyRecovery(t *testing.T) {
	s, _, legacy, _, c := serviceFixture(t)
	d, _ := catalog.MessagingDefinition("dingtalk")
	s.catalog, _ = catalog.New(d)
	repo := &protocolMemory{}
	s.repository = repo
	a := &adapterFixture{id: "dingtalk", identity: "app1"}
	if err := s.WithMessagingAdapters(a); err != nil {
		t.Fatal(err)
	}
	c.ConnectorID = "dingtalk"
	c.ProtocolVersion = 1
	c.Intent = "register_bot"
	c.Region = ""
	c.TemplateID = ""
	ctx := context.Background()
	first, err := s.Start(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Start(ctx, c)
	if err != nil || again.ID != first.ID || a.calls != 0 || legacy.beginCalls != 0 {
		t.Fatal("read registered")
	}
	for i := 0; i < 3; i++ {
		if _, err = s.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := s.Get(ctx, c.OrganizationID, c.OwnerAccountID, first.ID)
	if err != nil || snapshot.Step != "verify" || snapshot.QRPayload != "" || repo.commits != 0 {
		t.Fatal("verification was not separate durable step", err)
	}
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "PRIVATE_") || strings.Contains(repo.device.EncryptedDevice, "PRIVATE_") {
		t.Fatal("plaintext secret")
	}
	if _, err = s.ProcessOne(ctx); err != nil || repo.commits != 1 || a.calls != 3 || repo.flow.State != Ready || repo.device.EncryptedDevice != "" {
		t.Fatal("native commit", err)
	}
	if _, err = s.Start(ctx, StartCommand{OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectorID: "dingtalk", Intent: "register_bot", ProtocolVersion: 99, IdempotencyKey: uuid.NewString()}); err == nil || a.calls != 3 {
		t.Fatal("unknown protocol")
	}
}
func TestMessagingCancelDropsCandidateBeforeCommit(t *testing.T) {
	s, _, _, _, c := serviceFixture(t)
	d, _ := catalog.MessagingDefinition("dingtalk")
	s.catalog, _ = catalog.New(d)
	repo := &protocolMemory{}
	s.repository = repo
	a := &adapterFixture{id: "dingtalk", identity: "app1"}
	_ = s.WithMessagingAdapters(a)
	c.ConnectorID = "dingtalk"
	c.ProtocolVersion = 1
	c.Intent = "register_bot"
	c.Region = ""
	c.TemplateID = ""
	ctx := context.Background()
	first, err := s.Start(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = s.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Cancel(ctx, c.OrganizationID, c.OwnerAccountID, first.ID, repo.flow.Revision); err != nil {
		t.Fatal(err)
	}
	_, _ = s.ProcessOne(ctx)
	if repo.commits != 0 || repo.device.EncryptedDevice != "" || repo.flow.State != Cancelled {
		t.Fatal("cancelled candidate committed")
	}
}
