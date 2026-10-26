package digitalhuman

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	owners    *ArtifactOwnerNames
}

// WithOwnerNames names artifact owners in the console's lists and details.
func (h *ArtifactHandler) WithOwnerNames(owners *ArtifactOwnerNames) *ArtifactHandler {
	h.owners = owners
	return h
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
	group.GET("/owners", h.listOwners)
	group.GET("/:artifact_id", h.get)
	group.GET("/:artifact_id/versions", h.versions)
	group.GET("/:artifact_id/versions/:number/content", h.content)
	group.POST("/:artifact_id/archive", h.archive(true))
	group.POST("/:artifact_id/restore", h.archive(false))
	group.POST("/:artifact_id/sharing", h.sharing)
	group.GET("/:artifact_id/shares", h.shares)
	group.POST("/:artifact_id/shares", h.createShare)
	group.POST("/:artifact_id/shares/:share_id/revoke", h.revokeShare)
}

// RegisterShareRoutes serves shared artifacts to anyone holding a link; the
// group carries no sign-in.
func (h *ArtifactHandler) RegisterShareRoutes(router *gin.RouterGroup) {
	router.GET("/:token", h.openShare)
	router.GET("/:token/content", h.sharedContent)
}

// sharing stops or allows links out of the application to an artifact.
func (h *ArtifactHandler) sharing(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	id, ok := h.artifactID(c)
	if !ok {
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		h.writeError(c, artifacts.ErrInvalidRequest, "digital_human.artifact.sharing")
		return
	}
	if _, err := h.artifacts.SetSharing(c.Request.Context(), reader, id, *request.Enabled); err != nil {
		h.writeError(c, err, "digital_human.artifact.sharing")
		return
	}
	result, err := h.artifacts.Get(c.Request.Context(), reader, id)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.sharing")
		return
	}
	h.owners.Fill(c.Request.Context(), result)
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *ArtifactHandler) shares(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	id, ok := h.artifactID(c)
	if !ok {
		return
	}
	result, err := h.artifacts.Shares(c.Request.Context(), reader, id)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.shares")
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"items": result})
}

func (h *ArtifactHandler) createShare(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	id, ok := h.artifactID(c)
	if !ok {
		return
	}
	var request struct {
		Version   int    `json:"version"`
		ExpiresIn string `json:"expires_in"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			h.writeError(c, artifacts.ErrInvalidRequest, "digital_human.artifact.share")
			return
		}
	}
	result, err := h.artifacts.CreateShare(c.Request.Context(), reader, id, request.Version, request.ExpiresIn,
		artifacts.Actor{Kind: string(reader.Principal.Kind), ID: reader.Principal.ID.String()})
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.share")
		return
	}
	writeDigitalHumanData(c, http.StatusCreated, result)
}

func (h *ArtifactHandler) revokeShare(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	id, ok := h.artifactID(c)
	if !ok {
		return
	}
	shareID, err := uuid.Parse(strings.TrimSpace(c.Param("share_id")))
	if err != nil || shareID == uuid.Nil {
		h.writeError(c, artifacts.ErrInvalidRequest, "digital_human.artifact.share")
		return
	}
	if err := h.artifacts.RevokeShare(c.Request.Context(), reader, id, shareID); err != nil {
		h.writeError(c, err, "digital_human.artifact.share")
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"revoked": true})
}

// sharedArtifact is what a share link shows: no owner, workspace, or
// application identity.
type sharedArtifact struct {
	Title     string     `json:"title"`
	Kind      string     `json:"kind"`
	Version   int        `json:"version"`
	FileName  string     `json:"file_name,omitempty"`
	MimeType  string     `json:"mime_type"`
	Size      int64      `json:"size"`
	Content   *string    `json:"content,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (h *ArtifactHandler) openShare(c *gin.Context) {
	shared, err := h.artifacts.OpenShare(c.Request.Context(), c.Param("token"), true)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.shared")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Referrer-Policy", "no-referrer")
	writeDigitalHumanData(c, http.StatusOK, sharedArtifact{
		Title: shared.Artifact.Title, Kind: string(shared.Artifact.Kind), Version: shared.Version.Number,
		FileName: shared.Version.FileName, MimeType: shared.Version.MimeType, Size: shared.Version.Size,
		Content: shared.Version.Content, UpdatedAt: shared.Version.CreatedAt, ExpiresAt: shared.Share.ExpiresAt,
	})
}

// sharedContent streams a shared version. PDFs and raster images may show
// in the page; anything else, and any download, is an attachment, so the
// API origin never renders a shared file as a page.
func (h *ArtifactHandler) sharedContent(c *gin.Context) {
	shared, err := h.artifacts.OpenShare(c.Request.Context(), c.Param("token"), false)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.shared")
		return
	}
	data, err := h.artifacts.SharedContent(c.Request.Context(), shared)
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.shared")
		return
	}
	name := shared.Version.FileName
	if name == "" {
		name = shared.Artifact.Title + ".md"
	}
	mimeType := strings.ToLower(strings.TrimSpace(shared.Version.MimeType))
	disposition := "attachment"
	if c.Query("download") == "" && (mimeType == "application/pdf" || sharedInlineImage(mimeType)) {
		disposition = "inline"
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Referrer-Policy", "no-referrer")
	if mimeType != "application/pdf" {
		// A browser does not show a PDF in a sandboxed document.
		c.Header("Content-Security-Policy", "sandbox")
	}
	c.Header("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, url.PathEscape(name)))
	c.Data(http.StatusOK, shared.Version.MimeType, data)
}

func sharedInlineImage(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
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
	query.OwnerKind = artifacts.OwnerKind(strings.TrimSpace(c.Query("owner_kind")))
	if raw := strings.TrimSpace(c.Query("owner_id")); raw != "" {
		ownerID, err := uuid.Parse(raw)
		if err != nil {
			h.writeError(c, artifacts.ErrInvalidRequest, "digital_human.artifact.list")
			return
		}
		query.OwnerID = &ownerID
	}
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
	h.owners.Fill(c.Request.Context(), result.Items...)
	writeDigitalHumanData(c, http.StatusOK, result)
}

// listOwners pages the owners of the artifacts the reader sees, searched by
// name, email or end user identifier, so the console can filter by one.
func (h *ArtifactHandler) listOwners(c *gin.Context) {
	reader, ok := h.reader(c)
	if !ok {
		return
	}
	owners, err := h.artifacts.Owners(c.Request.Context(), reader, artifacts.OwnerKind(strings.TrimSpace(c.Query("kind"))),
		artifacts.ArchiveFilter(strings.TrimSpace(c.Query("archived"))))
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.owners")
		return
	}
	result, err := h.owners.Page(c.Request.Context(), owners, c.Query("query"), intQuery(c, "page"), intQuery(c, "page_size"))
	if err != nil {
		h.writeError(c, err, "digital_human.artifact.owners")
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
	h.owners.Fill(c.Request.Context(), result)
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
		h.owners.Fill(c.Request.Context(), result)
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
	case errors.Is(err, artifacts.ErrInvalidRequest), errors.Is(err, artifacts.ErrSharingDisabled):
		err = ErrInvalidRequest
	}
	h.base.writeError(c, applicationError(err, operation))
}
