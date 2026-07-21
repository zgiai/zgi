package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/util"
)

type modelPrecheckFake struct {
	result *llmclient.AppModelPrecheckResult
	err    error
	appCtx *llmclient.AppContext
	models []llmclient.AppModelRef
}

func (f *modelPrecheckFake) PrecheckAppModels(
	_ context.Context,
	appCtx *llmclient.AppContext,
	models []llmclient.AppModelRef,
) (*llmclient.AppModelPrecheckResult, error) {
	f.appCtx = appCtx
	f.models = append([]llmclient.AppModelRef(nil), models...)
	return f.result, f.err
}

func TestWorkChatModelPrecheckReturnsModelLevelWarning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	prechecker := &modelPrecheckFake{result: &llmclient.AppModelPrecheckResult{
		Status: llmclient.AppModelPrecheckStatusWarning,
		Warnings: []llmclient.AppModelPrecheckWarning{{
			Kind:   llmclient.AppModelPrecheckWarningPrivateChannelUpstreamUnavailable,
			Reason: "invalid_api_key",
			Scope:  llmclient.AppModelPrecheckWarningScopeAll,
		}},
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", accountID)
		util.SetOrganizationID(c, organizationID)
		c.Next()
	})
	NewHandler(nil, prechecker).RegisterRoutes(router.Group("/console/api"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/console/api/aichat/work-chat/models/precheck",
		bytes.NewBufferString(`{"provider":"deepseek","model":"deepseek-chat"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	var body struct {
		Code string `json:"code"`
		Data struct {
			Status   llmclient.AppModelPrecheckStatus `json:"status"`
			Warnings []struct {
				Kind   llmclient.AppModelPrecheckWarningKind  `json:"kind"`
				Reason string                                 `json:"reason"`
				Scope  llmclient.AppModelPrecheckWarningScope `json:"scope"`
			} `json:"warnings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusOK || body.Code != "0" {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if body.Data.Status != llmclient.AppModelPrecheckStatusWarning || len(body.Data.Warnings) != 1 {
		t.Fatalf("precheck = %#v, want one warning", body.Data)
	}
	warning := body.Data.Warnings[0]
	if warning.Kind != llmclient.AppModelPrecheckWarningPrivateChannelUpstreamUnavailable ||
		warning.Reason != "invalid_api_key" ||
		warning.Scope != llmclient.AppModelPrecheckWarningScopeAll {
		t.Fatalf("warning = %#v, want upstream unavailable with all scope", warning)
	}
	if len(prechecker.models) != 1 || prechecker.models[0].Provider != "deepseek" || prechecker.models[0].Model != "deepseek-chat" {
		t.Fatalf("models = %#v, want selected DeepSeek model", prechecker.models)
	}
	if prechecker.appCtx == nil ||
		prechecker.appCtx.OrganizationID != organizationID ||
		prechecker.appCtx.AccountID != accountID ||
		prechecker.appCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization ||
		prechecker.appCtx.AppType != workChatPrecheckAppType ||
		prechecker.appCtx.ModelUseCase != workChatPrecheckModelUseCase {
		t.Fatalf("app context = %#v, want current organization and account", prechecker.appCtx)
	}
}

func TestWorkChatModelPrecheckRejectsBlankModel(t *testing.T) {
	prechecker := &modelPrecheckFake{}
	recorder := runWorkChatModelPrecheckRequest(t, prechecker, `{"provider":"deepseek","model":" "}`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400", recorder.Code, recorder.Body.String())
	}
	if len(prechecker.models) != 0 {
		t.Fatalf("models = %#v, want prechecker not called", prechecker.models)
	}
}

func TestWorkChatModelPrecheckDegradesErrorsToUnknownWithoutBlocking(t *testing.T) {
	prechecker := &modelPrecheckFake{err: errors.New("precheck failed")}
	recorder := runWorkChatModelPrecheckRequest(t, prechecker, `{"provider":"deepseek","model":"deepseek-chat"}`)

	var body struct {
		Code string `json:"code"`
		Data struct {
			Status   llmclient.AppModelPrecheckStatus `json:"status"`
			Warnings []workChatModelPrecheckWarning   `json:"warnings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusOK || body.Code != "0" {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if body.Data.Status != llmclient.AppModelPrecheckStatusUnknown || len(body.Data.Warnings) != 0 {
		t.Fatalf("precheck = %#v, want unknown with no warnings", body.Data)
	}
}

func runWorkChatModelPrecheckRequest(
	t *testing.T,
	prechecker llmclient.AppModelPrechecker,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", uuid.NewString())
		util.SetOrganizationID(c, uuid.NewString())
		c.Next()
	})
	NewHandler(nil, prechecker).RegisterRoutes(router.Group("/console/api"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/console/api/aichat/work-chat/models/precheck",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}
