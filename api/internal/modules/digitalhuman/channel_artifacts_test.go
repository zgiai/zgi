package digitalhuman

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
)

func TestChannelDeliveryArtifactsProjectsOnlyTemporaryToolFiles(t *testing.T) {
	trace := datatypes.JSON(`{
		"generated_files":[
			{"artifact_id":"tool_file:tool-1","file_id":"tool-1","filename":"report.pdf","extension":".pdf","transfer_method":"tool_file"},
			{"file_id":"managed-1","upload_file_id":"managed-1","filename":"managed.pdf","target":"managed_file"},
			{"tool_file_id":"expired-1","filename":"expired.txt","availability":"expired"}
		],
		"conversation_artifacts":[
			{"tool_file_id":"tool-1","filename":"duplicate.pdf","extension":"pdf"},
			{"tool_file_id":"tool-2","filename":"notes.md","extension":"md"}
		]
	}`)

	artifacts := channelDeliveryArtifacts(trace)
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %#v, want 2 unique tool files", artifacts)
	}
	if artifacts[0].ToolFileID != "tool-1" || artifacts[0].Filename != "report.pdf" || artifacts[0].Extension != ".pdf" {
		t.Fatalf("first artifact = %#v", artifacts[0])
	}
	if artifacts[1].ToolFileID != "tool-2" || artifacts[1].Extension != ".md" {
		t.Fatalf("second artifact = %#v", artifacts[1])
	}
}

func TestDeliveryProjectsGeneratedFilesAsDownloadLinks(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	payload, digest, err := channelDeliveryPayload("报告已经生成。", channelDeliveryArtifact{
		ToolFileID: "tool-1", Filename: "增长报告.pdf", Extension: ".pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	delivery := &channelruntime.Delivery{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, BindingID: binding.Binding.ID, InboundEventID: uuid.New(), RunID: uuid.New(),
		Provider: "feishu", ConnectionID: installation.ConnectionID, ActorID: scope.ActorID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: digitalHumanID,
		ExternalConversationID: binding.Mapping.ExternalConversationID, Payload: payload, PayloadDigest: digest,
		Status: channelruntime.DeliveryPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	store.deliveries[delivery.ID] = delivery
	service.artifacts = channelArtifactLinkerStub{links: []channelArtifactLink{{
		Filename: "增长报告.pdf", URL: "https://files.example/download?sign=safe",
	}}}
	adapter.receipt = &channelruntime.DeliveryReceipt{ProviderMessageID: "om_file"}
	service.now = func() time.Time { return now }

	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatalf("DispatchDeliveries() error = %v", err)
	}
	if runtime.calls != 0 || adapter.deliveryCalls != 1 {
		t.Fatalf("runtime calls = %d, delivery calls = %d", runtime.calls, adapter.deliveryCalls)
	}
	for _, expected := range []string{"报告已经生成。", "📎 增长报告.pdf", "https://files.example/download?sign=safe"} {
		if !strings.Contains(adapter.lastDelivery.Text, expected) {
			t.Fatalf("delivery text = %q, want %q", adapter.lastDelivery.Text, expected)
		}
	}
	if strings.Contains(string(delivery.Payload), "files.example") {
		t.Fatalf("durable payload persisted an expiring signed URL: %s", delivery.Payload)
	}
	if delivery.Status != channelruntime.DeliverySucceeded {
		t.Fatalf("delivery status = %s, want succeeded", delivery.Status)
	}
}

func TestDeliveryRetriesWhenGeneratedFileLinkCannotBeResolved(t *testing.T) {
	service, store, _, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	delivery := newChannelTestDelivery(t, scope, digitalHumanID, installation, binding, time.Now().UTC())
	payload, digest, err := channelDeliveryPayload("文件已生成", channelDeliveryArtifact{ToolFileID: "tool-1", Extension: ".pdf"})
	if err != nil {
		t.Fatal(err)
	}
	delivery.Payload, delivery.PayloadDigest = payload, digest
	store.deliveries[delivery.ID] = delivery
	service.artifacts = channelArtifactLinkerStub{err: errors.New("temporary file store failure")}
	service.now = func() time.Time { return delivery.AvailableAt }

	if err := service.DispatchDeliveries(t.Context(), 10); !errors.Is(err, errChannelDeliveryAttemptFailed) {
		t.Fatalf("DispatchDeliveries() error = %v, want safe retry error", err)
	}
	if adapter.deliveryCalls != 0 || delivery.Status != channelruntime.DeliveryPending || delivery.AttemptCount != 1 {
		t.Fatalf("delivery = %#v, provider calls = %d", delivery, adapter.deliveryCalls)
	}
}

type channelArtifactLinkerStub struct {
	links []channelArtifactLink
	err   error
}

func (stub channelArtifactLinkerStub) Resolve(context.Context, *channelruntime.Delivery, []channelDeliveryArtifact) ([]channelArtifactLink, error) {
	return append([]channelArtifactLink(nil), stub.links...), stub.err
}
