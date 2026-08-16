package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProviderDefinitionRegistersWithFailClosedSend(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{Definition: ProviderDefinition(), Adapter: adapter, ConnectionTester: adapter, HealthProbe: adapter}); err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.ProviderDefinition(IntegrationID)
	if !ok || len(definition.Actions) != 5 {
		t.Fatalf("actions = %d", len(definition.Actions))
	}
	for _, action := range definition.Actions {
		if action.ID == ActionMessageSendUser && (action.DefaultPolicy.Enabled || action.SuccessDeduplication == nil) {
			t.Fatal("send action must be disabled by default and success-deduplicated")
		}
	}
}

func TestAdapterValidatesSearchesAndSendsWithTokenCache(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			tokenCalls.Add(1)
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/agent/get":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "agentid": 1000002, "name": "ZGI Assistant", "square_logo_url": "https://example.com/logo.png"})
		case "/cgi-bin/user/simplelist":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userlist": []map[string]interface{}{{"userid": "yangzhihang", "name": "杨志航", "department": []int{1}}}})
		case "/cgi-bin/user/get":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userid": "yangzhihang", "name": "杨志航", "department": []int{1}, "position": "Engineer", "status": 1})
		case "/cgi-bin/message/send":
			var payload map[string]interface{}
			json.NewDecoder(request.Body).Decode(&payload)
			if payload["touser"] != "yangzhihang" {
				t.Errorf("touser=%v", payload["touser"])
			}
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "msgid": "message-1"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	profile, err := adapter.ValidateConnection(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName != "ZGI Assistant" {
		t.Fatalf("profile=%#v", profile)
	}
	if profile.AccountID == "corp:1000002" || strings.Contains(profile.AccountID, "corp") {
		t.Fatalf("profile account ID must be pseudonymous: %#v", profile)
	}
	search, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "杨志航"}})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionContactSearch, search.Output)
	members := search.Output["members"].([]map[string]interface{})
	ref := members[0]["recipient_ref"].(string)
	user, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionUserGet, Connection: connection, Input: map[string]interface{}{"recipient_ref": ref}})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionUserGet, user.Output)
	sent, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionMessageSendUser, Connection: connection, Input: map[string]interface{}{"recipient_ref": ref, "content": "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(toJSON(sent.Output), "message-1") {
		t.Fatalf("output=%#v", sent.Output)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token calls=%d,want 1", tokenCalls.Load())
	}
}

func TestProviderInstallUsesV2PermanentCodeAndAuthInfo(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		paths = append(paths, request.URL.Path)
		if request.URL.Query().Get("suite_access_token") != "suite-token" {
			t.Errorf("suite_access_token=%q", request.URL.Query().Get("suite_access_token"))
		}
		switch request.URL.Path {
		case "/cgi-bin/service/get_pre_auth_code":
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "pre_auth_code": "pre-auth", "expires_in": 1200})
		case "/cgi-bin/service/v2/get_permanent_code":
			var payload map[string]string
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["auth_code"] != "one-time-code" {
				t.Errorf("auth_code=%q", payload["auth_code"])
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "permanent_code": "permanent", "auth_corp_info": map[string]any{"corpid": "wwcorp", "corp_name": "Example Corp"}})
		case "/cgi-bin/service/v2/get_auth_info":
			var payload map[string]string
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["auth_corpid"] != "wwcorp" || payload["permanent_code"] != "permanent" {
				t.Errorf("auth info payload=%#v", payload)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "auth_corp_info": map[string]any{"corpid": "wwcorp", "corp_name": "Example Corp"}, "auth_info": map[string]any{"agent": []map[string]any{{"agentid": 1000002, "name": "ZGI"}}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	authorizationURL, err := adapter.BuildProviderInstallAuthorizationURL(context.Background(), integrations.ProviderInstallAuthorizationRequest{
		RedirectURI: adapter.suite.InstallCallbackURL,
		State:       "0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "open.work.weixin.qq.com" || target.Query().Get("suite_id") != "ww_suite" || target.Query().Get("pre_auth_code") != "pre-auth" || target.Query().Get("state") != "0123456789abcdef" {
		t.Fatalf("authorization URL=%s", authorizationURL)
	}
	grant, err := adapter.ExchangeProviderInstallCode(context.Background(), integrations.ProviderInstallExchangeRequest{AuthorizationCode: "one-time-code"})
	if err != nil {
		t.Fatal(err)
	}
	defer grant.Destroy()
	if grant.Credentials["auth_corpid"] != "wwcorp" || grant.Credentials["permanent_code"] != "permanent" || grant.Credentials["agent_id"] != "1000002" {
		t.Fatalf("grant credentials=%#v", grant.Credentials)
	}
	if grant.ExternalTenantHash == "" || strings.Contains(grant.ExternalTenantHash, "wwcorp") {
		t.Fatalf("external tenant hash=%q", grant.ExternalTenantHash)
	}
	wantPaths := []string{"/cgi-bin/service/get_pre_auth_code", "/cgi-bin/service/v2/get_permanent_code", "/cgi-bin/service/v2/get_auth_info"}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("paths=%v want=%v", paths, wantPaths)
	}
}

func TestSuiteCallbackVerifiesSignatureDecryptsAndRejectsWrongReceiver(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	timestamp, nonce := "1723776000", "nonce-1"
	payload := []byte(`<xml><SuiteId>ww_suite</SuiteId><InfoType>cancel_auth</InfoType><AuthCorpId>wwcorp</AuthCorpId><TimeStamp>1723776000</TimeStamp></xml>`)
	encrypted := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, adapter.suite.SuiteID, payload)
	body, _ := xml.Marshal(encryptedCallbackEnvelope{Encrypt: encrypted})
	event, err := adapter.ReceiveSuiteCallback(context.Background(), callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encrypted), timestamp, nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.InfoType != "cancel_auth" || event.AuthCorpID != "wwcorp" {
		t.Fatalf("event=%#v", event)
	}
	if _, err := adapter.ReceiveSuiteCallback(context.Background(), "bad-signature", timestamp, nonce, body); integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("wrong signature error=%v", err)
	}
	wrongReceiver := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, "ww_other", payload)
	wrongBody, _ := xml.Marshal(encryptedCallbackEnvelope{Encrypt: wrongReceiver})
	if _, err := adapter.ReceiveSuiteCallback(context.Background(), callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, wrongReceiver), timestamp, nonce, wrongBody); integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("wrong receiver error=%v", err)
	}
}

func TestRecipientReferenceRejectsRawUserID(t *testing.T) {
	if _, err := decodeRecipientRef("yangzhihang"); err == nil {
		t.Fatal("raw provider user id must not be accepted as a recipient reference")
	}
}

func TestValidateConnectionPreservesSafeWeComDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"errcode":60020,"errmsg":"not allow to access from your ip","request_id":"req-wecom-1"}`))
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	_, err = adapter.ValidateConnection(context.Background(), &integrations.ResolvedConnection{
		ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID,
		Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("error = %v", err)
	}
	diagnostics := integrations.ProviderDiagnosticsFromError(err)
	if diagnostics.ErrorCode != "60020" || diagnostics.RequestID != "req-wecom-1" || diagnostics.HTTPStatus != http.StatusOK {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if strings.Contains(err.Error(), "not allow to access") {
		t.Fatalf("provider errmsg must not leak: %v", err)
	}
}

func TestWeComGuideCoversProviderInstallAndPermissionReview(t *testing.T) {
	guide := ProviderDefinition().AuthMethods[0].SetupGuide
	if guide == nil || !guide.ExpandedByDefault {
		t.Fatal("WeCom setup guide must be expanded")
	}
	ids := map[string]bool{}
	for _, step := range guide.Steps {
		ids[step.ID] = true
	}
	for _, id := range []string{"provider_configuration", "start_install", "review_permissions", "verify_connection"} {
		if !ids[id] {
			t.Fatalf("setup guide missing %q", id)
		}
	}
}

func configureSuiteTestAdapter(adapter *Adapter) {
	adapter.suite = SuiteConfig{
		SuiteID: "ww_suite", SuiteSecret: "suite-secret", CallbackToken: "token",
		EncodingAESKey:     "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		InstallCallbackURL: "https://example.com/console/api/integrations/wecom/install/callback",
	}
	adapter.suiteState = &SuiteStateRepository{}
	adapter.suiteToken = cachedToken{value: "suite-token", expiresAt: time.Now().Add(time.Hour)}
}

func encryptWeComCallbackForTest(t *testing.T, encodingAESKey, receiverID string, message []byte) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		t.Fatal(err)
	}
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(message)))
	plaintext := append(bytes.Repeat([]byte{0x42}, 16), length...)
	plaintext = append(plaintext, message...)
	plaintext = append(plaintext, []byte(receiverID)...)
	padding := 32 - len(plaintext)%32
	plaintext = append(plaintext, bytes.Repeat([]byte{byte(padding)}, padding)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(ciphertext, plaintext)
	return base64.StdEncoding.EncodeToString(ciphertext)
}
func toJSON(value interface{}) string { raw, _ := json.Marshal(value); return string(raw) }

func assertWeComOutputContract(t *testing.T, actionID string, output map[string]interface{}) {
	t.Helper()
	var schema map[string]interface{}
	for _, action := range ProviderDefinition().Actions {
		if action.ID == actionID {
			schema = action.OutputSchema
			break
		}
	}
	if schema == nil {
		t.Fatalf("action %q was not found", actionID)
	}
	normalized, err := tools.NormalizeJSONValue(output)
	if err != nil {
		t.Fatalf("normalize %s output: %v", actionID, err)
	}
	if err := tools.ValidateJSONSchemaValue(schema, normalized); err != nil {
		t.Fatalf("%s output contract: %v; output=%#v", actionID, err, normalized)
	}
}
