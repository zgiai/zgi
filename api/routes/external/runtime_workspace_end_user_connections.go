package external

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// registerRuntimeEndUserConnections lets the integrating server supply an
// end user's own key for the per-user Connector slots of the DigitalHumans
// its Runtime Workspace serves (ADR 0007). Keys are write-only: no response
// returns or echoes one.
func registerRuntimeEndUserConnections(group gin.IRoutes, service *runtimeworkspace.Service, db *gorm.DB) {
	group.GET("/end-users/:user/connections", func(c *gin.Context) {
		principal, subjectID, apps, ok := runtimeEndUserConnectionScope(c, service, db)
		if !ok {
			return
		}
		manager := digitalhuman.RegisteredEndUserConnections()
		if manager == nil {
			response.Success(c, gin.H{"items": []any{}})
			return
		}
		items, err := manager.List(c.Request.Context(), principal.RuntimeWorkspace.OrganizationID, subjectID, apps)
		if err != nil {
			runtimeFail(c, err)
			return
		}
		response.Success(c, gin.H{"items": items})
	})
	group.POST("/end-users/:user/connections", func(c *gin.Context) {
		principal, subjectID, apps, ok := runtimeEndUserConnectionScope(c, service, db)
		if !ok {
			return
		}
		idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
			response.FailWithMessage(c, response.ErrInvalidParam, "Idempotency-Key header is required")
			return
		}
		var request struct {
			AppKey  string            `json:"app_key"`
			SlotKey string            `json:"slot_key"`
			Values  map[string]string `json:"values"`
		}
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
		if err != nil || json.Unmarshal(body, &request) != nil || len(request.Values) == 0 {
			clear(body)
			clear(request.Values)
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		clear(body)
		app, found := runtimeEndUserApp(apps, request.AppKey)
		manager := digitalhuman.RegisteredEndUserConnections()
		if !found || manager == nil {
			clear(request.Values)
			response.Fail(c, response.ErrNotFound)
			return
		}
		slot, err := manager.Connect(c.Request.Context(), principal.RuntimeWorkspace.OrganizationID, subjectID, app, strings.TrimSpace(request.SlotKey), idempotencyKey, request.Values)
		if err != nil {
			runtimeConnectionFail(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, slot)
	})
	group.DELETE("/end-users/:user/connections/:app/:slot_key", func(c *gin.Context) {
		principal, subjectID, apps, ok := runtimeEndUserConnectionScope(c, service, db)
		if !ok {
			return
		}
		app, found := runtimeEndUserApp(apps, c.Param("app"))
		manager := digitalhuman.RegisteredEndUserConnections()
		if !found || manager == nil {
			response.Fail(c, response.ErrNotFound)
			return
		}
		if err := manager.Disconnect(c.Request.Context(), principal.RuntimeWorkspace.OrganizationID, subjectID, app.DigitalHumanID, c.Param("slot_key")); err != nil {
			runtimeConnectionFail(c, err)
			return
		}
		response.Success(c, gin.H{"result": "success"})
	})
}

// runtimeEndUserConnectionScope resolves the end user named by the path,
// creating their identity as a first request would, and the DigitalHumans
// the Runtime Workspace serves.
func runtimeEndUserConnectionScope(c *gin.Context, service *runtimeworkspace.Service, db *gorm.DB) (*runtimeworkspace.Principal, uuid.UUID, []digitalhuman.EndUserApp, bool) {
	principal, ok := runtimePrincipal(c)
	if !ok {
		return nil, uuid.Nil, nil, false
	}
	if !digitalhuman.PerUserConnectorsEnabled() {
		response.Fail(c, response.ErrNotFound)
		return nil, uuid.Nil, nil, false
	}
	scoped := *principal
	if err := service.ResolveRequestPrincipal(c.Request.Context(), &scoped, c.Param("user"), ""); err != nil || scoped.SubjectID == uuid.Nil {
		runtimeFail(c, runtimeworkspace.ErrSubjectRequired)
		return nil, uuid.Nil, nil, false
	}
	var bound []runtimeworkspace.AgentBinding
	if err := db.WithContext(c.Request.Context()).
		Where("runtime_workspace_id = ? AND app_type = ? AND status = ?", principal.RuntimeWorkspace.LogicalID(), runtimeworkspace.AppTypeDigitalHuman, runtimeworkspace.AgentBindingActive).
		Find(&bound).Error; err != nil {
		runtimeFail(c, err)
		return nil, uuid.Nil, nil, false
	}
	apps := make([]digitalhuman.EndUserApp, 0, len(bound))
	for _, binding := range bound {
		apps = append(apps, digitalhuman.EndUserApp{AppKey: binding.PublicAppKey, DigitalHumanID: binding.AgentID})
	}
	return &scoped, scoped.SubjectID, apps, true
}

func runtimeEndUserApp(apps []digitalhuman.EndUserApp, appKey string) (digitalhuman.EndUserApp, bool) {
	for _, app := range apps {
		if app.AppKey == strings.TrimSpace(appKey) {
			return app, true
		}
	}
	return digitalhuman.EndUserApp{}, false
}

// runtimeConnectionFail maps connection setup failures without exposing a
// provider cause or a submitted value. The connector error code tells the
// integrator what to fix.
func runtimeConnectionFail(c *gin.Context, err error) {
	if errors.Is(err, digitalhuman.ErrConnectorSlotNotFound) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	if errors.Is(err, digitalhuman.ErrConnectorSetupFailed) {
		c.JSON(400, response.Response{Code: string(core.ErrorCodeInvalidKey), Message: "The key could not be verified"})
		return
	}
	switch code := core.ErrorCodeOf(err); code {
	case core.ErrorCodeInvalidKey, core.ErrorCodeInsufficientKey, core.ErrorCodeInvalidInput, core.ErrorCodePermissionMissing:
		c.JSON(400, response.Response{Code: string(code), Message: "The key could not be verified"})
	case core.ErrorCodeConflict:
		c.JSON(409, response.Response{Code: string(code), Message: "The connection request conflicts with another one"})
	case core.ErrorCodeProviderUnavailable, core.ErrorCodeRateLimited:
		c.JSON(503, response.Response{Code: string(code), Message: "The provider could not be reached; try again later"})
	default:
		response.Fail(c, response.ErrSystemError)
	}
}
