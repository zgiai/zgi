package socialops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type outboundGatewaySpy struct {
	calls   int
	err     error
	receipt OutboundReceipt
}

func (g *outboundGatewaySpy) SendOutbound(context.Context, Outbound, OutboundMessage) (OutboundReceipt, error) {
	g.calls++
	return g.receipt, g.err
}
func outboundFixture(t *testing.T) (*Service, Scope, Outbound) {
	t.Helper()
	s, scope, cfg := configuredPublicationFixture(t, 2)
	if err := s.db.AutoMigrate(&Outbound{}); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(OutboundMessage{Text: "Reproduction steps and source", Subject: "Product feedback"})
	a, err := s.PutArtifact(context.Background(), scope, "application/json", b)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.PrepareOutbound(context.Background(), OutboundRequest{Scope: scope, WorkKey: "feedback-123", Kind: "handoff", BodyArtifactID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if out.ConnectionID != cfg.TeamHandoff.ConnectionID || out.OperationID != cfg.TeamHandoff.OperationID {
		t.Fatal("target not restored")
	}
	out, err = s.ReviewOutbound(context.Background(), scope, out.ID, out.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	return s, scope, out
}
func TestOutboundPersistsReceiptAndNeverRepeatsSend(t *testing.T) {
	s, scope, out := outboundFixture(t)
	g := &outboundGatewaySpy{receipt: OutboundReceipt{ID: "message-1", Status: "accepted_by_server"}}
	done, err := s.DispatchOutbound(context.Background(), scope, out.ID, g)
	if err != nil || done.State != "completed" || done.ReceiptID != "message-1" {
		t.Fatal(done, err)
	}
	if _, err = s.DispatchOutbound(context.Background(), scope, out.ID, g); err != nil || g.calls != 1 {
		t.Fatal("repeated send", err, g.calls)
	}
	foreign := scope
	foreign.OwnerID = out.ID
	if _, err = s.GetOutbound(context.Background(), foreign, out.ID); err == nil {
		t.Fatal("cross-owner receipt exposed")
	}
}
func TestOutboundUnknownNeverRetriesAndDefiniteRejectionCanRetry(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown", true: "rejected"}[known], func(t *testing.T) {
			s, scope, out := outboundFixture(t)
			failure := errors.New("transport failure")
			var sendErr error = failure
			if known {
				sendErr = &NotSubmittedError{Cause: failure}
			}
			g := &outboundGatewaySpy{err: sendErr}
			saved, err := s.DispatchOutbound(context.Background(), scope, out.ID, g)
			if err == nil {
				t.Fatal("error suppressed")
			}
			if !known && saved.State != "outcome_unknown" {
				t.Fatal(saved)
			}
			if known && saved.State != "reviewed" {
				t.Fatal(saved)
			}
			g.err = nil
			g.receipt = OutboundReceipt{ID: "receipt-1", Status: "sent"}
			_, err = s.DispatchOutbound(context.Background(), scope, out.ID, g)
			if known && (err != nil || g.calls != 2) {
				t.Fatal(err, g.calls)
			}
			if !known && (err == nil || g.calls != 1) {
				t.Fatal("unknown retried", err, g.calls)
			}
		})
	}
}
