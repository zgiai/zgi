package digitalhuman

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

func newBackgroundTestFile(t *testing.T, entry string) (*Run, *toolfile.ToolFile, *executionFileStorage) {
	t.Helper()
	run, _, _, _ := backgroundLinuxFixture(t, entry)
	if entry == SurfaceTask {
		entry = backgroundTaskEntry
	}
	_, repo := newTestService(t)
	if err := repo.db.AutoMigrate(&toolfile.ToolFile{}); err != nil {
		t.Fatal(err)
	}
	storage := &executionFileStorage{data: map[string][]byte{}}
	manager := toolfile.NewToolFileManager(repo.db, storage)
	previous := toolfile.GlobalToolFileManager
	toolfile.GlobalToolFileManager = manager
	t.Cleanup(func() { toolfile.GlobalToolFileManager = previous })
	subject := execution.BackgroundSubject(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID, *run.ConversationID, entry)
	scope, err := execution.NewScope(execution.VerifiedSubject{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, ApplicationType: "digital_human", ApplicationID: run.DigitalHumanID,
		Entry: entry, User: subject, ConversationID: *run.ConversationID, PublicationRef: run.DigitalHumanVersionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	cid, mid, name := run.ConversationID.String(), run.MessageID.String(), "result.txt"
	file, err := manager.CreateFileByRaw(t.Context(), toolfile.CreateFileByRawParams{UserID: subject.ID.String(), TenantID: run.OrganizationID.String(), ConversationID: &cid,
		Filename: &name, FileData: []byte("verified-result"), MimeType: "text/plain", ExecutionIdentity: scope.Identity, ExecutionMessageID: &mid})
	if err != nil {
		t.Fatal(err)
	}
	return run, file, storage
}

type channelFileReaderStub struct {
	run    *Run
	denied bool
	reads  int
}

func (s *channelFileReaderStub) DescribeChannelExecutionArtifact(ctx context.Context, delivery *channelruntime.Delivery, id string) (*toolfile.ToolFile, error) {
	if s.denied || delivery.RunID != s.run.ID || delivery.SubjectID != s.run.DigitalHumanID {
		return nil, toolfile.ErrExecutionFileAccess
	}
	f, err := toolfile.GlobalToolFileManager.GetToolFileByID(ctx, id)
	if err != nil {
		return nil, err
	}
	_, err = backgroundExecutionFileAccess(s.run, f)
	return f, err
}
func (s *channelFileReaderStub) ReadChannelExecutionArtifact(ctx context.Context, delivery *channelruntime.Delivery, id string) ([]byte, error) {
	s.reads++
	f, err := s.DescribeChannelExecutionArtifact(ctx, delivery, id)
	if err != nil {
		return nil, err
	}
	access, err := backgroundExecutionFileAccess(s.run, f)
	if err != nil {
		return nil, err
	}
	data, _, err := toolfile.GlobalToolFileManager.GetExecutionFileBinary(ctx, access, id)
	return data, err
}

func TestChannelExecutionArtifactsChooseNativeOrDirectDownload(t *testing.T) {
	run, file, backend := newBackgroundTestFile(t, SurfaceChannel)
	previous := toolfile.GlobalFileSignature
	toolfile.GlobalFileSignature = toolfile.NewFileSignature(&config.Config{App: config.AppConfig{SecretKey: "test-channel-download", FilesURL: "https://files.example"}})
	t.Cleanup(func() { toolfile.GlobalFileSignature = previous })
	reader := &channelFileReaderStub{run: run}
	linker := toolFileChannelArtifactLinker{runtime: reader}
	delivery := &channelruntime.Delivery{ID: uuid.New(), OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: run.DigitalHumanID, RunID: run.ID}
	artifacts := []channelDeliveryArtifact{{ToolFileID: file.ID}, {ToolFileID: file.ID}}
	for _, capabilities := range []channelruntime.ProviderCapabilities{{}, {NativeAttachments: true, MaxAttachmentBytes: 64 << 20}, {NativeAttachments: true, MaxAttachmentBytes: 1}} {
		links, files, err := linker.ResolveDelivery(t.Context(), delivery, artifacts, capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if capabilities.NativeAttachments && capabilities.MaxAttachmentBytes >= file.Size {
			if len(files) != 1 || len(links) != 0 || backend.reads != 0 {
				t.Fatal("native resolution read eagerly or generated a link")
			}
			r, err := files[0].Open(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(r)
			_ = r.Close()
			if string(data) != "verified-result" {
				t.Fatalf("data=%q", data)
			}
			reader.denied = true
			if _, err := files[0].Open(t.Context()); !errors.Is(err, toolfile.ErrExecutionFileAccess) {
				t.Fatal("revocation did not stop file read")
			}
			reader.denied = false
		} else {
			if len(links) != 1 || len(files) != 0 || !strings.Contains(links[0].URL, "/console/api/files/tools/"+file.ID) || !strings.Contains(links[0].URL, "sign=") || !strings.Contains(links[0].URL, "download=1") {
				t.Fatalf("links=%+v files=%d", links, len(files))
			}
		}
	}
	links, _, err := linker.ResolveDelivery(t.Context(), delivery, artifacts, channelruntime.ProviderCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	download, err := url.Parse(links[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/console/api/files/tools/:tool_file_id", toolfile.NewHTTPHandler(toolfile.GlobalToolFileManager).GetToolFile)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, download.RequestURI(), nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "verified-result" || !strings.Contains(recorder.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("direct download=%d %s", recorder.Code, recorder.Body.String())
	}
	reader.denied = true
	if _, _, err := linker.ResolveDelivery(t.Context(), delivery, artifacts, channelruntime.ProviderCapabilities{}); err == nil {
		t.Fatal("revoked route minted a download link")
	}
	reader.denied = false
	toolfile.GlobalFileSignature = nil
	if _, _, err := linker.ResolveDelivery(t.Context(), delivery, artifacts, channelruntime.ProviderCapabilities{}); err == nil {
		t.Fatal("missing signer produced a download link")
	}
	linker.runtime = nil
	if _, _, err := linker.ResolveDelivery(t.Context(), delivery, artifacts, channelruntime.ProviderCapabilities{NativeAttachments: true, MaxAttachmentBytes: 64 << 20}); err == nil {
		t.Fatal("missing owner reader accepted")
	}
}

func TestBackgroundFileOwnerRejectsOtherRunMessageAndApplication(t *testing.T) {
	for _, entry := range []string{SurfaceChannel, SurfaceTask} {
		t.Run(entry, func(t *testing.T) {
			run, file, _ := newBackgroundTestFile(t, entry)
			if _, err := backgroundExecutionFileAccess(run, file); err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []func(*Run, *toolfile.ToolFile){
				func(r *Run, _ *toolfile.ToolFile) { r.DigitalHumanID = uuid.New() },
				func(r *Run, _ *toolfile.ToolFile) { r.MessageID = uuidPointer(uuid.New()) },
				func(r *Run, _ *toolfile.ToolFile) { r.ConversationID = uuidPointer(uuid.New()) },
				func(_ *Run, f *toolfile.ToolFile) { f.UserID = uuid.NewString() },
				func(_ *Run, f *toolfile.ToolFile) { f.ExecutionIdentity = nil },
				func(_ *Run, f *toolfile.ToolFile) {
					past := time.Now().Add(-time.Minute)
					f.Lifecycle, f.ExpiresAt = string(toolfile.ToolFileLifecycleTemporary), &past
				},
			} {
				r, f := *run, *file
				mutate(&r, &f)
				if _, err := backgroundExecutionFileAccess(&r, &f); err == nil {
					t.Fatal("foreign protected artifact accepted")
				}
			}
		})
	}
}

type backgroundChannelFileStore struct {
	RunStore
	ChannelStore
}

type channelNativeAdapterFixture struct {
	*channelAdapterStub
	native bool
}

func (a channelNativeAdapterFixture) Capabilities() channelruntime.ProviderCapabilities {
	caps := a.channelAdapterStub.Capabilities()
	caps.NativeAttachments, caps.MaxAttachmentBytes = a.native, 30_000_000
	return caps
}

type channelNativeArtifactFixture struct{ t *testing.T }

func (s channelNativeArtifactFixture) Resolve(context.Context, *channelruntime.Delivery, []channelDeliveryArtifact) ([]channelArtifactLink, error) {
	s.t.Fatal("dispatcher ignored attachment capability resolver")
	return nil, nil
}

func (s channelNativeArtifactFixture) ResolveDelivery(_ context.Context, _ *channelruntime.Delivery, files []channelDeliveryArtifact, caps channelruntime.ProviderCapabilities) ([]channelArtifactLink, []channelruntime.DeliveryAttachment, error) {
	if len(files) != 1 || files[0].ToolFileID != "test-file" {
		s.t.Fatal("dispatcher lost durable file reference")
	}
	if !caps.NativeAttachments {
		return []channelArtifactLink{{Filename: "result.txt", URL: "https://zgi.example/console/api/files/tools/result.txt?download=1"}}, nil, nil
	}
	return nil, []channelruntime.DeliveryAttachment{{ID: "test-file", Filename: "result.txt", Size: 1,
		Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("x")), nil }}}, nil
}

func TestChannelDispatchUsesAttachmentCapabilityWithoutExceptionFallback(t *testing.T) {
	for _, native := range []bool{false, true} {
		service, store, _, adapter, scope, application := newChannelTestService(t)
		installation, route := createChannelTestRoute(t, service, scope, application, channelruntime.ConversationTypeDirect)
		registry, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: channelNativeAdapterFixture{channelAdapterStub: adapter, native: native}, Provisioner: channelProvisionerStub{}})
		if err != nil {
			t.Fatal(err)
		}
		service.adapters, service.artifacts = registry, channelNativeArtifactFixture{t: t}
		delivery := newChannelTestDelivery(t, scope, application, installation, route, time.Now().UTC())
		delivery.Payload, delivery.PayloadDigest, err = channelDeliveryPayload("", channelDeliveryArtifact{ToolFileID: "test-file"})
		if err != nil {
			t.Fatal(err)
		}
		store.deliveries[delivery.ID] = delivery
		service.now = func() time.Time { return delivery.AvailableAt }
		adapter.receipt = &channelruntime.DeliveryReceipt{ProviderMessageID: "sent"}
		if native {
			adapter.deliveryErr = channelruntime.ErrDeliveryOutcomeUnknown
		}
		err = service.DispatchDeliveries(t.Context(), 10)
		if native {
			if err == nil || delivery.Status != channelruntime.DeliveryUnknown || len(adapter.lastDelivery.Attachments) != 1 || adapter.lastDelivery.Text != "" {
				t.Fatal("attachment-only delivery failed or uncertain send became link fallback")
			}
			adapter.deliveryErr = nil
			if err := service.DispatchDeliveries(t.Context(), 10); err != nil || adapter.deliveryCalls != 1 {
				t.Fatal("uncertain attachment delivery was resent")
			}
		} else if err != nil || delivery.Status != channelruntime.DeliverySucceeded || len(adapter.lastDelivery.Attachments) != 0 || !strings.Contains(adapter.lastDelivery.Text, "/console/api/files/tools/result.txt?download=1") {
			t.Fatal("unadapted provider did not receive download link")
		}
	}
}

func TestChannelExecutionFileReaderRequiresLiveExactRoute(t *testing.T) {
	run, file, backend := newBackgroundTestFile(t, SurfaceChannel)
	_, event, err := decodeAndVerifyRun(run)
	if err != nil {
		t.Fatal(err)
	}
	installationID := uuid.MustParse(event.ReplyTo.InstallationID)
	bindingID := uuid.MustParse(event.Context["channel_binding_id"].(string))
	connectionID, mappingID := uuid.New(), uuid.New()
	channels, runs := newChannelStoreStub(), newFakeRunStore()
	channels.installations[installationID] = &channelruntime.Installation{ID: installationID,
		OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, Provider: event.ReplyTo.Provider,
		ConnectionID: connectionID, Status: channelruntime.InstallationStatusActive}
	channels.mappings[mappingID] = &channelruntime.ConversationMapping{ID: mappingID, InstallationID: installationID,
		OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID,
		ExternalConversationID: event.ReplyTo.ConversationID, LocalConversationID: run.ConversationID}
	channels.bindings[bindingID] = &channelruntime.Binding{ID: bindingID, InstallationID: installationID,
		OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, ConversationMappingID: mappingID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: run.DigitalHumanID, Enabled: true}
	runs.runs[run.ID] = run
	service := &RuntimeService{store: backgroundChannelFileStore{RunStore: runs, ChannelStore: channels}}
	delivery := &channelruntime.Delivery{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: run.DigitalHumanID, RunID: run.ID,
		InstallationID: installationID, BindingID: bindingID, ConnectionID: connectionID,
		Provider: event.ReplyTo.Provider, ExternalConversationID: event.ReplyTo.ConversationID}
	if _, err := service.DescribeChannelExecutionArtifact(t.Context(), delivery, file.ID); err != nil || backend.reads != 0 {
		t.Fatalf("metadata access=%v reads=%d", err, backend.reads)
	}
	data, err := service.ReadChannelExecutionArtifact(t.Context(), delivery, file.ID)
	if err != nil || string(data) != "verified-result" {
		t.Fatalf("file access=%v data=%q", err, data)
	}
	for _, mutate := range []func(*channelruntime.Delivery){
		func(d *channelruntime.Delivery) { d.ExternalConversationID = "oc_other" },
		func(d *channelruntime.Delivery) { d.ExternalThreadID = "other-thread" },
		func(d *channelruntime.Delivery) { d.BindingID = uuid.New() },
		func(d *channelruntime.Delivery) { d.SubjectID = uuid.New() },
		func(d *channelruntime.Delivery) { d.ConnectionID = uuid.New() },
	} {
		other := *delivery
		mutate(&other)
		if _, err := service.ReadChannelExecutionArtifact(t.Context(), &other, file.ID); !errors.Is(err, toolfile.ErrExecutionFileAccess) || backend.reads != 1 {
			t.Fatal("foreign route read storage")
		}
	}
	channels.installations[installationID].Status = channelruntime.InstallationStatusDisabled
	if _, err := service.ReadChannelExecutionArtifact(t.Context(), delivery, file.ID); !errors.Is(err, toolfile.ErrExecutionFileAccess) || backend.reads != 1 {
		t.Fatal("revoked installation read storage")
	}
}

func TestBackgroundFileDownloadRechecksApplicationAndFrozenMessage(t *testing.T) {
	run, file, backend := newBackgroundTestFile(t, SurfaceTask)
	scope := Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, ActorID: uuid.New()}
	application := &executionFileApplicationStub{}
	service := runtimeHandlerServiceStub{get: func(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error) { return run, nil }}
	router := digitalHumanRuntimeHandlerRouter(t, service, scope, application)
	path := "/digital-humans/" + run.DigitalHumanID.String() + "/runs/" + run.ID.String() + "/files/" + file.ID
	t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "false")
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "verified-result" {
		t.Fatalf("download=%d %s", recorder.Code, recorder.Body.String())
	}
	application.denied = true
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK || backend.reads != 1 {
		t.Fatal("revoked app read file")
	}
	application.denied = false
	run.MessageID = uuidPointer(uuid.New())
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK || backend.reads != 1 {
		t.Fatal("another message read file")
	}
}
