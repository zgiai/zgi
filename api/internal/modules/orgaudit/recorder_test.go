package orgaudit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const (
	testOrganizationID = "11111111-1111-1111-1111-111111111111"
	testActorID        = "22222222-2222-2222-2222-222222222222"
	testTargetID       = "33333333-3333-3333-3333-333333333333"
	testWorkspaceID    = "44444444-4444-4444-4444-444444444444"
)

type captureWriter struct{ logs []Log }

func (w *captureWriter) Create(_ context.Context, logs []Log) error {
	w.logs = append(w.logs, logs...)
	return nil
}

func serve(t *testing.T, recorder *Recorder, route, target string, handler gin.HandlerFunc) {
	t.Helper()
	engine := gin.New()
	engine.POST(route, func(c *gin.Context) {
		c.Set("account_id", testActorID)
		c.Set("request_id", "req-1")
		c.Next()
	}, recorder.Wrap(ActionMemberStatusUpdate, handler))
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.RemoteAddr = "10.0.0.7:5555"
	engine.ServeHTTP(httptest.NewRecorder(), request)
}

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return fields
}

func TestRecorderWritesOneSuccessRecordWithRequestContext(t *testing.T) {
	writer := &captureWriter{}
	serve(t, NewRecorder(writer), "/organizations/:organization_id/members/:member_id/status",
		"/organizations/"+testOrganizationID+"/members/"+testTargetID+"/status",
		func(c *gin.Context) {
			SetBefore(c, map[string]any{"status": "active"})
			SetAfter(c, map[string]any{"status": "inactive", "password": "secret", "nested": map[string]any{"api_key": "k", "keep": 1}})
			c.JSON(http.StatusOK, gin.H{"code": "0"})
		})

	if len(writer.logs) != 1 {
		t.Fatalf("records = %d, want 1", len(writer.logs))
	}
	log := writer.logs[0]
	if log.OrganizationID.String() != testOrganizationID || log.ActorAccountID == nil || log.ActorAccountID.String() != testActorID {
		t.Fatalf("organization/actor = %s/%v", log.OrganizationID, log.ActorAccountID)
	}
	if log.TargetAccountID == nil || log.TargetAccountID.String() != testTargetID || log.TargetType != TargetMember {
		t.Fatalf("target = %v %q", log.TargetAccountID, log.TargetType)
	}
	if log.Result != ResultSuccess || log.Action != ActionMemberStatusUpdate {
		t.Fatalf("result/action = %s/%s", log.Result, log.Action)
	}
	if log.RequestID != "req-1" || log.IP != "10.0.0.7" {
		t.Fatalf("request_id/ip = %q/%q", log.RequestID, log.IP)
	}
	after := decode(t, log.After)
	if _, leaked := after["password"]; leaked {
		t.Fatal("password reached the audit record")
	}
	nested := after["nested"].(map[string]any)
	if _, leaked := nested["api_key"]; leaked || nested["keep"] != float64(1) {
		t.Fatalf("nested sanitize = %v", nested)
	}
	if decode(t, log.Before)["status"] != "active" {
		t.Fatalf("before = %s", log.Before)
	}
}

func TestRecorderResultFollowsResponseStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   Result
		count  int
	}{
		{http.StatusForbidden, ResultDenied, 1},
		{http.StatusBadRequest, ResultFailed, 1},
		{http.StatusInternalServerError, ResultFailed, 1},
		{http.StatusUnauthorized, "", 0},
	} {
		writer := &captureWriter{}
		serve(t, NewRecorder(writer), "/organizations/:organization_id/members/:member_id/status",
			"/organizations/"+testOrganizationID+"/members/"+testTargetID+"/status",
			func(c *gin.Context) { c.JSON(tc.status, gin.H{}) })
		if len(writer.logs) != tc.count {
			t.Fatalf("status %d: records = %d, want %d", tc.status, len(writer.logs), tc.count)
		}
		if tc.count == 1 && writer.logs[0].Result != tc.want {
			t.Fatalf("status %d: result = %s, want %s", tc.status, writer.logs[0].Result, tc.want)
		}
	}
}

func TestRecorderBatchWritesOneRecordPerTarget(t *testing.T) {
	writer := &captureWriter{}
	serve(t, NewRecorder(writer), "/organizations/:organization_id/roles/:role_id/apply-template",
		"/organizations/"+testOrganizationID+"/roles/role-1/apply-template",
		func(c *gin.Context) {
			MarkBatch(c)
			AddTarget(c, Target{AccountID: testTargetID, Type: TargetMember, ID: testTargetID, WorkspaceID: testWorkspaceID})
			AddTarget(c, Target{AccountID: testActorID, Type: TargetMember, ID: testActorID, Result: ResultFailed})
			c.JSON(http.StatusOK, gin.H{})
		})
	if len(writer.logs) != 2 {
		t.Fatalf("records = %d, want 2", len(writer.logs))
	}
	if writer.logs[0].Result != ResultSuccess || writer.logs[0].WorkspaceID == nil || writer.logs[0].WorkspaceID.String() != testWorkspaceID {
		t.Fatalf("first = %+v", writer.logs[0])
	}
	if writer.logs[1].Result != ResultFailed {
		t.Fatalf("second result = %s", writer.logs[1].Result)
	}
	if writer.logs[0].RequestID != writer.logs[1].RequestID {
		t.Fatal("batch records do not share the request id")
	}
}

func TestRecorderSkipsEmptyBatchAndSkippedRequests(t *testing.T) {
	writer := &captureWriter{}
	recorder := NewRecorder(writer)
	route := "/organizations/:organization_id/x"
	target := "/organizations/" + testOrganizationID + "/x"
	serve(t, recorder, route, target, func(c *gin.Context) {
		MarkBatch(c)
		c.JSON(http.StatusOK, gin.H{})
	})
	serve(t, recorder, route, target, func(c *gin.Context) {
		Skip(c)
		c.JSON(http.StatusOK, gin.H{})
	})
	if len(writer.logs) != 0 {
		t.Fatalf("records = %d, want 0", len(writer.logs))
	}
}

func TestRecorderNeedsAnOrganization(t *testing.T) {
	writer := &captureWriter{}
	serve(t, NewRecorder(writer), "/public/invites/:token/accept", "/public/invites/t/accept", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})
	if len(writer.logs) != 0 {
		t.Fatalf("records = %d, want 0 without an organization", len(writer.logs))
	}

	serve(t, NewRecorder(writer), "/public/invites/:token/accept", "/public/invites/t/accept", func(c *gin.Context) {
		SetOrganization(c, testOrganizationID)
		SetTarget(c, Target{AccountID: testActorID, Type: TargetMember, ID: testActorID})
		c.JSON(http.StatusOK, gin.H{})
	})
	if len(writer.logs) != 1 || writer.logs[0].OrganizationID.String() != testOrganizationID {
		t.Fatalf("records = %+v", writer.logs)
	}
}

func TestNilRecorderLeavesHandlerUnwrapped(t *testing.T) {
	var recorder *Recorder
	called := false
	handler := recorder.Wrap(ActionRoleCreate, func(*gin.Context) { called = true })
	handler(&gin.Context{})
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRecorderResolvesCurrentOrganizationFromSession(t *testing.T) {
	writer := &captureWriter{}
	engine := gin.New()
	engine.POST("/organizations/:organization_id/roles", func(c *gin.Context) {
		c.Set("account_id", testActorID)
		c.Set("organization_id", testOrganizationID)
	}, NewRecorder(writer).Wrap(ActionRoleCreate, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) }))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/organizations/current/roles", nil))
	if len(writer.logs) != 1 || writer.logs[0].OrganizationID.String() != testOrganizationID {
		t.Fatalf("records = %+v", writer.logs)
	}
}
