package zgimarket

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	callLogsPath = "call-logs"
	// maxCallLogRange is the longest period one read of the call logs covers.
	maxCallLogRange = 31 * 24 * time.Hour
	maxCallLogPage  = 100
	// callChargeSettleDelay is how long after a call its charge is first asked for: the market
	// rates usage a moment after the answer.
	callChargeSettleDelay = 3 * time.Second
)

// callLogRoute answers GET call-logs, the organization's market calls, for administrators. It
// reports whether the request was that one.
//
//	GET call-logs?from=&to=&status=&subject_type=&subject_id=&listing=&page=&per_page=
//
// from and to are Unix seconds, [from, to), at most 31 days apart. Before the page is answered,
// the calls on it whose charge is still pending are asked for at the market once.
func (p *Proxy) callLogRoute(c *gin.Context) bool {
	if strings.Trim(c.Param("path"), "/") != callLogsPath {
		return false
	}
	if c.Request.Method != http.MethodGet {
		response.Fail(c, response.ErrNotFound)
		return true
	}
	if p.tools == nil {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market tools are not configured")
		return true
	}
	who, ok := p.member(c)
	if !ok {
		return true
	}
	if !who.admin {
		response.Fail(c, response.ErrPermissionDenied)
		return true
	}
	c.Header("Cache-Control", "private, no-store")
	query, ok := callLogQuery(c.Request.URL.Query())
	if !ok {
		response.Fail(c, response.ErrInvalidParams)
		return true
	}
	organizationID, err := uuid.Parse(who.organizationID)
	if err != nil {
		response.Fail(c, response.ErrPermissionDenied)
		return true
	}
	page, err := p.tools.CallLogs(c.Request.Context(), who, organizationID, query)
	if err != nil {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "The market call logs could not be read")
		return true
	}
	response.Success(c, page)
	return true
}

func callLogQuery(values url.Values) (CallLogQuery, bool) {
	from, errFrom := strconv.ParseInt(values.Get("from"), 10, 64)
	to, errTo := strconv.ParseInt(values.Get("to"), 10, 64)
	if errFrom != nil || errTo != nil || to <= from || time.Duration(to-from)*time.Second > maxCallLogRange {
		return CallLogQuery{}, false
	}
	query := CallLogQuery{From: time.Unix(from, 0).UTC(), To: time.Unix(to, 0).UTC(), Page: 1, PerPage: 20}
	status, ok := trimmedStatus(values.Get("status"))
	if !ok {
		return CallLogQuery{}, false
	}
	query.Status = status
	switch subjectType := values.Get("subject_type"); subjectType {
	case "", SubjectAgent, SubjectDigitalHuman:
		query.SubjectType = subjectType
	default:
		return CallLogQuery{}, false
	}
	if raw := values.Get("subject_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return CallLogQuery{}, false
		}
		query.SubjectID = &id
	}
	if listing := values.Get("listing"); listing != "" {
		if !marketSlugPattern.MatchString(listing) {
			return CallLogQuery{}, false
		}
		query.Listing = listing
	}
	if raw := values.Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 || page > 10000 {
			return CallLogQuery{}, false
		}
		query.Page = page
	}
	if raw := values.Get("per_page"); raw != "" {
		perPage, err := strconv.Atoi(raw)
		if err != nil || perPage < 1 || perPage > maxCallLogPage {
			return CallLogQuery{}, false
		}
		query.PerPage = perPage
	}
	return query, true
}

// CallLogs returns one page of the organization's market calls. The calls on it whose charge is
// pending are asked for at the market first; a market that cannot answer leaves them pending.
func (t *Tools) CallLogs(ctx context.Context, who caller, organizationID uuid.UUID, query CallLogQuery) (*CallLogPage, error) {
	page, err := t.logs.list(ctx, organizationID, query)
	if err != nil {
		return nil, err
	}
	if t.settleCharges(ctx, who, page.Data) {
		// Charges were settled: the sums are read again so they agree with the rows.
		if page, err = t.logs.list(ctx, organizationID, query); err != nil {
			return nil, err
		}
	}
	return page, nil
}

// marketCallCharge is what the market answers for one call.
type marketCallCharge struct {
	RequestID    string `json:"request_id"`
	State        string `json:"state"`
	AmountMicros int64  `json:"amount_micros"`
	Lines        []struct {
		Meter        string `json:"meter"`
		UnitName     string `json:"unit_name"`
		Quantity     int64  `json:"quantity"`
		AmountMicros int64  `json:"amount_micros"`
	} `json:"lines"`
}

// settleCharges asks the market what the pending calls among rows were charged and keeps the
// answers. It reports whether any row changed.
func (t *Tools) settleCharges(ctx context.Context, who caller, rows []*CallLog) bool {
	now := t.now()
	pending := map[string]*CallLog{}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ChargeState == CallChargePending && row.RequestID != "" && now.Sub(row.CreatedAt) >= callChargeSettleDelay {
			if _, seen := pending[row.RequestID]; !seen {
				ids = append(ids, row.RequestID)
			}
			pending[row.RequestID] = row
		}
	}
	if len(ids) == 0 {
		return false
	}
	var answer struct {
		Charges []marketCallCharge `json:"charges"`
	}
	path := "calls/charges?request_ids=" + url.QueryEscape(strings.Join(ids, ","))
	if err := t.marketData(ctx, who, http.MethodGet, path, nil, &answer); err != nil {
		logger.WarnContext(ctx, "the charges of ZGI Market calls could not be read; they stay pending",
			"organization_id", who.organizationID, "calls", len(ids), "error", err.Error())
		return false
	}
	changed := false
	for _, charge := range answer.Charges {
		row := pending[charge.RequestID]
		if row == nil || charge.State == CallChargePending {
			continue
		}
		lines := make([]CallChargeLine, 0, len(charge.Lines))
		for _, line := range charge.Lines {
			lines = append(lines, CallChargeLine{
				Meter: line.Meter, UnitName: line.UnitName, Quantity: line.Quantity, AmountMicros: line.AmountMicros,
			})
		}
		state := CallChargeNone
		if charge.State == CallChargeCharged {
			state = CallChargeCharged
		}
		for _, same := range rows {
			if same.RequestID != charge.RequestID || same.ChargeState != CallChargePending {
				continue
			}
			if err := t.logs.settle(ctx, same.ID, state, charge.AmountMicros, lines, now); err != nil {
				logger.WarnContext(ctx, "the charge of a ZGI Market call could not be kept",
					"request_id", charge.RequestID, "error", err.Error())
				continue
			}
			changed = true
		}
	}
	return changed
}
