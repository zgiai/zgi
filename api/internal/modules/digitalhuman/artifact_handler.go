package digitalhuman

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// ArtifactHandler serves a DigitalHuman's artifacts to console members
// (ADR 0009). Members read their own and the team's; task managers also read
// members' and end users' private artifacts.
type ArtifactHandler struct {
	artifacts *artifacts.Service
	base      *Handler
}

// NewArtifactHandler builds the console artifact handler.
func NewArtifactHandler(service *artifacts.Service, projector *apptransport.Projector) *ArtifactHandler {
	if service == nil || projector == nil {
		panic("digital human artifact handler requires artifacts and projector")
	}
	return &ArtifactHandler{artifacts: service, base: &Handler{projector: projector}}
}

// RegisterRoutes serves the artifact center of each DigitalHuman.
func (h *ArtifactHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/digital-humans/:id/artifacts")
	group.GET("", h.list)
	group.GET("/:artifact_id", h.get)
	group.GET("/:artifact_id/versions", h.versions)
	group.GET("/:artifact_id/versions/:number/content", h.content)
	group.POST("/:artifact_id/archive", h.archive(true))
	group.POST("/:artifact_id/restore", h.archive(false))
}

func (h *ArtifactHandler) reader(c *gin.Context) (artifacts.Reader, bool) {
	scope, id, ok := h.base.scopeAndID(c)
	if !ok {
		return artifacts.Reader{}, false
	}
	return artifacts.Reader{
		Scope:     artifactScope(scope.OrganizationID, scope.WorkspaceID, id),
		Principal: artifacts.Owner{Kind: artifacts.OwnerAccount, ID: scope.ActorID},
		Member:    true,
		Operator:  c.GetBool(TaskManagerContextKey),
	}, true
}

func (h *ArtifactHandler) list(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	query := artifacts.ListQuery{Kind: artifacts.Kind(strings.TrimSpace(c.Query("kind"))), Query: c.Query("q"), Mine: c.Query("scope") == "mine",
		Archive: artifacts.ArchiveFilter(strings.TrimSpace(c.Query("archived"))), Page: intQuery(c, "page"), PageSize: intQuery(c, "page_size")}
	if raw := strings.TrimSpace(c.Query("task_id")); raw != "" {
		taskID, err := uuid.Parse(raw)
		if err != nil {
			h.writeError(c, artifacts.ErrInvalidRequest, "digital_human.artifact.list")
			return
		}
		query.TaskID = &taskID
	}
	result, err := h.artifacts.List(c.Request.Context(), reader, query)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.list")
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *ArtifactHandler) get(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	id, ok := h.artifactID(c)
	if !ok {
		return
	}
	result, err := h.artifacts.Get(c.Request.Context(), reader, id)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.get")
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *ArtifactHandler) versions(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	id, ok := h.artifactID(c)
	if !ok {
		return
	}
	result, err := h.artifacts.Versions(c.Request.Context(), reader, id)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.versions")
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"items": result})
}

// archive archives or restores an artifact the reader manages: their own,
// a workspace's shared one, or any for a task manager.
func (h *ArtifactHandler) archive(archived bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		reader, ok := h.reader(c)
		if !ok {
			return
		}
		id, ok := h.artifactID(c)
		if !ok {
			return
		}
		if _, err := h.artifacts.SetArchived(c.Request.Context(), reader, id, archived); err != nil {
			h.writeError(c, err, "digital_human.artifact.archive")
			return
		}
		result, err := h.artifacts.Get(c.Request.Context(), reader, id)
		if err != nil {
			h.writeError(c, err, "digital_human.artifact.archive")
			return
		}
		writeDigitalHumanData(c, http.StatusOK, result)
	}
}

// content streams a version: a document as Markdown, a file as itself. It is
// always an attachment, so the console origin never renders it as a page.
func (h *ArtifactHandler) content(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	id, ok := h.artifactID(c)
	if !ok {
		return
	}
	number, err := strconv.Atoi(c.Param("number"))
	if err != nil || number < 0 {
		h.writeError(c, artifacts.ErrInvalidRequest, "digital_human.artifact.content")
		return
	}
	artifact, version, data, err := h.artifacts.Content(c.Request.Context(), reader, id, number)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.content")
		return
	}
	name := version.FileName
	if name == "" {
		name = artifact.Title + ".md"
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Security-Policy", "sandbox")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(name)))
	c.Data(http.StatusOK, version.MimeType, data)
}

func (h *ArtifactHandler) artifactID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("artifact_id")))
	if err != nil || id == uuid.Nil {
		h.writeError(c, artifacts.ErrInvalidRequest, "digital_human.artifact.request")
		return uuid.Nil, false
	}
	return id, true
}

func (h *ArtifactHandler) writeError(c *gin.Context, err error, operation string) {
	switch {
	case errors.Is(err, artifacts.ErrNotFound):
		err = ErrNotFound
	case errors.Is(err, artifacts.ErrInvalidRequest):
		err = ErrInvalidRequest
	}
	h.base.writeError(c, applicationError(err, operation))
}
