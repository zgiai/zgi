package v1

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
	"io"
	"net/http"
)

type socialOperationsManagement interface {
	GetConfiguration(context.Context, digitalhuman.Scope, uuid.UUID) (socialops.Configuration, socialops.Record, error)
	SaveConfiguration(context.Context, digitalhuman.Scope, uuid.UUID, int64, socialops.Configuration) (socialops.Configuration, socialops.Record, error)
	Preflight(context.Context, digitalhuman.Scope, uuid.UUID) (socialops.PreflightReport, error)
	Setup(context.Context, digitalhuman.Scope, uuid.UUID, int64, uuid.UUID) (socialops.ProvisioningResult, error)
}

// Deliberately omits identity, employee version and revision authority fields.
// The management service derives them from the authenticated owner/publication.
type socialOperationsSettings struct {
	Product                socialops.ProductConfiguration `json:"product"`
	Audience               string                         `json:"audience"`
	PublicationLanguage    string                         `json:"publication_language"`
	InternalReviewLanguage string                         `json:"internal_review_language"`
	Sources                []socialops.SourceReference    `json:"sources"`
	Research               socialops.ResearchBinding      `json:"research"`
	BrandArtifactRefs      []uuid.UUID                    `json:"brand_artifact_refs,omitempty"`
	Platforms              []socialops.PlatformBinding    `json:"platforms"`
	Timezone               string                         `json:"timezone"`
	Cadence                socialops.Cadence              `json:"cadence"`
	TeamHandoff            socialops.TeamHandoff          `json:"team_handoff"`
}

func (r socialOperationsSettings) configuration() socialops.Configuration {
	return socialops.Configuration{Product: r.Product, Audience: r.Audience, PublicationLanguage: r.PublicationLanguage, InternalReviewLanguage: r.InternalReviewLanguage, Sources: r.Sources, Research: r.Research, BrandArtifactRefs: r.BrandArtifactRefs, Platforms: r.Platforms, Timezone: r.Timezone, Cadence: r.Cadence, TeamHandoff: r.TeamHandoff}
}

type socialOperationsHandler struct{ service socialOperationsManagement }

func registerSocialOperationsRoutes(group *gin.RouterGroup, service socialOperationsManagement) {
	if group == nil || service == nil {
		return
	}
	handler := &socialOperationsHandler{service: service}
	route := group.Group("/digital-humans/:id/social-operations")
	route.GET("/configuration", handler.get)
	route.PUT("/configuration", handler.save)
	route.POST("/preflight", handler.preflight)
	route.POST("/setup", handler.setup)
	route.POST("/assets/images", handler.uploadImage)
	route.GET("/accounts", handler.accounts)
}
func socialOperationsRouteScope(c *gin.Context) (digitalhuman.Scope, uuid.UUID, bool) {
	org, e1 := uuid.Parse(util.GetOrganizationID(c))
	workspace, e2 := uuid.Parse(util.GetWorkspaceID(c))
	owner, e3 := uuid.Parse(middleware.GetAccountID(c))
	employee, e4 := uuid.Parse(c.Param("id"))
	if e1 != nil || e2 != nil || e3 != nil || org == uuid.Nil || workspace == uuid.Nil || owner == uuid.Nil {
		connectorAuthError(c)
		return digitalhuman.Scope{}, uuid.Nil, false
	}
	if e4 != nil || employee == uuid.Nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return digitalhuman.Scope{}, uuid.Nil, false
	}
	return digitalhuman.Scope{OrganizationID: org, WorkspaceID: workspace, ActorID: owner}, employee, true
}
func socialOperationsRouteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, socialops.ErrProvisioningUnauthorized), errors.Is(err, digitalhuman.ErrNotFound):
		err = core.NewError(core.ErrorCodeAccessDenied, false, nil)
	case errors.Is(err, socialops.ErrProvisioningStale), errors.Is(err, socialops.ErrProvisioningNotReady):
		err = core.NewError(core.ErrorCodeConflict, false, nil)
	case errors.Is(err, gorm.ErrRecordNotFound):
		err = core.NewError(core.ErrorCodeTargetNotReady, false, nil)
	}
	connectorRouteError(c, err)
}
func (h *socialOperationsHandler) get(c *gin.Context) {
	scope, id, ok := socialOperationsRouteScope(c)
	if !ok {
		return
	}
	cfg, record, err := h.service.GetConfiguration(c.Request.Context(), scope, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Success(c, gin.H{"configuration": nil, "record": nil})
		return
	}
	if err != nil {
		socialOperationsRouteError(c, err)
		return
	}
	response.Success(c, gin.H{"configuration": cfg, "record": record})
}
func (h *socialOperationsHandler) save(c *gin.Context) {
	scope, id, ok := socialOperationsRouteScope(c)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                    `json:"expected_revision"`
		Configuration    socialOperationsSettings `json:"configuration"`
	}
	if err := decodeConnectorJSON(c, &request); err != nil || request.ExpectedRevision < 0 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, err))
		return
	}
	cfg, record, err := h.service.SaveConfiguration(c.Request.Context(), scope, id, request.ExpectedRevision, request.Configuration.configuration())
	if err != nil {
		socialOperationsRouteError(c, err)
		return
	}
	response.Success(c, gin.H{"configuration": cfg, "record": record})
}
func (h *socialOperationsHandler) preflight(c *gin.Context) {
	scope, id, ok := socialOperationsRouteScope(c)
	if !ok {
		return
	}
	var request struct{}
	if err := decodeConnectorJSON(c, &request); err != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, err))
		return
	}
	report, err := h.service.Preflight(c.Request.Context(), scope, id)
	if err != nil {
		socialOperationsRouteError(c, err)
		return
	}
	response.Success(c, report)
}
func (h *socialOperationsHandler) setup(c *gin.Context) {
	scope, id, ok := socialOperationsRouteScope(c)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64     `json:"expected_configuration_revision"`
		ExpectedVersion  uuid.UUID `json:"expected_employee_version_id"`
	}
	if err := decodeConnectorJSON(c, &request); err != nil || request.ExpectedRevision < 1 || request.ExpectedVersion == uuid.Nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, err))
		return
	}
	result, err := h.service.Setup(c.Request.Context(), scope, id, request.ExpectedRevision, request.ExpectedVersion)
	if err != nil {
		socialOperationsRouteError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *socialOperationsHandler) uploadImage(c *gin.Context) {
	scope, id, ok := socialOperationsRouteScope(c)
	if !ok {
		return
	}
	upload, ok := h.service.(interface {
		UploadBrandImage(context.Context, digitalhuman.Scope, uuid.UUID, []byte) (socialops.Artifact, error)
	})
	if !ok {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (8<<20)+(64<<10))
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	defer file.Close()
	if header.Size < 1 || header.Size > 8<<20 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	artifact, err := upload.UploadBrandImage(c.Request.Context(), scope, id, data)
	if err != nil {
		socialOperationsRouteError(c, err)
		return
	}
	response.Success(c, gin.H{"artifact": artifact})
}

func (h *socialOperationsHandler) accounts(c *gin.Context) {
	scope, id, ok := socialOperationsRouteScope(c)
	if !ok {
		return
	}
	reader, ok := h.service.(interface {
		ListAccounts(context.Context, digitalhuman.Scope, uuid.UUID) ([]socialops.AccountConnection, error)
	})
	if !ok {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	items, err := reader.ListAccounts(c.Request.Context(), scope, id)
	if err != nil {
		socialOperationsRouteError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}
