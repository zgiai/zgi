package aichat_test

import (
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	aichatmodule "github.com/zgiai/ginext/internal/modules/aichat"
	aichatdto "github.com/zgiai/ginext/internal/modules/aichat/dto"
	aichathandler "github.com/zgiai/ginext/internal/modules/aichat/handler"
	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	aichatservice "github.com/zgiai/ginext/internal/modules/aichat/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/modules/skills"
	"github.com/zgiai/ginext/middleware"
	"gorm.io/gorm"
)

type fakeAIChatService struct {
	conversationLimit int
	messageLimit      int
	streamEvents      []aichatservice.StreamEvent
	stopResult        *aichatservice.StopConversationResult
	skillMetadata     []skills.SkillDiscoveryMetadata
	skillDetail       *skills.SkillDiscoveryMetadata
	skillDetailErr    error
	skillConfig       *aichatservice.SkillConfig
	updateSkillConfig *aichatservice.SkillConfig
	updateSkillErr    error
	importSkill       *skills.SkillDiscoveryMetadata
	importSkillErr    error
	deleteSkillErr    error
	deleteSkillCalled bool
	conversations     []*aichatmodel.Conversation
	getConversation   *aichatmodel.Conversation
}

func (f *fakeAIChatService) CreateConversation(context.Context, aichatservice.Scope, string) (*aichatmodel.Conversation, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) ListConversations(_ context.Context, _ aichatservice.Scope, _ int, limit int) ([]*aichatmodel.Conversation, int64, error) {
	f.conversationLimit = limit
	if f.conversations != nil {
		return f.conversations, int64(len(f.conversations)), nil
	}
	return []*aichatmodel.Conversation{}, 250, nil
}

func (f *fakeAIChatService) GetConversation(context.Context, aichatservice.Scope, uuid.UUID) (*aichatmodel.Conversation, error) {
	if f.getConversation != nil {
		return f.getConversation, nil
	}
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) UpdateConversation(context.Context, aichatservice.Scope, uuid.UUID, aichatdto.UpdateConversationRequest) (*aichatmodel.Conversation, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) DeleteConversation(context.Context, aichatservice.Scope, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeAIChatService) ListMessages(_ context.Context, _ aichatservice.Scope, _ uuid.UUID, _ int, limit int) ([]*aichatmodel.Message, int64, error) {
	f.messageLimit = limit
	return []*aichatmodel.Message{}, 450, nil
}

func (f *fakeAIChatService) DeleteMessage(context.Context, aichatservice.Scope, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeAIChatService) StopMessage(context.Context, aichatservice.Scope, uuid.UUID) (*aichatmodel.Message, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) StopConversation(context.Context, aichatservice.Scope, uuid.UUID) (*aichatservice.StopConversationResult, error) {
	if f.stopResult != nil {
		return f.stopResult, nil
	}
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) PrepareChat(context.Context, aichatservice.Scope, aichatdto.ChatRequest) (*aichatservice.PreparedChat, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) PrepareRootRegeneration(context.Context, aichatservice.Scope, uuid.UUID, aichatdto.RegenerateMessageRequest) (*aichatservice.PreparedChat, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) RunPreparedStream(context.Context, *aichatservice.PreparedChat, func(string) error, ...func(aichatservice.StreamEvent) error) (*aichatservice.ChatResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAIChatService) StreamConversationEvents(_ context.Context, _ aichatservice.Scope, _ uuid.UUID, _ uuid.UUID, _ string, onEvent func(aichatservice.StreamEvent) error) error {
	for _, event := range f.streamEvents {
		if err := onEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeAIChatService) ListSkills(context.Context, aichatservice.Scope) ([]skills.SkillDiscoveryMetadata, error) {
	return f.skillMetadata, nil
}

func (f *fakeAIChatService) GetSkill(context.Context, aichatservice.Scope, string) (*skills.SkillDiscoveryMetadata, error) {
	if f.skillDetailErr != nil {
		return nil, f.skillDetailErr
	}
	return f.skillDetail, nil
}

func (f *fakeAIChatService) GetSkillConfig(context.Context, aichatservice.Scope) (*aichatservice.SkillConfig, error) {
	if f.skillConfig != nil {
		return f.skillConfig, nil
	}
	return &aichatservice.SkillConfig{EnabledSkillIDs: []string{}}, nil
}

func (f *fakeAIChatService) UpdateSkillConfig(context.Context, aichatservice.Scope, aichatdto.UpdateSkillConfigRequest) (*aichatservice.SkillConfig, error) {
	if f.updateSkillErr != nil {
		return nil, f.updateSkillErr
	}
	if f.updateSkillConfig != nil {
		return f.updateSkillConfig, nil
	}
	return &aichatservice.SkillConfig{EnabledSkillIDs: []string{}}, nil
}

func (f *fakeAIChatService) ImportCustomSkill(context.Context, aichatservice.Scope, *multipart.FileHeader) (*skills.SkillDiscoveryMetadata, error) {
	if f.importSkillErr != nil {
		return nil, f.importSkillErr
	}
	if f.importSkill != nil {
		return f.importSkill, nil
	}
	return &skills.SkillDiscoveryMetadata{ID: "brief-writer", Name: "brief-writer", Source: skills.SkillSourceCustom}, nil
}

func (f *fakeAIChatService) DeleteSkill(context.Context, aichatservice.Scope, string) error {
	f.deleteSkillCalled = true
	if f.deleteSkillErr != nil {
		return f.deleteSkillErr
	}
	return nil
}

func (f *fakeAIChatService) CleanupStaleActiveMessages(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeAIChatService) MigrateWebAppConversation(context.Context, aichatservice.Scope, uuid.UUID) (*aichatmodel.Conversation, error) {
	return nil, errors.New("not implemented")
}

func TestHandler_ListSkillsReturnsMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		skillMetadata: []skills.SkillDiscoveryMetadata{
			{
				ID:          "calculator",
				Name:        "calculator",
				Description: "Calculate deterministic arithmetic.",
				WhenToUse:   "Use for arithmetic.",
				Display: skills.SkillDisplayMetadata{
					Icon:        "calculator",
					Category:    "productivity",
					Label:       map[string]string{"en_US": "Calculator"},
					Description: map[string]string{"en_US": "Calculate deterministic arithmetic."},
					WhenToUse:   map[string]string{"en_US": "Use for arithmetic."},
				},
				RuntimeType:      skills.SkillRuntimeTypeTool,
				Enabled:          true,
				HasTools:         true,
				MaxCallsPerTurn:  3,
				TimeoutSeconds:   5,
				ScriptsSupported: false,
			},
		},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/skills", nil)

	h.ListSkills(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Data []aichatdto.SkillResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	if len(body.Data) != 1 || body.Data[0].SkillID != "calculator" || !body.Data[0].HasTools {
		t.Fatalf("skills = %#v, want calculator metadata", body.Data)
	}
	if body.Data[0].RuntimeType != skills.SkillRuntimeTypeTool || !body.Data[0].Enabled {
		t.Fatalf("skill runtime/enabled = %#v, want enabled tool skill", body.Data[0])
	}
	if body.Data[0].Display.Icon != "calculator" || body.Data[0].Display.Label["en_US"] != "Calculator" {
		t.Fatalf("display = %#v, want calculator display metadata", body.Data[0].Display)
	}
}

func TestHandler_RegisterRoutesIncludesSkillDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		skillMetadata: []skills.SkillDiscoveryMetadata{{ID: "calculator", Name: "calculator", HasTools: true}},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setAIChatScope(c)
	})
	aichathandler.NewHandler(fakeSvc).RegisterRoutes(router.Group("/console/api"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/api/aichat/skills", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
}

func TestHandler_SkillWriteRoutesRequireOrganizationAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "update config", method: http.MethodPut, path: "/console/api/aichat/skills/config", body: `{"enabled_skill_ids":["calculator"]}`},
		{name: "import skill", method: http.MethodPost, path: "/console/api/aichat/skills/import"},
		{name: "delete skill", method: http.MethodDelete, path: "/console/api/aichat/skills/brief-writer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeSvc := &fakeAIChatService{}
			router := newAIChatPermissionRouter(fakeSvc, false)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s, want 403", w.Code, w.Body.String())
			}
			if fakeSvc.deleteSkillCalled {
				t.Fatalf("delete skill was called for unauthorized request")
			}
		})
	}
}

func TestHandler_SkillReadRoutesAllowOrganizationMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		skillMetadata: []skills.SkillDiscoveryMetadata{{ID: "calculator", Name: "calculator"}},
		skillDetail:   &skills.SkillDiscoveryMetadata{ID: "calculator", Name: "calculator"},
		skillConfig:   &aichatservice.SkillConfig{EnabledSkillIDs: []string{"calculator"}},
	}
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "list skills", path: "/console/api/aichat/skills"},
		{name: "get config", path: "/console/api/aichat/skills/config"},
		{name: "get skill", path: "/console/api/aichat/skills/calculator"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newAIChatPermissionRouter(fakeSvc, false)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)

			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandler_SkillWriteRoutesAllowOrganizationAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		updateSkillConfig: &aichatservice.SkillConfig{EnabledSkillIDs: []string{"calculator"}},
	}
	router := newAIChatPermissionRouter(fakeSvc, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/console/api/aichat/skills/config", strings.NewReader(`{"enabled_skill_ids":["calculator"]}`))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
}

func TestHandler_GetSkillReturnsMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		skillDetail: &skills.SkillDiscoveryMetadata{
			ID:          "calculator",
			Name:        "calculator",
			Description: "Calculate deterministic arithmetic.",
			WhenToUse:   "Use for arithmetic.",
			Display: skills.SkillDisplayMetadata{
				Icon:        "calculator",
				Category:    "productivity",
				Label:       map[string]string{"en_US": "Calculator"},
				Description: map[string]string{"en_US": "Calculate deterministic arithmetic."},
				WhenToUse:   map[string]string{"en_US": "Use for arithmetic."},
			},
			RuntimeType:     skills.SkillRuntimeTypeTool,
			Enabled:         true,
			HasTools:        true,
			MaxCallsPerTurn: 3,
			TimeoutSeconds:  5,
		},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Params = gin.Params{{Key: "id", Value: "calculator"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/skills/calculator", nil)

	h.GetSkill(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Data aichatdto.SkillResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	if body.Data.SkillID != "calculator" || !body.Data.HasTools {
		t.Fatalf("skill = %#v, want calculator metadata", body.Data)
	}
	if body.Data.RuntimeType != skills.SkillRuntimeTypeTool || !body.Data.Enabled {
		t.Fatalf("skill runtime/enabled = %#v, want enabled tool skill", body.Data)
	}
	if body.Data.Display.Icon != "calculator" || body.Data.Display.Label["en_US"] != "Calculator" {
		t.Fatalf("display = %#v, want calculator display metadata", body.Data.Display)
	}
}

func TestHandler_GetSkillUnknownReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		skillDetailErr: aichatservice.ErrNotFound,
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/skills/missing", nil)

	h.GetSkill(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s, want 404", w.Code, w.Body.String())
	}
}

func TestHandler_GetSkillConfigReturnsEnabledSkillIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		skillConfig: &aichatservice.SkillConfig{EnabledSkillIDs: []string{"calculator", "time"}},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/skills/config", nil)

	h.GetSkillConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Data aichatdto.SkillConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	if !sameStrings(body.Data.EnabledSkillIDs, []string{"calculator", "time"}) {
		t.Fatalf("enabled_skill_ids = %v, want calculator/time", body.Data.EnabledSkillIDs)
	}
}

func TestHandler_UpdateSkillConfigReturnsUpdatedConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{
		updateSkillConfig: &aichatservice.SkillConfig{EnabledSkillIDs: []string{"calculator"}},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Request = httptest.NewRequest(http.MethodPut, "/console/api/aichat/skills/config", strings.NewReader(`{"enabled_skill_ids":["calculator"]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateSkillConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Data aichatdto.SkillConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	if !sameStrings(body.Data.EnabledSkillIDs, []string{"calculator"}) {
		t.Fatalf("enabled_skill_ids = %v, want calculator", body.Data.EnabledSkillIDs)
	}
}

func TestHandler_UpdateSkillConfigUnknownSkillReturnsInvalidParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{updateSkillErr: aichatservice.ErrInvalidInput}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Request = httptest.NewRequest(http.MethodPut, "/console/api/aichat/skills/config", strings.NewReader(`{"enabled_skill_ids":["missing"]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateSkillConfig(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", w.Code, w.Body.String())
	}
}

func TestHandler_ListMessagesReturnsEffectiveLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{}
	h := aichathandler.NewHandler(fakeSvc)
	conversationID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Params = gin.Params{{Key: "id", Value: conversationID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/conversations/"+conversationID.String()+"/messages?limit=999", nil)

	h.ListMessages(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	if fakeSvc.messageLimit != 200 {
		t.Fatalf("service limit = %d, want 200", fakeSvc.messageLimit)
	}
	body := decodeListResponse(t, w)
	if body.Data.Limit != 200 {
		t.Fatalf("response limit = %d, want 200", body.Data.Limit)
	}
	if !body.Data.HasMore {
		t.Fatalf("has_more = false, want true")
	}
}

func TestHandler_ListMessagesUsesDefaultLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{}
	h := aichathandler.NewHandler(fakeSvc)
	conversationID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Params = gin.Params{{Key: "id", Value: conversationID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/conversations/"+conversationID.String()+"/messages", nil)

	h.ListMessages(c)

	if fakeSvc.messageLimit != 50 {
		t.Fatalf("service limit = %d, want 50", fakeSvc.messageLimit)
	}
	body := decodeListResponse(t, w)
	if body.Data.Limit != 50 {
		t.Fatalf("response limit = %d, want 50", body.Data.Limit)
	}
}

func TestHandler_ListConversationsReturnsEffectiveLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeAIChatService{}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/conversations?limit=999", nil)

	h.ListConversations(c)

	if fakeSvc.conversationLimit != 100 {
		t.Fatalf("service limit = %d, want 100", fakeSvc.conversationLimit)
	}
	body := decodeListResponse(t, w)
	if body.Data.Limit != 100 {
		t.Fatalf("response limit = %d, want 100", body.Data.Limit)
	}
	if !body.Data.HasMore {
		t.Fatalf("has_more = false, want true")
	}
}

func TestHandler_ListConversationsReturnsMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	conversationID := uuid.New()
	fakeSvc := &fakeAIChatService{
		conversations: []*aichatmodel.Conversation{
			{
				ID:             conversationID,
				OrganizationID: uuid.New(),
				AccountID:      uuid.New(),
				Title:          "Skill conversation",
				Status:         aichatmodel.ConversationStatusNormal,
				RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
				Source:         aichatmodel.ConversationSourceConsole,
				Metadata: map[string]interface{}{
					"skill_config": map[string]interface{}{
						"enabled_skill_ids": []interface{}{"time"},
						"skill_mode":        "auto",
					},
				},
			},
		},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/conversations", nil)

	h.ListConversations(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Data []aichatdto.ConversationResponse `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	if len(body.Data.Data) != 1 {
		t.Fatalf("conversation count = %d, want 1", len(body.Data.Data))
	}
	config, ok := body.Data.Data[0].Metadata["skill_config"].(map[string]interface{})
	if !ok || config["skill_mode"] != "auto" {
		t.Fatalf("metadata = %#v, want skill_config auto", body.Data.Data[0].Metadata)
	}
}

func TestHandler_GetConversationReturnsMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	conversationID := uuid.New()
	fakeSvc := &fakeAIChatService{
		getConversation: &aichatmodel.Conversation{
			ID:             conversationID,
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
			Title:          "Skill conversation",
			Status:         aichatmodel.ConversationStatusNormal,
			RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
			Source:         aichatmodel.ConversationSourceConsole,
			Metadata: map[string]interface{}{
				"skill_config": map[string]interface{}{
					"enabled_skill_ids": []interface{}{"calculator"},
					"skill_mode":        "auto",
				},
			},
		},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Params = gin.Params{{Key: "id", Value: conversationID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/conversations/"+conversationID.String(), nil)

	h.GetConversation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Data aichatdto.ConversationResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	config, ok := body.Data.Metadata["skill_config"].(map[string]interface{})
	if !ok || config["skill_mode"] != "auto" {
		t.Fatalf("metadata = %#v, want skill_config auto", body.Data.Metadata)
	}
}

func TestHandler_StreamConversationEventsWritesSSEID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	conversationID := uuid.New()
	messageID := uuid.New()
	fakeSvc := &fakeAIChatService{
		streamEvents: []aichatservice.StreamEvent{
			{
				ID:        "1730000000000-0",
				EventType: "message",
				Payload: map[string]interface{}{
					"conversation_id": conversationID.String(),
					"message_id":      messageID.String(),
					"answer":          "hello",
				},
			},
		},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Params = gin.Params{{Key: "id", Value: conversationID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/aichat/conversations/"+conversationID.String()+"/events?message_id="+messageID.String(), nil)

	h.StreamConversationEvents(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "id: 1730000000000-0\n") {
		t.Fatalf("body = %q, want sse id", body)
	}
	if !strings.Contains(body, `"event":"message"`) {
		t.Fatalf("body = %q, want message event envelope", body)
	}
}

func TestHandler_StopConversationReturnsStoppedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	conversationID := uuid.New()
	messageID := uuid.New()
	fakeSvc := &fakeAIChatService{
		stopResult: &aichatservice.StopConversationResult{
			Conversation: &aichatmodel.Conversation{
				ID:            conversationID,
				RuntimeStatus: aichatmodel.ConversationRuntimeStatusIdle,
			},
			Message: &aichatmodel.Message{
				ID:     messageID,
				Status: aichatmodel.MessageStatusStopped,
			},
		},
	}
	h := aichathandler.NewHandler(fakeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAIChatScope(c)
	c.Params = gin.Params{{Key: "id", Value: conversationID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/console/api/aichat/conversations/"+conversationID.String()+"/stop", nil)

	h.StopConversation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Data aichatdto.StopConversationResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	if body.Data.ConversationID != conversationID.String() {
		t.Fatalf("conversation_id = %q, want %s", body.Data.ConversationID, conversationID)
	}
	if body.Data.MessageID == nil || *body.Data.MessageID != messageID.String() {
		t.Fatalf("message_id = %v, want %s", body.Data.MessageID, messageID)
	}
	if body.Data.Status != aichatmodel.MessageStatusStopped {
		t.Fatalf("status = %q, want stopped", body.Data.Status)
	}
	if body.Data.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle {
		t.Fatalf("runtime_status = %q, want idle", body.Data.RuntimeStatus)
	}
}

func TestModule_NewModuleIgnoresStartupCleanupFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:aichat_module_cleanup_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	module := aichatmodule.NewModule(db, &fakeLLMClient{}, nil)
	if module == nil || module.Handler == nil || module.Service == nil {
		t.Fatalf("module = %#v, want initialized module", module)
	}
}

func setAIChatScope(c *gin.Context) {
	c.Set("account_id", uuid.NewString())
	c.Set("organization_id", uuid.NewString())
}

type fakeOrganizationAdminAccountService struct {
	interfaces.AccountService
	isAdmin bool
}

func (f fakeOrganizationAdminAccountService) IsOrganizationAdminOrOwner(context.Context, string, string) (bool, error) {
	return f.isAdmin, nil
}

func newAIChatPermissionRouter(service aichatservice.Service, isAdmin bool) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setAIChatScope(c)
		c.Next()
	})
	router.Use(middleware.SetAccountService(fakeOrganizationAdminAccountService{isAdmin: isAdmin}))
	aichathandler.NewHandler(service).RegisterRoutes(router.Group("/console/api"))
	return router
}

func decodeListResponse(t *testing.T, w *httptest.ResponseRecorder) struct {
	Data struct {
		Limit   int  `json:"limit"`
		HasMore bool `json:"has_more"`
	} `json:"data"`
} {
	t.Helper()
	var body struct {
		Data struct {
			Limit   int  `json:"limit"`
			HasMore bool `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v body=%s", err, w.Body.String())
	}
	return body
}
