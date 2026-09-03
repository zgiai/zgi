package wecom

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

const (
	memberSearchQueryTypeUser   = 1
	memberSearchRangeActiveOnly = 0
	memberSearchExactNameField  = 1
)

type memberSearchUserIDs struct {
	UserIDs     []string `json:"userid"`
	OpenUserIDs []string `json:"open_userid"`
}

type memberSearchResponse struct {
	apiEnvelope
	QueryResult struct {
		Users memberSearchUserIDs `json:"user"`
	} `json:"query_result"`
	IsLast     bool   `json:"is_last"`
	NextCursor string `json:"next_cursor"`
}

func (adapter *Adapter) searchMembers(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	query := strings.TrimSpace(inputString(input, "query"))
	if query == "" || !utf8.ValidString(query) || len([]rune(query)) > 128 {
		return nil, 0, wecomError(integrations.ErrorCodeInvalidInput, "WeCom member search query is invalid", nil)
	}
	if inputInt(input, "department_id", 0) > 0 {
		return nil, 0, integrations.NewErrorWithReason(
			integrations.ErrorCodeInvalidInput,
			"wecom_member_search_department_filter_unsupported",
			"WeCom official member search does not accept a department filter",
			nil,
		)
	}
	limit := inputInt(input, "max_results", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	cursor := strings.TrimSpace(inputString(input, "cursor"))
	if len(cursor) > 512 || strings.ContainsAny(cursor, "\r\n") {
		return nil, 0, wecomError(integrations.ErrorCodeInvalidInput, "WeCom member search cursor is invalid", nil)
	}
	matchMode := strings.ToLower(strings.TrimSpace(inputString(input, "match_mode")))
	if matchMode == "" {
		matchMode = "fuzzy"
	}
	if matchMode != "fuzzy" && matchMode != "exact" {
		return nil, 0, wecomError(integrations.ErrorCodeInvalidInput, "WeCom member search match mode is invalid", nil)
	}
	agentID, err := strconv.ParseInt(creds.AgentID, 10, 64)
	if err != nil {
		return nil, 0, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom Agent ID is invalid", err)
	}
	payload := map[string]interface{}{
		"auth_corpid": creds.AuthCorpID,
		"query_word":  query,
		"query_type":  memberSearchQueryTypeUser,
		"query_range": memberSearchRangeActiveOnly,
		"agentid":     agentID,
		"limit":       limit,
	}
	if cursor != "" {
		payload["cursor"] = cursor
	}
	if matchMode == "exact" {
		payload["full_match_field"] = memberSearchExactNameField
	}

	response := memberSearchResponse{}
	if err := adapter.providerJSON(ctx, http.MethodPost, "/cgi-bin/service/contact/search", nil, payload, &response); err != nil {
		return nil, 0, err
	}
	userIDs := response.QueryResult.Users.UserIDs
	openUserIDs := response.QueryResult.Users.OpenUserIDs
	if len(userIDs) > limit {
		return nil, 0, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom member search returned too many results", nil)
	}
	if len(openUserIDs) != 0 && len(openUserIDs) != len(userIDs) {
		return nil, 0, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom member search returned mismatched identifier lists", nil)
	}
	items := make([]map[string]interface{}, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for index, rawUserID := range userIDs {
		userID := strings.TrimSpace(rawUserID)
		if !validProviderSubjectID(userID) || len(userID) > 64 {
			return nil, 0, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom member search returned an invalid member identifier", nil)
		}
		key := strings.ToLower(userID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		openUserID := ""
		if len(openUserIDs) != 0 {
			openUserID = strings.TrimSpace(openUserIDs[index])
			if openUserID != "" && !validProviderSubjectID(openUserID) {
				return nil, 0, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom member search returned an invalid open member identifier", nil)
			}
		}
		alternateKind := ""
		if openUserID != "" {
			alternateKind = SubjectIDKindOpenUserID
		}
		details, err := adapter.readUser(ctx, creds, userID)
		if err != nil {
			return nil, 0, err
		}
		departments := details.Department
		if departments == nil {
			departments = []int{}
		}
		reference, err := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, userID, alternateKind, openUserID)
		if err != nil {
			return nil, 0, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom member reference could not be created", err)
		}
		items = append(items, map[string]interface{}{
			"recipient_ref":  reference,
			"name":           providerDisplayName(details.Name, details.UserID),
			"department_ids": departments,
		})
	}
	nextCursor := bounded(response.NextCursor, 512)
	if response.IsLast {
		nextCursor = ""
	}
	return map[string]interface{}{
		"provider":           IntegrationID,
		"members":            items,
		"is_last":            response.IsLast,
		"next_cursor":        nextCursor,
		"requires_selection": len(items) > 1,
	}, len(items), nil
}

func (adapter *Adapter) providerJSON(ctx context.Context, method, path string, query url.Values, body interface{}, out interface{}) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := adapter.providerAccessToken(ctx)
		if err != nil {
			return err
		}
		values := url.Values{}
		for key, list := range query {
			for _, value := range list {
				values.Add(key, value)
			}
		}
		values.Set("provider_access_token", token)
		status, envelope, err := adapter.doJSON(ctx, method, path, values, body, out)
		if err != nil {
			return err
		}
		if envelope.ErrCode == 0 {
			return nil
		}
		if (envelope.ErrCode == 40014 || envelope.ErrCode == 42001) && attempt == 0 {
			adapter.evictProviderAccessToken()
			continue
		}
		return mapProviderError(status, envelope)
	}
	return wecomError(integrations.ErrorCodeAuthInvalid, "WeCom provider access token is invalid", nil)
}
