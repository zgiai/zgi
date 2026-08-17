package feishu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type Adapter struct {
	client *client
}

func New(httpClient *http.Client) (*Adapter, error) {
	apiClient, err := newClient(httpClient)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func newForBaseURLs(httpClient *http.Client, cnBaseURL, globalBaseURL string) (*Adapter, error) {
	apiClient, err := newClientForBaseURLs(httpClient, cnBaseURL, globalBaseURL)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	region, err := feishuRegion(request.Connection)
	if err != nil {
		return nil, err
	}
	switch request.ActionID {
	case ActionGetAccount:
		token, tokenErr := feishuUserAccessToken(request.Connection)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.getAccount(ctx, region, token)
		return feishuActionResult(output, meta, 1), actionErr
	case ActionListDriveFiles:
		token, tokenMeta, tokenErr := adapter.connectionAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.listDriveFiles(ctx, region, token, request.Input)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, outputCount(output, "files")), actionErr
	case ActionReadDocument:
		token, tokenMeta, tokenErr := adapter.connectionAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.readDocument(ctx, region, token, request.Input)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, 1), actionErr
	case ActionSearchContacts:
		token, tokenErr := feishuUserAccessToken(request.Connection)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.searchContacts(ctx, region, token, request.Input)
		return feishuActionResult(output, meta, outputCount(output, "users")), actionErr
	case ActionListChats:
		token, tokenMeta, tokenErr := adapter.connectionAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.listChats(ctx, region, token, request.Input)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, outputCount(output, "chats")), actionErr
	case ActionListMessages:
		token, tokenMeta, tokenErr := adapter.connectionAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.listMessages(ctx, region, token, request.Input)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, outputCount(output, "messages")), actionErr
	case ActionListCalendars:
		token, tokenMeta, tokenErr := adapter.connectionAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.listCalendars(ctx, region, token, request.Input)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, outputCount(output, "calendars")), actionErr
	case ActionListEvents:
		token, tokenMeta, tokenErr := adapter.connectionAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.listCalendarEvents(ctx, region, token, request.Input)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, outputCount(output, "events")), actionErr
	case ActionCreateEvent:
		token, tokenMeta, tokenErr := adapter.connectionAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.createCalendarEvent(ctx, region, token, request)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, 1), actionErr
	case ActionSendUserMessage:
		token, tokenErr := feishuUserAccessToken(request.Connection)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.sendMessage(ctx, region, token, request.Connection, request.Input, true)
		return feishuActionResult(output, meta, 1), actionErr
	case ActionSendBotMessage:
		token, tokenMeta, tokenErr := adapter.tenantAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return feishuActionResult(nil, tokenMeta, 0), tokenErr
		}
		output, meta, actionErr := adapter.sendMessage(ctx, region, token, request.Connection, request.Input, false)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, 1), actionErr
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu action is not supported", nil)
	}
}

func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	region, err := feishuRegion(connection)
	if err != nil {
		return nil, err
	}
	if isTenantAppConnection(connection) {
		token, meta, err := adapter.tenantAccessToken(ctx, connection, region)
		if err != nil {
			return nil, err
		}
		var tenant feishuTenantData
		tenantMeta, err := adapter.client.getTenant(ctx, region, token, &tenant)
		if err != nil {
			return nil, err
		}
		return &integrations.ConnectionProfile{
			AccountID:         firstNonEmpty(tenant.Tenant.TenantKey, tenant.Tenant.DisplayID),
			DisplayName:       firstNonEmpty(tenant.Tenant.Name, tenant.Tenant.DisplayID, tenant.Tenant.TenantKey),
			GrantedScopes:     append([]string(nil), connection.GrantedScopes...),
			ProviderRequestID: firstNonEmpty(tenantMeta.RequestID, meta.RequestID),
		}, nil
	}
	token, err := feishuUserAccessToken(connection)
	if err != nil {
		return nil, err
	}
	var user feishuUserData
	meta, err := adapter.client.getUserInfo(ctx, region, token, &user)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(firstNonEmpty(user.OpenID, user.UserID, user.UnionID)) == "" {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu identity response is incomplete", nil)
	}
	return &integrations.ConnectionProfile{
		AccountID:         firstNonEmpty(user.OpenID, user.UserID, user.UnionID),
		DisplayName:       firstNonEmpty(user.Name, user.EnName, user.Email, user.OpenID),
		GrantedScopes:     append([]string(nil), connection.GrantedScopes...),
		ProviderRequestID: meta.RequestID,
	}, nil
}

func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		switch integrations.ErrorCode(err) {
		case integrations.ErrorCodeTimeout, integrations.ErrorCodeUpstream, integrations.ErrorCodeRateLimited:
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{
			Status: status,
			Checks: []integrations.HealthProbeCheck{{
				Code: integrations.ErrorCode(err), Status: status, Message: "Feishu connection check failed",
			}},
		}, err
	}
	return &integrations.HealthProbeReport{
		Status: integrations.HealthProbeStatusHealthy, Profile: profile,
		Checks: []integrations.HealthProbeCheck{{
			Code: "feishu_authenticated_identity", Status: integrations.HealthProbeStatusHealthy,
		}},
	}, nil
}

func (adapter *Adapter) getAccount(ctx context.Context, region, token string) (map[string]interface{}, responseMeta, error) {
	var user feishuUserData
	meta, err := adapter.client.getUserInfo(ctx, region, token, &user)
	if err != nil {
		return nil, meta, err
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"account": map[string]interface{}{
			"open_id": bounded(user.OpenID, 128), "union_id": bounded(user.UnionID, 128),
			"user_id": bounded(user.UserID, 128), "name": bounded(user.Name, 255),
			"email": bounded(user.Email, 320), "tenant_key": bounded(user.TenantKey, 128),
			"avatar_url": safeFeishuURL(user.AvatarURL),
		},
	}, meta, nil
}

func (adapter *Adapter) listDriveFiles(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	pageSize := inputInteger(input, "page_size", 20, 1, 50)
	query := url.Values{"page_size": []string{strconv.Itoa(pageSize)}}
	if folderToken := bounded(inputString(input, "folder_token"), 255); folderToken != "" {
		query.Set("folder_token", folderToken)
	}
	if pageToken := bounded(inputString(input, "page_token"), 1024); pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var data feishuDriveFilesData
	meta, err := adapter.client.listDriveFiles(ctx, region, token, query, &data)
	if err != nil {
		return nil, meta, err
	}
	files := make([]interface{}, 0, min(len(data.Files), pageSize))
	for index, file := range data.Files {
		if index >= pageSize {
			break
		}
		files = append(files, map[string]interface{}{
			"token": bounded(file.Token, 255), "name": bounded(file.Name, 500),
			"type": bounded(file.Type, 64), "parent_token": bounded(file.ParentToken, 255),
			"url": safeFeishuURL(file.URL), "created_time": bounded(file.CreatedTime, 64),
			"modified_time": bounded(file.ModifiedTime, 64), "owner_id": bounded(file.OwnerID, 128),
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"files": files, "next_page_token": bounded(data.NextPageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) readDocument(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	documentID := strings.TrimSpace(inputString(input, "document_id"))
	if !validFeishuToken(documentID) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu document id is invalid", nil)
	}
	maxCharacters := inputInteger(input, "max_characters", 20_000, 1, 50_000)
	var data feishuDocumentData
	meta, err := adapter.client.getDocumentRawContent(ctx, region, token, documentID, &data)
	if err != nil {
		return nil, meta, err
	}
	content, truncated := truncateRunes(strings.TrimSpace(data.Content), maxCharacters)
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"document_id": documentID, "content": content, "truncated": truncated,
	}, meta, nil
}

func (adapter *Adapter) searchContacts(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	queryText := strings.TrimSpace(inputString(input, "query"))
	if queryText == "" || len([]rune(queryText)) > 128 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu contact search query is invalid", nil)
	}
	pageSize := inputInteger(input, "page_size", 20, 1, 50)
	query := url.Values{
		"query":     []string{queryText},
		"page_size": []string{strconv.Itoa(pageSize)},
	}
	if pageToken := bounded(inputString(input, "page_token"), 1024); pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var data feishuUserSearchData
	meta, err := adapter.client.searchUsers(ctx, region, token, query, &data)
	if err != nil {
		return nil, meta, err
	}
	users := make([]interface{}, 0, min(len(data.Users), pageSize))
	for index, user := range data.Users {
		if index >= pageSize {
			break
		}
		users = append(users, map[string]interface{}{
			"open_id":        bounded(user.OpenID, 128),
			"user_id":        bounded(user.UserID, 128),
			"name":           bounded(user.Name, 255),
			"department_ids": boundedStrings(user.DepartmentIDs, 50, 128),
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"users": users, "next_page_token": bounded(data.PageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) listChats(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	pageSize := inputInteger(input, "page_size", 20, 1, 50)
	query := url.Values{"page_size": []string{strconv.Itoa(pageSize)}}
	if pageToken := bounded(inputString(input, "page_token"), 1024); pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var data feishuChatsData
	meta, err := adapter.client.listChats(ctx, region, token, query, &data)
	if err != nil {
		return nil, meta, err
	}
	chats := make([]interface{}, 0, min(len(data.Items), pageSize))
	for index, chat := range data.Items {
		if index >= pageSize {
			break
		}
		chats = append(chats, map[string]interface{}{
			"chat_id":      bounded(chat.ChatID, 255),
			"name":         bounded(chat.Name, 500),
			"description":  bounded(chat.Description, 2000),
			"owner_id":     bounded(chat.OwnerID, 128),
			"chat_mode":    bounded(chat.ChatMode, 32),
			"chat_type":    bounded(chat.ChatType, 32),
			"member_count": max(chat.MemberCount, 0),
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"chats": chats, "next_page_token": bounded(data.PageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) listMessages(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	chatID := strings.TrimSpace(inputString(input, "chat_id"))
	if !validFeishuToken(chatID) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu chat id is invalid", nil)
	}
	pageSize, err := strictInputInteger(input, "page_size", 20, 1, 50)
	if err != nil {
		return nil, responseMeta{}, err
	}
	query := url.Values{
		"container_id_type": []string{"chat"},
		"container_id":      []string{chatID},
		"page_size":         []string{strconv.Itoa(pageSize)},
	}
	pageToken := strings.TrimSpace(inputString(input, "page_token"))
	if len([]rune(pageToken)) > 1024 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu page token is invalid", nil)
	}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	sortType := strings.TrimSpace(inputString(input, "sort_type"))
	switch sortType {
	case "", "newest_first":
		query.Set("sort_type", "ByCreateTimeDesc")
	case "oldest_first":
		query.Set("sort_type", "ByCreateTimeAsc")
	default:
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu message sort order is invalid", nil)
	}
	startTime, hasStart, err := optionalInputUnixSeconds(input, "start_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	endTime, hasEnd, err := optionalInputUnixSeconds(input, "end_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	if hasStart != hasEnd || (hasStart && (endTime <= startTime || endTime-startTime > 7*24*60*60)) {
		return nil, responseMeta{}, integrations.NewError(
			integrations.ErrorCodeInvalidInput,
			"Feishu message time range must include start and end and cannot exceed seven days",
			nil,
		)
	}
	if hasStart {
		query.Set("start_time", strconv.FormatInt(startTime, 10))
		query.Set("end_time", strconv.FormatInt(endTime, 10))
	}
	var data feishuMessagesData
	meta, err := adapter.client.listMessages(ctx, region, token, query, &data)
	if err != nil {
		return nil, meta, err
	}
	messages := make([]interface{}, 0, min(len(data.Items), pageSize))
	for index, message := range data.Items {
		if index >= pageSize {
			break
		}
		messages = append(messages, map[string]interface{}{
			"message_id": bounded(message.MessageID, 255), "root_id": bounded(message.RootID, 255),
			"parent_id": bounded(message.ParentID, 255), "thread_id": bounded(message.ThreadID, 255),
			"chat_id": bounded(message.ChatID, 255), "message_type": bounded(message.MsgType, 64),
			"text":      normalizedMessageText(message.MsgType, message.Body.Content),
			"sender_id": bounded(message.Sender.ID, 128), "sender_type": bounded(message.Sender.SenderType, 64),
			"create_time": bounded(message.CreateTime, 64), "update_time": bounded(message.UpdateTime, 64),
			"deleted": message.Deleted, "updated": message.Updated,
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"messages": messages, "next_page_token": bounded(data.PageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) listCalendars(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	const pageSize = 50
	query := url.Values{"page_size": []string{strconv.Itoa(pageSize)}}
	pageToken := bounded(inputString(input, "page_token"), 1024)
	syncToken := bounded(inputString(input, "sync_token"), 1024)
	if pageToken != "" && syncToken != "" {
		return nil, responseMeta{}, integrations.NewError(
			integrations.ErrorCodeInvalidInput,
			"Feishu page_token and sync_token cannot be used together",
			nil,
		)
	}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	if syncToken != "" {
		query.Set("sync_token", syncToken)
	}
	var data feishuCalendarsData
	meta, err := adapter.client.listCalendars(ctx, region, token, query, &data)
	if err != nil {
		return nil, meta, err
	}
	calendars := make([]interface{}, 0, min(len(data.CalendarList), pageSize))
	for index, calendar := range data.CalendarList {
		if index >= pageSize {
			break
		}
		calendars = append(calendars, map[string]interface{}{
			"calendar_id":    bounded(calendar.CalendarID, 512),
			"summary":        bounded(calendar.Summary, 255),
			"description":    bounded(calendar.Description, 1000),
			"permissions":    bounded(calendar.Permissions, 64),
			"type":           bounded(calendar.Type, 64),
			"role":           bounded(calendar.Role, 64),
			"is_deleted":     calendar.IsDeleted,
			"is_third_party": calendar.IsThirdParty,
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"calendars": calendars, "next_page_token": bounded(data.PageToken, 1024),
		"sync_token": bounded(data.SyncToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) listCalendarEvents(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	calendarID := strings.TrimSpace(inputString(input, "calendar_id"))
	if !validFeishuCalendarID(calendarID) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu calendar id is invalid", nil)
	}
	startTime, err := requiredInputUnixSeconds(input, "start_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	endTime, err := requiredInputUnixSeconds(input, "end_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	if endTime <= startTime || endTime-startTime > 40*24*60*60 {
		return nil, responseMeta{}, integrations.NewError(
			integrations.ErrorCodeInvalidInput,
			"Feishu calendar event time range must be positive and cannot exceed forty days",
			nil,
		)
	}
	// Feishu calendar event pagination currently accepts page_size in the
	// provider range [50, 1000]. ZGI deliberately fixes the public Action to
	// 50 so one tool result remains bounded while omitted arguments still
	// produce a provider-valid request.
	pageSize, err := strictInputInteger(input, "page_size", 50, 50, 50)
	if err != nil {
		return nil, responseMeta{}, err
	}
	query := url.Values{
		"start_time": []string{strconv.FormatInt(startTime, 10)},
		"end_time":   []string{strconv.FormatInt(endTime, 10)},
		"page_size":  []string{strconv.Itoa(pageSize)},
	}
	pageToken := strings.TrimSpace(inputString(input, "page_token"))
	if len([]rune(pageToken)) > 1024 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu page token is invalid", nil)
	}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var data feishuCalendarEventsData
	meta, err := adapter.client.listCalendarEvents(ctx, region, token, calendarID, query, &data)
	if err != nil {
		return nil, meta, err
	}
	events := make([]interface{}, 0, min(len(data.Items), pageSize))
	for index, event := range data.Items {
		if index >= pageSize {
			break
		}
		events = append(events, normalizedCalendarEvent(event))
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"events": events, "next_page_token": bounded(data.PageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) createCalendarEvent(ctx context.Context, region, token string, request integrations.ActionRequest) (map[string]interface{}, responseMeta, error) {
	calendarID := strings.TrimSpace(inputString(request.Input, "calendar_id"))
	if !validFeishuCalendarID(calendarID) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu calendar id is invalid", nil)
	}
	summary := strings.TrimSpace(inputString(request.Input, "summary"))
	description := strings.TrimSpace(inputString(request.Input, "description"))
	if summary == "" || len([]rune(summary)) > 1000 || len([]rune(description)) > 40960 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu calendar event title or description is invalid", nil)
	}
	startTime, err := requiredInputUnixSeconds(request.Input, "start_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	endTime, err := requiredInputUnixSeconds(request.Input, "end_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	if endTime <= startTime || endTime-startTime > 366*24*60*60 {
		return nil, responseMeta{}, integrations.NewError(
			integrations.ErrorCodeInvalidInput,
			"Feishu calendar event time range must be positive and cannot exceed 366 days",
			nil,
		)
	}
	timezone := strings.TrimSpace(inputString(request.Input, "timezone"))
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	if !validIANATimezone(timezone) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu calendar event timezone is invalid", nil)
	}
	idempotencyKey, err := calendarEventIdempotencyKey(request)
	if err != nil {
		return nil, responseMeta{}, err
	}
	visibility, err := strictInputEnum(request.Input, "visibility", "default", "default", "public", "private")
	if err != nil {
		return nil, responseMeta{}, err
	}
	freeBusyStatus, err := strictInputEnum(request.Input, "free_busy_status", "busy", "busy", "free")
	if err != nil {
		return nil, responseMeta{}, err
	}
	needNotification, err := strictInputBoolean(request.Input, "need_notification", true)
	if err != nil {
		return nil, responseMeta{}, err
	}
	body := map[string]interface{}{
		"summary": summary, "description": description,
		"need_notification": needNotification,
		"start_time":        map[string]string{"timestamp": strconv.FormatInt(startTime, 10), "timezone": timezone},
		"end_time":          map[string]string{"timestamp": strconv.FormatInt(endTime, 10), "timezone": timezone},
		"visibility":        visibility,
		"free_busy_status":  freeBusyStatus,
	}
	locationName := strings.TrimSpace(inputString(request.Input, "location_name"))
	locationAddress := strings.TrimSpace(inputString(request.Input, "location_address"))
	if len([]rune(locationName)) > 512 || len([]rune(locationAddress)) > 1000 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu calendar event location is invalid", nil)
	}
	if locationName != "" || locationAddress != "" {
		body["location"] = map[string]string{"name": locationName, "address": locationAddress}
	}
	var data feishuCalendarEventCreateData
	meta, err := adapter.client.createCalendarEvent(ctx, region, token, calendarID, idempotencyKey, body, &data)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(data.Event.EventID) == "" {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu calendar event response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"event": normalizedCalendarEvent(data.Event),
	}, meta, nil
}

func (adapter *Adapter) sendMessage(
	ctx context.Context,
	region string,
	token string,
	connection *integrations.ResolvedConnection,
	input map[string]interface{},
	allowSelf bool,
) (map[string]interface{}, responseMeta, error) {
	recipientType := inputEnum(input, "recipient_type", "open_id", "self", "open_id", "user_id", "union_id", "chat_id")
	receiveID := strings.TrimSpace(inputString(input, "recipient_id"))
	receiveIDType := recipientType
	if recipientType == "self" {
		if !allowSelf || connection == nil {
			return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu self recipient is unavailable for this action", nil)
		}
		receiveID = strings.TrimSpace(connection.AccountID)
		receiveIDType = "open_id"
	}
	text := strings.TrimSpace(inputString(input, "text"))
	if !validFeishuToken(receiveID) || text == "" || len([]rune(text)) > 10_000 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu message target or text is invalid", nil)
	}
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu message could not be encoded", err)
	}
	var data feishuMessageData
	meta, err := adapter.client.sendMessage(ctx, region, token, receiveIDType, map[string]interface{}{
		"receive_id": receiveID, "msg_type": "text", "content": string(content),
	}, &data)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(data.MessageID) == "" {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu message response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"message": map[string]interface{}{
			"message_id": bounded(data.MessageID, 255), "root_id": bounded(data.RootID, 255),
			"parent_id": bounded(data.ParentID, 255), "create_time": bounded(data.CreateTime, 64),
		},
	}, meta, nil
}

func (adapter *Adapter) connectionAccessToken(ctx context.Context, connection *integrations.ResolvedConnection, region string) (string, responseMeta, error) {
	if isTenantAppConnection(connection) {
		return adapter.tenantAccessToken(ctx, connection, region)
	}
	token, err := feishuUserAccessToken(connection)
	return token, responseMeta{}, err
}

func (adapter *Adapter) tenantAccessToken(ctx context.Context, connection *integrations.ResolvedConnection, region string) (string, responseMeta, error) {
	if connection == nil || !isTenantAppConnection(connection) {
		return "", responseMeta{}, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Feishu tenant app connection is required", nil)
	}
	appID := strings.TrimSpace(connection.Credentials["app_id"])
	appSecret := strings.TrimSpace(connection.Credentials["app_secret"])
	if appID == "" || appSecret == "" {
		return "", responseMeta{}, integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu tenant app credentials are unavailable", nil)
	}
	return adapter.client.tenantAccessToken(ctx, region, appID, appSecret)
}

func feishuUserAccessToken(connection *integrations.ResolvedConnection) (string, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) ||
		!strings.EqualFold(connection.DriverID, DriverID) || isTenantAppConnection(connection) {
		return "", integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Feishu user OAuth connection is required", nil)
	}
	token := strings.TrimSpace(connection.Credentials["access_token"])
	if token == "" {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu user credentials are unavailable", nil)
	}
	return token, nil
}

func feishuRegion(connection *integrations.ResolvedConnection) (string, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) ||
		!strings.EqualFold(connection.DriverID, DriverID) {
		return "", integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Feishu connection is invalid", nil)
	}
	// This provider is the Feishu China product. Lark uses distinct OAuth and
	// data-egress hosts and will be exposed as a separate provider so approval
	// records always name the exact external destination.
	return RegionCN, nil
}

func isTenantAppConnection(connection *integrations.ResolvedConnection) bool {
	return connection != nil && strings.EqualFold(connection.AuthMethodID, TenantAppAuthMethodID)
}

func validFeishuToken(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func validFeishuCalendarID(value string) bool {
	if value == "" || len(value) > 512 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' || char == '@' {
			continue
		}
		return false
	}
	return true
}

func validIANATimezone(value string) bool {
	if value == "" || len(value) > 64 || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return false
	}
	segments := strings.Split(value, "/")
	for index, segment := range segments {
		if segment == "" {
			return false
		}
		for _, char := range segment {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' ||
				(index > 0 && ((char >= '0' && char <= '9') || char == '-' || char == '+' || char == '.')) {
				continue
			}
			return false
		}
	}
	return true
}

func normalizedMessageText(messageType, raw string) string {
	if !strings.EqualFold(strings.TrimSpace(messageType), "text") {
		return ""
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return ""
	}
	return bounded(content.Text, 4000)
}

func normalizedCalendarEvent(event feishuCalendarEvent) map[string]interface{} {
	return map[string]interface{}{
		"event_id": bounded(event.EventID, 255), "organizer_calendar_id": bounded(event.OrganizerCalendarID, 512),
		"summary": bounded(event.Summary, 1000), "description": bounded(event.Description, 4000),
		"start_time": normalizedCalendarTime(event.StartTime), "end_time": normalizedCalendarTime(event.EndTime),
		"status": bounded(event.Status, 64), "visibility": bounded(event.Visibility, 32),
		"free_busy_status": bounded(event.FreeBusyStatus, 32),
		"location_name":    bounded(event.Location.Name, 512), "location_address": bounded(event.Location.Address, 1000),
		"app_link": safeFeishuURL(event.AppLink), "recurrence": bounded(event.Recurrence, 2000),
		"is_exception": event.IsException,
	}
}

func normalizedCalendarTime(info feishuCalendarTimeInfo) map[string]interface{} {
	return map[string]interface{}{
		"timestamp": bounded(info.Timestamp, 32), "date": bounded(info.Date, 32), "timezone": bounded(info.Timezone, 64),
	}
}

func calendarEventIdempotencyKey(request integrations.ActionRequest) (string, error) {
	messageID := strings.TrimSpace(request.MessageID)
	if messageID == "" {
		return "", integrations.NewError(
			integrations.ErrorCodeInvalidInput,
			"Feishu calendar event creation requires a request idempotency marker",
			nil,
		)
	}
	calendarID := strings.TrimSpace(inputString(request.Input, "calendar_id"))
	if calendarID == "" {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu calendar event calendar id is required", nil)
	}
	batchID := strings.TrimSpace(request.BatchID)
	operationItemID := strings.TrimSpace(request.OperationItemID)
	if batchID == "" {
		batchID = "single-" + messageID
	}
	if operationItemID == "" {
		operationItemID = "single"
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(request.OrganizationID), strings.TrimSpace(request.ConnectionID),
		messageID, ActionCreateEvent, calendarID, batchID, operationItemID,
	}, "\x00")))
	return hex.EncodeToString(digest[:]), nil
}

func feishuActionResult(output map[string]interface{}, meta responseMeta, count int) *integrations.ActionResult {
	diagnostics := meta.Diagnostics
	if diagnostics.RequestID == "" {
		diagnostics.RequestID = bounded(meta.RequestID, 128)
	}
	if output == nil && meta.Attempts <= 0 && diagnostics == (integrations.ProviderDiagnostics{}) {
		return nil
	}
	if output == nil {
		count = 0
	}
	attempts := meta.Attempts
	if attempts <= 0 && output != nil {
		attempts = 1
	}
	return &integrations.ActionResult{
		Output: output, ProviderRequestID: bounded(meta.RequestID, 128),
		ProviderDiagnostics: diagnostics,
		ResultCount:         max(count, 0), AttemptCount: max(attempts, 0),
	}
}

func outputCount(output map[string]interface{}, key string) int {
	if values, ok := output[key].([]interface{}); ok {
		return len(values)
	}
	return 0
}

type feishuUserData struct {
	OpenID    string `json:"open_id"`
	UnionID   string `json:"union_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	EnName    string `json:"en_name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
	TenantKey string `json:"tenant_key"`
}

type feishuDriveFilesData struct {
	Files         []feishuDriveFile `json:"files"`
	NextPageToken string            `json:"next_page_token"`
	HasMore       bool              `json:"has_more"`
}

type feishuDriveFile struct {
	Token        string `json:"token"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	ParentToken  string `json:"parent_token"`
	URL          string `json:"url"`
	CreatedTime  string `json:"created_time"`
	ModifiedTime string `json:"modified_time"`
	OwnerID      string `json:"owner_id"`
}

type feishuDocumentData struct {
	Content string `json:"content"`
}

type feishuUserSearchData struct {
	Users     []feishuSearchUser `json:"users"`
	PageToken string             `json:"page_token"`
	HasMore   bool               `json:"has_more"`
}

type feishuSearchUser struct {
	OpenID        string   `json:"open_id"`
	UserID        string   `json:"user_id"`
	Name          string   `json:"name"`
	DepartmentIDs []string `json:"department_ids"`
}

type feishuChatsData struct {
	Items     []feishuChat `json:"items"`
	PageToken string       `json:"page_token"`
	HasMore   bool         `json:"has_more"`
}

type feishuMessagesData struct {
	Items     []feishuMessage `json:"items"`
	PageToken string          `json:"page_token"`
	HasMore   bool            `json:"has_more"`
}

type feishuMessage struct {
	MessageID  string `json:"message_id"`
	RootID     string `json:"root_id"`
	ParentID   string `json:"parent_id"`
	ThreadID   string `json:"thread_id"`
	ChatID     string `json:"chat_id"`
	MsgType    string `json:"msg_type"`
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
	Deleted    bool   `json:"deleted"`
	Updated    bool   `json:"updated"`
	Sender     struct {
		ID         string `json:"id"`
		IDType     string `json:"id_type"`
		SenderType string `json:"sender_type"`
	} `json:"sender"`
	Body struct {
		Content string `json:"content"`
	} `json:"body"`
}

type feishuCalendarsData struct {
	CalendarList []feishuCalendar `json:"calendar_list"`
	PageToken    string           `json:"page_token"`
	SyncToken    string           `json:"sync_token"`
	HasMore      bool             `json:"has_more"`
}

type feishuCalendarEventsData struct {
	Items     []feishuCalendarEvent `json:"items"`
	PageToken string                `json:"page_token"`
	HasMore   bool                  `json:"has_more"`
}

type feishuCalendarEventCreateData struct {
	Event feishuCalendarEvent `json:"event"`
}

type feishuCalendarEvent struct {
	EventID             string                 `json:"event_id"`
	OrganizerCalendarID string                 `json:"organizer_calendar_id"`
	Summary             string                 `json:"summary"`
	Description         string                 `json:"description"`
	StartTime           feishuCalendarTimeInfo `json:"start_time"`
	EndTime             feishuCalendarTimeInfo `json:"end_time"`
	Status              string                 `json:"status"`
	Visibility          string                 `json:"visibility"`
	FreeBusyStatus      string                 `json:"free_busy_status"`
	Recurrence          string                 `json:"recurrence"`
	AppLink             string                 `json:"app_link"`
	IsException         bool                   `json:"is_exception"`
	Location            struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"location"`
}

type feishuCalendarTimeInfo struct {
	Timestamp string `json:"timestamp"`
	Date      string `json:"date"`
	Timezone  string `json:"timezone"`
}

type feishuCalendar struct {
	CalendarID   string `json:"calendar_id"`
	Summary      string `json:"summary"`
	Description  string `json:"description"`
	Permissions  string `json:"permissions"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	IsDeleted    bool   `json:"is_deleted"`
	IsThirdParty bool   `json:"is_third_party"`
}

type feishuChat struct {
	ChatID      string `json:"chat_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id"`
	ChatMode    string `json:"chat_mode"`
	ChatType    string `json:"chat_type"`
	MemberCount int    `json:"member_count"`
}

type feishuMessageData struct {
	MessageID  string `json:"message_id"`
	RootID     string `json:"root_id"`
	ParentID   string `json:"parent_id"`
	CreateTime string `json:"create_time"`
}

type feishuTenantData struct {
	Tenant struct {
		TenantKey string `json:"tenant_key"`
		Name      string `json:"name"`
		DisplayID string `json:"display_id"`
	} `json:"tenant"`
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func safeFeishuURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "feishu.cn" && !strings.HasSuffix(host, ".feishu.cn") &&
		host != "larksuite.com" && !strings.HasSuffix(host, ".larksuite.com") {
		return ""
	}
	parsed.Fragment = ""
	return bounded(parsed.String(), 2048)
}

func boundedStrings(values []string, maxItems, maxRunes int) []interface{} {
	result := make([]interface{}, 0, min(len(values), maxItems))
	for _, value := range values {
		value = bounded(value, maxRunes)
		if value == "" {
			continue
		}
		result = append(result, value)
		if len(result) >= maxItems {
			break
		}
	}
	return result
}

func truncateRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}

func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}

func inputInteger(input map[string]interface{}, key string, fallback, minimum, maximum int) int {
	value := fallback
	switch typed := input[key].(type) {
	case int:
		value = typed
	case int64:
		value = int(typed)
	case float64:
		if typed == float64(int(typed)) {
			value = int(typed)
		}
	case json.Number:
		if parsed, err := strconv.Atoi(string(typed)); err == nil {
			value = parsed
		}
	}
	return min(max(value, minimum), maximum)
}

func strictInputInteger(input map[string]interface{}, key string, fallback, minimum, maximum int) (int, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return fallback, nil
	}
	value, ok := integerValue(raw)
	if !ok || value < int64(minimum) || value > int64(maximum) {
		return 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu integer input is invalid", nil)
	}
	return int(value), nil
}

func optionalInputUnixSeconds(input map[string]interface{}, key string) (int64, bool, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	value, ok := integerValue(raw)
	if !ok || value < 1 || value > 4_102_444_800 {
		return 0, false, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu Unix timestamp is invalid", nil)
	}
	return value, true, nil
}

func requiredInputUnixSeconds(input map[string]interface{}, key string) (int64, error) {
	value, found, err := optionalInputUnixSeconds(input, key)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu Unix timestamp is required", nil)
	}
	return value, nil
}

func integerValue(raw interface{}) (int64, bool) {
	switch typed := raw.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed), true
		}
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func inputEnum(input map[string]interface{}, key, fallback string, allowed ...string) string {
	value := strings.ToLower(strings.TrimSpace(inputString(input, key)))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func strictInputEnum(input map[string]interface{}, key, fallback string, allowed ...string) (string, error) {
	raw, found := input[key]
	if !found || raw == nil {
		return fallback, nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu enum input is invalid", nil)
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu enum input is invalid", nil)
}

func strictInputBoolean(input map[string]interface{}, key string, fallback bool) (bool, error) {
	raw, found := input[key]
	if !found || raw == nil {
		return fallback, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu boolean input is invalid", nil)
	}
	return value, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

var _ integrations.Adapter = (*Adapter)(nil)
var _ integrations.ConnectionTester = (*Adapter)(nil)
var _ integrations.HealthProbe = (*Adapter)(nil)
