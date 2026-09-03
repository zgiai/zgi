package wecom

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

func TestOfficialMemberSearchUsesProviderTokenAndReturnsBoundReferences(t *testing.T) {
	var tokenCalls atomic.Int32
	var searchCalls atomic.Int32
	var corpTokenCalls atomic.Int32
	var userGetCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_provider_token":
			tokenCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "provider_access_token": "provider-token", "expires_in": 7200})
		case "/cgi-bin/service/get_corp_token":
			corpTokenCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "corp-token", "expires_in": 7200})
		case "/cgi-bin/service/contact/search":
			searchCalls.Add(1)
			if request.Method != http.MethodPost || request.URL.Query().Get("provider_access_token") != "provider-token" {
				t.Errorf("search request=%s %s", request.Method, request.URL.String())
			}
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["auth_corpid"] != "corp" || payload["agentid"] != float64(1000002) || payload["query_type"] != float64(1) || payload["query_range"] != float64(0) {
				t.Errorf("search payload=%#v", payload)
			}
			if payload["query_word"] != "张三" || payload["cursor"] != "cursor-1" || payload["full_match_field"] != float64(1) || payload["limit"] != float64(2) {
				t.Errorf("search query payload=%#v", payload)
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"query_result": map[string]interface{}{"user": map[string]interface{}{
					"userid":      []string{"User-A", "User-B"},
					"open_userid": []string{"Open-A", "Open-B"},
				}},
				"is_last": false, "next_cursor": "cursor-2",
			})
		case "/cgi-bin/user/get":
			userGetCalls.Add(1)
			if request.URL.Query().Get("access_token") != "corp-token" {
				t.Errorf("user get query=%s", request.URL.RawQuery)
			}
			userID := request.URL.Query().Get("userid")
			departments := []int{7}
			name := "张三甲"
			if userID == "User-B" {
				departments = []int{8, 9}
				name = "张三乙"
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userid": userID, "name": name, "department": departments})
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
	connection := testWeComConnection("connection", "corp", "1000002")
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionMemberSearch,
		Connection:    connection,
		Input: map[string]interface{}{
			"query": "张三", "match_mode": "exact", "max_results": 2, "cursor": "cursor-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionContactSearch, result.Output)
	if result.ResultCount != 2 || result.Output["is_last"] != false || result.Output["next_cursor"] != "cursor-2" || result.Output["requires_selection"] != true {
		t.Fatalf("search output=%#v", result.Output)
	}
	members := result.Output["members"].([]map[string]interface{})
	for index, member := range members {
		reference := member["recipient_ref"].(string)
		if strings.Contains(reference, "User-") || strings.Contains(reference, "Open-") {
			t.Fatalf("reference exposed a raw provider identifier: %q", reference)
		}
		claims, decodeErr := adapter.decodeSubjectRef(reference, mustParseCredentials(t, connection), SubjectTypeMember, SubjectIDKindUserID)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		wantSuffix := string(rune('A' + index))
		if claims.SubjectID != "User-"+wantSuffix || claims.AlternateIDKind != SubjectIDKindOpenUserID || claims.AlternateID != "Open-"+wantSuffix {
			t.Fatalf("claims=%#v", claims)
		}
		wantName := "张三甲"
		wantDepartments := []int{7}
		if index == 1 {
			wantName = "张三乙"
			wantDepartments = []int{8, 9}
		}
		if member["name"] != wantName || !equalIntSlices(member["department_ids"].([]int), wantDepartments) {
			t.Fatalf("enriched member=%#v", member)
		}
	}
	if tokenCalls.Load() != 1 || searchCalls.Load() != 1 || corpTokenCalls.Load() != 1 || userGetCalls.Load() != 2 {
		t.Fatalf("provider token=%d search=%d corp token=%d user get=%d", tokenCalls.Load(), searchCalls.Load(), corpTokenCalls.Load(), userGetCalls.Load())
	}
}

func TestOfficialMemberSearchRejectsMismatchedIdentifierLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_provider_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "provider_access_token": "provider-token", "expires_in": 7200})
		case "/cgi-bin/service/contact/search":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"query_result": map[string]interface{}{"user": map[string]interface{}{
					"userid": []string{"user-a", "user-b"}, "open_userid": []string{"open-a"},
				}},
				"is_last": true,
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, _ := newForBaseURL(server.Client(), server.URL)
	configureSuiteTestAdapter(adapter)
	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionContactSearch,
		Connection: testWeComConnection("connection", "corp", "1000002"),
		Input:      map[string]interface{}{"query": "user"},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid {
		t.Fatalf("search mismatch error=%v", err)
	}
}

func TestOfficialMemberSearchRejectsMismatchedMemberDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_provider_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "provider_access_token": "provider-token", "expires_in": 7200})
		case "/cgi-bin/service/contact/search":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "query_result": map[string]interface{}{"user": map[string]interface{}{"userid": []string{"user-a"}}}, "is_last": true,
			})
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "corp-token", "expires_in": 7200})
		case "/cgi-bin/user/get":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userid": "user-b", "name": "Wrong member", "department": []int{7}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, _ := newForBaseURL(server.Client(), server.URL)
	configureSuiteTestAdapter(adapter)
	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionContactSearch,
		Connection: testWeComConnection("connection", "corp", "1000002"),
		Input:      map[string]interface{}{"query": "user"},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid {
		t.Fatalf("mismatched member details error=%v", err)
	}
}

func TestSubjectReferenceV2RejectsTamperingAndBindingChanges(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	base := testWeComConnection("connection-1", "corp-1", "1000002")
	creds := mustParseCredentials(t, base)
	reference, err := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "user-1", SubjectIDKindOpenUserID, "open-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.decodeSubjectRef(reference, creds, SubjectTypeMember, SubjectIDKindUserID); err != nil {
		t.Fatal(err)
	}
	secondReference, err := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "user-1", SubjectIDKindOpenUserID, "open-1")
	if err != nil {
		t.Fatal(err)
	}
	if secondReference == reference {
		t.Fatal("subject references reused a nonce for identical claims")
	}
	parts := strings.Split(reference, ".")
	if len(parts) != 3 || parts[0] != "v2" {
		t.Fatalf("reference envelope=%q", reference)
	}
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(parts[1])
	ciphertext, ciphertextErr := base64.RawURLEncoding.DecodeString(parts[2])
	if nonceErr != nil || ciphertextErr != nil {
		t.Fatalf("reference envelope could not be decoded: nonce=%v ciphertext=%v", nonceErr, ciphertextErr)
	}
	for _, providerID := range [][]byte{[]byte("user-1"), []byte("open-1")} {
		if bytes.Contains(nonce, providerID) || bytes.Contains(ciphertext, providerID) {
			t.Fatalf("reference envelope exposed provider identifier %q", providerID)
		}
	}
	for name, connection := range map[string]*integrations.ResolvedConnection{
		"connection": testWeComConnection("connection-2", "corp-1", "1000002"),
		"tenant":     testWeComConnection("connection-1", "corp-2", "1000002"),
		"agent":      testWeComConnection("connection-1", "corp-1", "1000003"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.decodeSubjectRef(reference, mustParseCredentials(t, connection), SubjectTypeMember, SubjectIDKindUserID); err == nil {
				t.Fatal("cross-binding reference was accepted")
			}
		})
	}
	tamperSuffix := "A"
	if strings.HasSuffix(reference, tamperSuffix) {
		tamperSuffix = "B"
	}
	tampered := reference[:len(reference)-1] + tamperSuffix
	if _, err := adapter.decodeSubjectRef(tampered, creds, SubjectTypeMember, SubjectIDKindUserID); err == nil {
		t.Fatal("tampered reference was accepted")
	}
	legacy := encodeLegacyRecipientRefForTest("legacy-user")
	if _, err := adapter.decodeSubjectRef(legacy, creds, SubjectTypeMember, SubjectIDKindUserID); err == nil {
		t.Fatal("subject decoder accepted a legacy reference")
	}
	if _, err := adapter.getUser(context.Background(), creds, legacy); integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("get_wecom_user legacy reference error=%v", err)
	}
}

func equalIntSlices(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func encodeLegacyRecipientRefForTest(userID string) string {
	raw, _ := json.Marshal(map[string]string{"v": "1", "u": userID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestSendUserReportsPartialAcceptanceWithoutLeakingProviderIDs(t *testing.T) {
	var sendPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "corp-token", "expires_in": 7200})
		case "/cgi-bin/message/send":
			_ = json.NewDecoder(request.Body).Decode(&sendPayload)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "msgid": "message-1",
				"invaliduser": "USER-B", "unlicenseduser": "user-c",
				"invalidparty": "7", "invalidtag": "8",
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, _ := newForBaseURL(server.Client(), server.URL)
	configureSuiteTestAdapter(adapter)
	connection := testWeComConnection("connection", "corp", "1000002")
	creds := mustParseCredentials(t, connection)
	references := make([]string, 0, 3)
	for _, userID := range []string{"user-a", "user-b", "user-c"} {
		reference, err := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, userID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		references = append(references, reference)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionMessageSendUser, Connection: connection,
		Input: map[string]interface{}{"recipient_refs": references, "content": "你好", "safe": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionMessageSendUser, result.Output)
	if sendPayload["touser"] != "user-a|user-b|user-c" || sendPayload["safe"] != float64(1) {
		t.Fatalf("send payload=%#v", sendPayload)
	}
	message := result.Output["message"].(map[string]interface{})
	if result.ResultCount != 1 || message["accepted_count"] != 1 || message["rejected_count"] != 2 || message["operation_status"] != "partially_succeeded" {
		t.Fatalf("partial send result=%#v", result)
	}
	encoded, _ := json.Marshal(result.Output)
	if strings.Contains(string(encoded), "user-b") || strings.Contains(string(encoded), "user-c") {
		t.Fatalf("send output leaked provider user IDs: %s", encoded)
	}
}

func TestSendUserPreserves81013DiagnosticsAndRejectsUnconfirmedSuccess(t *testing.T) {
	t.Run("all recipients rejected", func(t *testing.T) {
		adapter, connection := newMessageTestAdapter(t, map[string]interface{}{"errcode": 81013, "request_id": "request-81013"})
		reference, _ := adapter.encodeSubjectRef(mustParseCredentials(t, connection), SubjectTypeMember, SubjectIDKindUserID, "user-a", "", "")
		_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			IntegrationID: IntegrationID, ActionID: ActionMessageSendUser, Connection: connection,
			Input: map[string]interface{}{"recipient_ref": reference, "content": "hello"},
		})
		if integrations.ErrorCode(err) != integrations.ErrorCodeProviderRejected {
			t.Fatalf("81013 error=%v", err)
		}
		integrationError, ok := err.(*integrations.Error)
		if !ok || integrationError.ReasonCode != "wecom_all_recipients_invalid_or_unauthorized" {
			t.Fatalf("81013 reason=%#v", err)
		}
		diagnostics := integrations.ProviderDiagnosticsFromError(err)
		if diagnostics.ErrorCode != "81013" || diagnostics.RequestID != "request-81013" {
			t.Fatalf("81013 diagnostics=%#v", diagnostics)
		}
	})

	t.Run("missing message id", func(t *testing.T) {
		adapter, connection := newMessageTestAdapter(t, map[string]interface{}{"errcode": 0})
		reference, _ := adapter.encodeSubjectRef(mustParseCredentials(t, connection), SubjectTypeMember, SubjectIDKindUserID, "user-a", "", "")
		_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			IntegrationID: IntegrationID, ActionID: ActionMessageSendUser, Connection: connection,
			Input: map[string]interface{}{"recipient_ref": reference, "content": "hello"},
		})
		if integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid {
			t.Fatalf("missing msgid error=%v", err)
		}
	})
}

func TestSendUserEnforcesUTF8ByteLimitAndV2MemberDomain(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := testWeComConnection("connection", "corp", "1000002")
	creds := mustParseCredentials(t, connection)
	memberReference, _ := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "user-a", "", "")
	_, _, err = adapter.sendUser(context.Background(), creds, map[string]interface{}{
		"recipient_ref": memberReference, "content": strings.Repeat("你", 683),
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("UTF-8 byte limit error=%v", err)
	}
	openReference, _ := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindOpenUserID, "open-a", "", "")
	if _, _, err := adapter.sendUser(context.Background(), creds, map[string]interface{}{"recipient_ref": openReference, "content": "hello"}); integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("open_userid write error=%v", err)
	}
	legacyReference := encodeLegacyRecipientRefForTest("legacy-user")
	if _, _, err := adapter.sendUser(context.Background(), creds, map[string]interface{}{"recipient_ref": legacyReference, "content": "hello"}); integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("legacy write error=%v", err)
	}
}

func newMessageTestAdapter(t *testing.T, sendResponse map[string]interface{}) (*Adapter, *integrations.ResolvedConnection) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "corp-token", "expires_in": 7200})
		case "/cgi-bin/message/send":
			_ = json.NewEncoder(writer).Encode(sendResponse)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	return adapter, testWeComConnection("connection", "corp", "1000002")
}

func testWeComConnection(connectionID, authCorpID, agentID string) *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{
		ID: connectionID, IntegrationID: IntegrationID, DriverID: DriverID,
		CredentialVersion: 1, Revision: 1,
		Credentials: map[string]string{"auth_corpid": authCorpID, "agent_id": agentID, "permanent_code": "permanent"},
	}
}

func mustParseCredentials(t *testing.T, connection *integrations.ResolvedConnection) credentials {
	t.Helper()
	creds, err := parseCredentials(connection)
	if err != nil {
		t.Fatal(err)
	}
	return creds
}
