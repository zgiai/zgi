package zgimarket

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	activationRequestsPath = "activation-requests"
	maxActivationBody      = 8 << 10
)

// activationRequestRoute answers the members' requests to activate a market item. It reports
// whether the request was one of them.
//
//	GET  activation-requests?status=&listing_id=&mine=1  an administrator reads everyone's, a member their own
//	POST activation-requests                             a member asks for an item
//	POST activation-requests/:id/approve                 an administrator, once the item is activated
//	POST activation-requests/:id/reject                  an administrator, with a note
//	POST activation-requests/:id/cancel                  the requester, while it is pending
func (p *Proxy) activationRequestRoute(c *gin.Context) bool {
	path := strings.Trim(c.Param("path"), "/")
	if path != activationRequestsPath && !strings.HasPrefix(path, activationRequestsPath+"/") {
		return false
	}
	if p.tools == nil {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market tools are not configured")
		return true
	}
	who, ok := p.member(c)
	if !ok {
		return true
	}
	organizationID, errOrganization := uuid.Parse(who.organizationID)
	accountID, errAccount := uuid.Parse(who.accountID)
	if errOrganization != nil || errAccount != nil {
		response.Fail(c, response.ErrPermissionDenied)
		return true
	}
	c.Header("Cache-Control", "private, no-store")
	rest := strings.TrimPrefix(strings.TrimPrefix(path, activationRequestsPath), "/")
	switch {
	case rest == "" && c.Request.Method == http.MethodGet:
		p.listActivationRequests(c, who, organizationID, accountID)
	case rest == "" && c.Request.Method == http.MethodPost:
		p.createActivationRequest(c, organizationID, accountID)
	case c.Request.Method == http.MethodPost:
		id, action, found := strings.Cut(rest, "/")
		requestID, err := uuid.Parse(id)
		if !found || err != nil {
			response.Fail(c, response.ErrNotFound)
			return true
		}
		p.settleActivationRequest(c, who, organizationID, accountID, requestID, action)
	default:
		response.Fail(c, response.ErrNotFound)
	}
	return true
}

func (p *Proxy) listActivationRequests(c *gin.Context, who caller, organizationID, accountID uuid.UUID) {
	values := c.Request.URL.Query()
	query := ActivationRequestQuery{Status: values.Get("status")}
	switch query.Status {
	case "", ActivationPending, ActivationApproved, ActivationRejected, ActivationCancelled:
	default:
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if raw := values.Get("listing_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		query.ListingID = id
	}
	// A member reads only their own requests; an administrator reads everyone's unless asking
	// for their own.
	if !who.admin || values.Get("mine") == "1" {
		query.RequesterID = accountID
	}
	page, err := p.tools.requests.list(c.Request.Context(), organizationID, query)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "market activation requests could not be read", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, page)
}

func (p *Proxy) createActivationRequest(c *gin.Context, organizationID, accountID uuid.UUID) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxActivationBody+1))
	var input ActivationRequestInput
	if err != nil || len(body) > maxActivationBody || json.Unmarshal(body, &input) != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	row, err := p.tools.requests.create(c.Request.Context(), organizationID, accountID, input)
	if errors.Is(err, errActivationInput) {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "a market activation request could not be saved", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, row)
}

func (p *Proxy) settleActivationRequest(
	c *gin.Context, who caller, organizationID, accountID, requestID uuid.UUID, action string,
) {
	var status string
	var reviewer *uuid.UUID
	switch action {
	case "approve", "reject":
		if !who.admin {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		status, reviewer = ActivationApproved, &accountID
		if action == "reject" {
			status = ActivationRejected
		}
	case "cancel":
		status = ActivationCancelled
	default:
		response.Fail(c, response.ErrNotFound)
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxActivationBody+1))
	if err != nil || len(body) > maxActivationBody || (len(body) > 0 && json.Unmarshal(body, &input) != nil) {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	// Only the requester takes their request back.
	var requester *uuid.UUID
	if status == ActivationCancelled {
		requester, input.Note = &accountID, ""
	}
	row, err := p.tools.requests.settle(c.Request.Context(), organizationID, requestID, status, reviewer, requester, input.Note)
	switch {
	case errors.Is(err, errActivationNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, errActivationSettled):
		response.FailWithMessage(c, response.ErrInvalidParams, "The request is no longer pending")
	case err != nil:
		logger.ErrorContext(c.Request.Context(), "a market activation request could not be settled", err)
		response.Fail(c, response.ErrSystemError)
	default:
		response.Success(c, row)
	}
}
