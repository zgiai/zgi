package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"io"
	"net/http"
	"strings"
	"testing"
)

type resourceStoreFixture struct {
	reads, saves int
	scope        providers.ResourceScope
	deny         bool
	content      []byte
	id           string
}

func (s *resourceStoreFixture) Read(_ context.Context, scope providers.ResourceScope, id string) (ResourceFile, error) {
	s.reads++
	s.scope = scope
	if s.deny {
		return ResourceFile{}, errors.New("private file")
	}
	return ResourceFile{Name: "../../not-used.txt", Content: append([]byte{}, s.content...)}, nil
}
func (s *resourceStoreFixture) Save(_ context.Context, scope providers.ResourceScope, f ResourceFile) (string, error) {
	s.saves++
	s.scope = scope
	s.content = append([]byte{}, f.Content...)
	return s.id, nil
}
func resourceInput() providers.CredentialInput {
	input := fixtureInput()
	input.ResourceScope = providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}}
	return input
}

func TestResourcesRequireAuthorizedPlatformScopeBeforeNetwork(t *testing.T) {
	for _, withScope := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing runtime scope", true: "foreign file"}[withScope], func(t *testing.T) {
			store := &resourceStoreFixture{deny: true}
			c, _ := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("unauthorized file reached provider")
				return nil, nil
			})}, store)
			input := fixtureInput()
			if withScope {
				input = resourceInput()
			}
			_, err := c.InvokeCredentials(context.Background(), "feishu.resource.upload", input, map[string]any{"file_id": uuid.NewString()})
			if err == nil {
				t.Fatal("unauthorized resource allowed")
			}
		})
	}
}
func TestResourceUploadUsesBoundedMultipartAndReturnsOnlyReference(t *testing.T) {
	store := &resourceStoreFixture{content: []byte("hello authorized file")}
	calls := 0
	c, _ := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return fixtureResponse(`{"code":0,"tenant_access_token":"token-fixture","expire":7200}`), nil
		}
		if r.URL.Path != "/open-apis/im/v1/files" || r.Method != "POST" {
			t.Fatal("upload route")
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		parts := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(part)
			parts[part.FormName()] = string(body)
			if part.FileName() != "" && part.FileName() != "feishu-resource.txt" {
				t.Fatal("unsanitized filename")
			}
		}
		if parts["file_type"] != "stream" || parts["file"] != "hello authorized file" {
			t.Fatal("multipart fields")
		}
		return fixtureResponse(`{"code":0,"data":{"file_key":"file_fixture","app_secret":"should-not-expose"}}`), nil
	})}, store)
	input := resourceInput()
	res, err := c.InvokeCredentials(context.Background(), "feishu.resource.upload", input, map[string]any{"file_id": uuid.NewString()})
	if err != nil || calls != 2 || store.scope != input.ResourceScope {
		t.Fatalf("resource upload %v", err)
	}
	raw, _ := json.Marshal(res.Value)
	if strings.Contains(string(raw), "hello") || strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "not-used") {
		t.Fatal("binary or secret output")
	}
}
func TestResourceDownloadStoresPrivateReferenceAndRejectsOversizeAndAPIError(t *testing.T) {
	for _, mode := range []string{"ok", "oversize", "api error", "json receipt", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			store := &resourceStoreFixture{id: uuid.NewString()}
			calls := 0
			c, _ := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return fixtureResponse(`{"code":0,"tenant_access_token":"token-fixture","expire":7200}`), nil
				}
				if r.URL.Path != "/open-apis/im/v1/messages/om_id/resources/file_id" || r.URL.Query().Get("type") != "file" {
					t.Fatal("resource scope")
				}
				res := fixtureResponse("hello file")
				switch mode {
				case "json receipt":
					res = fixtureResponse(`{"code":0,"data":{"file_key":"not-a-download"}}`)
					res.Header.Set("Content-Type", "application/json")
				case "oversize":
					res.ContentLength = MaxResourceBytes + 1
				case "api error":
					res = fixtureResponse(`{"code":99991672}`)
					res.Header.Set("Content-Type", "application/json")
				case "unsupported":
					res = fixtureResponse("<html>not a file</html>")
				}
				return res, nil
			})}, store)
			res, err := c.InvokeCredentials(context.Background(), "feishu.resource.get", resourceInput(), map[string]any{"message_id": "om_id", "file_key": "file_id", "type": "file"})
			if mode == "ok" {
				if err != nil || store.saves != 1 {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(res.Value)
				if strings.Contains(string(raw), "hello file") || !strings.Contains(string(raw), store.id) {
					t.Fatal("binary instead of reference")
				}
			} else if err == nil || store.saves != 0 {
				t.Fatalf("invalid download persisted %v", err)
			}
		})
	}
}
