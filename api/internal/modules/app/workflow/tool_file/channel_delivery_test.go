package tool_file

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
)

func TestChannelDeliveryDownloadGrantIsFiniteAndPurposeBound(t *testing.T) {
	for _, entry := range []string{"channel", "task"} {
		t.Run(entry, func(t *testing.T) {
			manager, backend, params, _ := executionFileFixture(t)
			i := params.ExecutionIdentity
			i.Version, i.ApplicationType, i.Entry = execution.BackgroundIdentityVersion, "digital_human", entry
			i.SubjectKind = "channel_conversation"
			if entry == "task" {
				i.SubjectKind = "scheduled_task"
			}
			name := "report.txt"
			params.Filename = &name
			file, err := manager.CreateFileByRaw(t.Context(), params)
			if err != nil {
				t.Fatal(err)
			}
			signer := NewFileSignature(&config.Config{App: config.AppConfig{SecretKey: "test-channel-grant", FilesURL: "https://files.example", FilesAccessTimeout: 7200}})
			deliveryID := uuid.NewString()
			raw, err := signer.signChannelDeliveryFile(file, deliveryID, ".txt")
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			expires, _ := strconv.ParseInt(parsed.Query().Get("expires_at"), 10, 64)
			if expires <= time.Now().Add(29*24*time.Hour).Unix() || expires > time.Now().Add(30*24*time.Hour).Unix() {
				t.Fatalf("invalid expiry %d", expires)
			}
			router := gin.New()
			handler := &HTTPHandler{manager: manager, signature: signer}
			router.GET("/console/api/files/tools/:tool_file_id", handler.GetToolFile)
			request := func(uri string) *httptest.ResponseRecorder {
				r := httptest.NewRecorder()
				router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, uri, nil))
				return r
			}
			r := request(parsed.RequestURI() + "&delivery=direct")
			if r.Code != http.StatusOK || r.Body.String() != "private-result" || !strings.Contains(r.Header().Get("Content-Disposition"), "attachment") || r.Header().Get("Location") != "" {
				t.Fatalf("download=%d %s", r.Code, r.Body.String())
			}
			if backend.presignCalls != 0 {
				t.Fatal("grant redirected to storage")
			}
			loads := backend.loadCalls
			for _, mutate := range []func(*url.URL){
				func(u *url.URL) { u.Path = strings.Replace(u.Path, file.ID, uuid.NewString(), 1) },
				func(u *url.URL) {
					q := u.Query()
					q.Set(channelDeliveryQueryKey, uuid.NewString())
					u.RawQuery = q.Encode()
				},
				func(u *url.URL) { q := u.Query(); q.Del(channelDeliveryQueryKey); u.RawQuery = q.Encode() },
				func(u *url.URL) { q := u.Query(); q.Set("nonce", strings.Repeat("0", 32)); u.RawQuery = q.Encode() },
				func(u *url.URL) { q := u.Query(); q.Set("download", "0"); u.RawQuery = q.Encode() },
				func(u *url.URL) {
					q := u.Query()
					q.Set("expires_at", "0")
					q.Set("sign", base64.URLEncoding.EncodeToString(signer.channelDeliveryMAC(file.ID, deliveryID, "0", q.Get("nonce"))))
					u.RawQuery = q.Encode()
				},
				func(u *url.URL) {
					q := u.Query()
					expiry := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
					q.Set("expires_at", expiry)
					q.Set("sign", base64.URLEncoding.EncodeToString(signer.channelDeliveryMAC(file.ID, deliveryID, expiry, q.Get("nonce"))))
					u.RawQuery = q.Encode()
				},
				func(u *url.URL) {
					q := u.Query()
					expiry := strconv.FormatInt(time.Now().Add(31*24*time.Hour).Unix(), 10)
					q.Set("expires_at", expiry)
					q.Set("sign", base64.URLEncoding.EncodeToString(signer.channelDeliveryMAC(file.ID, deliveryID, expiry, q.Get("nonce"))))
					u.RawQuery = q.Encode()
				},
			} {
				other := *parsed
				mutate(&other)
				if response := request(other.RequestURI()); response.Code == http.StatusOK || strings.Contains(response.Body.String(), "private-result") {
					t.Fatalf("invalid grant accepted: %s", other.RawQuery)
				}
			}
			legacy, err := signer.SignToolFile(file.ID, ".txt")
			if err != nil {
				t.Fatal(err)
			}
			legacyURL, _ := url.Parse(legacy)
			q := legacyURL.Query()
			q.Set(channelDeliveryQueryKey, deliveryID)
			q.Set("download", "1")
			legacyURL.RawQuery = q.Encode()
			if response := request(legacyURL.RequestURI()); response.Code == http.StatusOK {
				t.Fatal("legacy signature acquired a channel grant")
			}
			if backend.loadCalls != loads {
				t.Fatal("invalid grant touched storage")
			}
			deleted := time.Now()
			if err := manager.db.Model(file).Update("deleted_at", &deleted).Error; err != nil {
				t.Fatal(err)
			}
			if response := request(parsed.RequestURI()); response.Code == http.StatusOK || backend.loadCalls != loads {
				t.Fatal("deleted artifact remained readable")
			}
		})
	}
}

func TestChannelDeliveryDownloadGrantSupportsUnprotectedFiles(t *testing.T) {
	manager, backend, params, _ := executionFileFixture(t)
	params.ExecutionIdentity = nil
	params.ExecutionMessageID = nil
	name := "legacy-report.txt"
	params.Filename = &name
	file, err := manager.CreateFileByRaw(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	signer := NewFileSignature(&config.Config{App: config.AppConfig{SecretKey: "test-channel-grant", FilesURL: "https://files.example"}})
	raw, err := signer.signChannelDeliveryFile(file, uuid.NewString(), ".txt")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/console/api/files/tools/:tool_file_id", (&HTTPHandler{manager: manager, signature: signer}).GetToolFile)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "private-result" {
		t.Fatalf("download=%d %s", recorder.Code, recorder.Body.String())
	}
	if backend.loadCalls != 1 {
		t.Fatalf("storage loads=%d, want 1", backend.loadCalls)
	}
}

func TestChannelDeliveryGrantRejectsInteractiveFilesAndCapsFileExpiry(t *testing.T) {
	manager, _, params, _ := executionFileFixture(t)
	file, err := manager.CreateFileByRaw(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	signer := NewFileSignature(&config.Config{App: config.AppConfig{SecretKey: "test-only", FilesURL: "https://files.example"}})
	if _, err := signer.signChannelDeliveryFile(file, uuid.NewString(), ".txt"); err == nil {
		t.Fatal("interactive artifact shared through channel grant")
	}
	i := file.ExecutionIdentity
	i.Version, i.ApplicationType, i.Entry, i.SubjectKind = execution.BackgroundIdentityVersion, "digital_human", "channel", "channel_conversation"
	expires := time.Now().Add(30 * time.Second)
	file.Lifecycle, file.ExpiresAt = string(ToolFileLifecycleTemporary), &expires
	raw, err := signer.signChannelDeliveryFile(file, uuid.NewString(), ".txt")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	if u.Query().Get("expires_at") != strconv.FormatInt(expires.Unix(), 10) {
		t.Fatal("grant outlived original file")
	}
	signer.filesURL = ""
	if _, err := signer.signChannelDeliveryFile(file, uuid.NewString(), ".txt"); err == nil {
		t.Fatal("missing download origin accepted")
	}
}

func TestChannelDeliveryTTLDoesNotChangeOrdinaryFileTTL(t *testing.T) {
	manager, _, params, _ := executionFileFixture(t)
	name := "ordinary.txt"
	params.ExecutionIdentity = nil
	params.ExecutionMessageID = nil
	params.Filename = &name
	file, err := manager.CreateFileByRaw(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	signer := NewFileSignature(&config.Config{App: config.AppConfig{
		SecretKey: "test-channel-grant", FilesURL: "https://files.example", FilesAccessTimeout: 3600,
	}})
	ordinary, err := signer.SignToolFile(file.ID, ".txt")
	if err != nil {
		t.Fatal(err)
	}
	ordinaryURL, err := url.Parse(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	expires, err := strconv.ParseInt(ordinaryURL.Query().Get("expires_at"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if expires <= time.Now().Add(59*time.Minute).Unix() || expires > time.Now().Add(time.Hour).Unix() {
		t.Fatalf("ordinary file expiry=%d, want within (59m, 1h]", expires)
	}
}
