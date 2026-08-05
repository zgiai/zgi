package dingtalk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type departmentReference struct {
	Version      int    `json:"v"`
	ConnectionID string `json:"c"`
	DepartmentID int64  `json:"d"`
}

type roleReference struct {
	Version      int    `json:"v"`
	ConnectionID string `json:"c"`
	RoleID       int64  `json:"r"`
}

type departmentDetails struct {
	ID       int64  `json:"dept_id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parent_id"`
}

func (adapter *Adapter) searchDepartments(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	query := strings.TrimSpace(inputString(input, "query"))
	if query == "" {
		return nil, 0, dingError(integrations.ErrorCodeInvalidInput, "DingTalk department search query is required", nil)
	}
	limit := boundedLimit(inputInt(input, "max_results", 10), 20, 10)
	var response map[string]json.RawMessage
	if err := adapter.apiJSON(ctx, creds, http.MethodPost, "/v1.0/contact/departments/search", map[string]interface{}{"queryWord": query, "offset": 0, "size": limit}, &response); err != nil {
		return nil, 0, err
	}
	items := extractSearchItems(response)
	departments := make([]map[string]interface{}, 0, min(limit, len(items)))
	for _, item := range items {
		if len(departments) >= limit {
			break
		}
		departmentID := rawInt64(item, "deptId", "dept_id", "id")
		if departmentID <= 0 {
			continue
		}
		details, err := adapter.getDepartmentByID(ctx, creds, departmentID)
		if err != nil {
			return nil, 0, err
		}
		departments = append(departments, departmentOutput(creds.ConnectionID, details))
	}
	return map[string]interface{}{"provider": IntegrationID, "departments": departments}, len(departments), nil
}

func (adapter *Adapter) getDepartment(ctx context.Context, creds credentials, encoded string) (map[string]interface{}, error) {
	departmentID, err := decodeDepartmentRef(encoded, creds.ConnectionID)
	if err != nil {
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk department reference is invalid", err)
	}
	details, err := adapter.getDepartmentByID(ctx, creds, departmentID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"provider": IntegrationID, "department": departmentOutput(creds.ConnectionID, details)}, nil
}

func (adapter *Adapter) getDepartmentByID(ctx context.Context, creds credentials, departmentID int64) (departmentDetails, error) {
	var response struct {
		legacyEnvelope
		Result departmentDetails `json:"result"`
	}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/v2/department/get", map[string]interface{}{"dept_id": departmentID, "language": "zh_CN"}, &response); err != nil {
		return departmentDetails{}, err
	}
	if response.Result.ID <= 0 || strings.TrimSpace(response.Result.Name) == "" {
		return departmentDetails{}, dingError(integrations.ErrorCodeResponseInvalid, "DingTalk department response is incomplete", nil)
	}
	return response.Result, nil
}

func (adapter *Adapter) listDepartmentMembers(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	departmentID, err := decodeDepartmentRef(inputString(input, "department_ref"), creds.ConnectionID)
	if err != nil {
		return nil, 0, dingError(integrations.ErrorCodeInvalidInput, "DingTalk department reference is invalid", err)
	}
	limit := boundedLimit(inputInt(input, "max_results", 50), 100, 50)
	var response struct {
		legacyEnvelope
		Result struct {
			HasMore bool `json:"has_more"`
			List    []struct {
				UserID string `json:"userid"`
				Name   string `json:"name"`
				Title  string `json:"title"`
				Active bool   `json:"active"`
			} `json:"list"`
		} `json:"result"`
	}
	payload := map[string]interface{}{"dept_id": departmentID, "cursor": 0, "size": limit, "order_field": "entry_asc", "contain_access_limit": false, "language": "zh_CN"}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/v2/user/list", payload, &response); err != nil {
		return nil, 0, err
	}
	members := make([]map[string]interface{}, 0, min(limit, len(response.Result.List)))
	for _, user := range response.Result.List {
		if len(members) >= limit || !validOpaqueID(user.UserID, 128) {
			continue
		}
		members = append(members, memberOutput(creds.ConnectionID, user.UserID, user.Name, user.Title, &user.Active))
	}
	return map[string]interface{}{"provider": IntegrationID, "members": members, "has_more": response.Result.HasMore}, len(members), nil
}

func (adapter *Adapter) listRoles(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	limit := boundedLimit(inputInt(input, "max_results", 50), 100, 50)
	var response struct {
		legacyEnvelope
		Result struct {
			HasMore bool `json:"hasMore"`
			List    []struct {
				Name  string `json:"name"`
				Roles []struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				} `json:"roles"`
			} `json:"list"`
		} `json:"result"`
	}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/role/list", map[string]interface{}{"offset": 0, "size": limit}, &response); err != nil {
		return nil, 0, err
	}
	roles := make([]map[string]interface{}, 0, limit)
	for _, group := range response.Result.List {
		for _, role := range group.Roles {
			if len(roles) >= limit {
				break
			}
			if role.ID <= 0 {
				continue
			}
			roles = append(roles, map[string]interface{}{"role_ref": encodeRoleRef(creds.ConnectionID, role.ID), "name": bounded(role.Name, 255), "group_name": bounded(group.Name, 255)})
		}
	}
	hasMore := response.Result.HasMore || len(roles) >= limit
	return map[string]interface{}{"provider": IntegrationID, "roles": roles, "has_more": hasMore}, len(roles), nil
}

func (adapter *Adapter) listRoleMembers(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	roleID, err := decodeRoleRef(inputString(input, "role_ref"), creds.ConnectionID)
	if err != nil {
		return nil, 0, dingError(integrations.ErrorCodeInvalidInput, "DingTalk role reference is invalid", err)
	}
	limit := boundedLimit(inputInt(input, "max_results", 50), 100, 50)
	var response struct {
		legacyEnvelope
		Result struct {
			HasMore bool `json:"hasMore"`
			List    []struct {
				UserID string `json:"userid"`
				Name   string `json:"name"`
			} `json:"list"`
		} `json:"result"`
	}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/role/simplelist", map[string]interface{}{"role_id": roleID, "offset": 0, "size": limit}, &response); err != nil {
		return nil, 0, err
	}
	members := make([]map[string]interface{}, 0, min(limit, len(response.Result.List)))
	for _, user := range response.Result.List {
		if len(members) >= limit || !validOpaqueID(user.UserID, 128) {
			continue
		}
		members = append(members, memberOutput(creds.ConnectionID, user.UserID, user.Name, "", nil))
	}
	return map[string]interface{}{"provider": IntegrationID, "members": members, "has_more": response.Result.HasMore}, len(members), nil
}

func (adapter *Adapter) listAttendanceRecords(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	recipientRef := strings.TrimSpace(inputString(input, "recipient_ref"))
	userID, err := decodeRecipientRef(recipientRef, creds.ConnectionID)
	if err != nil {
		return nil, 0, dingError(integrations.ErrorCodeInvalidInput, "DingTalk recipient reference is invalid", err)
	}
	start, end, err := attendanceWindow(inputString(input, "start_time"), inputString(input, "end_time"))
	if err != nil {
		return nil, 0, dingError(integrations.ErrorCodeInvalidInput, "DingTalk attendance time range is invalid", err)
	}
	var response struct {
		legacyEnvelope
		Records []struct {
			WorkDate       int64  `json:"workDate"`
			PlanCheckTime  int64  `json:"planCheckTime"`
			UserCheckTime  int64  `json:"userCheckTime"`
			CheckType      string `json:"checkType"`
			TimeResult     string `json:"timeResult"`
			LocationResult string `json:"locationResult"`
			SourceType     string `json:"sourceType"`
		} `json:"recordresult"`
	}
	shanghai := shanghaiLocation()
	payload := map[string]interface{}{
		"userIds":       []string{userID},
		"checkDateFrom": start.In(shanghai).Format("2006-01-02 15:04:05"),
		"checkDateTo":   end.In(shanghai).Format("2006-01-02 15:04:05"),
		"isI18n":        false,
	}
	if err := adapter.legacyJSON(ctx, creds, "/attendance/listRecord", payload, &response); err != nil {
		return nil, 0, err
	}
	if len(response.Records) > 100 {
		response.Records = response.Records[:100]
	}
	records := make([]map[string]interface{}, 0, len(response.Records))
	for _, record := range response.Records {
		records = append(records, map[string]interface{}{
			"work_time":       millisTime(record.WorkDate),
			"planned_time":    millisTime(record.PlanCheckTime),
			"actual_time":     millisTime(record.UserCheckTime),
			"check_type":      bounded(record.CheckType, 32),
			"time_result":     bounded(record.TimeResult, 64),
			"location_result": bounded(record.LocationResult, 64),
			"source_type":     bounded(record.SourceType, 64),
		})
	}
	return map[string]interface{}{"provider": IntegrationID, "recipient_ref": recipientRef, "records": records}, len(records), nil
}

func (adapter *Adapter) sendDepartment(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, error) {
	departmentRef := strings.TrimSpace(inputString(input, "department_ref"))
	departmentID, err := decodeDepartmentRef(departmentRef, creds.ConnectionID)
	if err != nil {
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk department reference is invalid", err)
	}
	content := strings.TrimSpace(inputString(input, "content"))
	if content == "" || len([]rune(content)) > 2048 {
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk notification content is invalid", nil)
	}
	agentID, _ := strconv.ParseInt(creds.AgentID, 10, 64)
	var response struct {
		legacyEnvelope
		TaskID int64 `json:"task_id"`
	}
	payload := map[string]interface{}{
		"agent_id": agentID, "dept_id_list": strconv.FormatInt(departmentID, 10), "to_all_user": false,
		"msg": map[string]interface{}{"msgtype": "text", "text": map[string]string{"content": content}},
	}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/message/corpconversation/asyncsend_v2", payload, &response); err != nil {
		return nil, err
	}
	if response.TaskID <= 0 {
		return nil, dingError(integrations.ErrorCodeResponseInvalid, "DingTalk notification response is incomplete", nil)
	}
	messageRef := encodeDepartmentMessageRef(creds.ConnectionID, response.TaskID, departmentID)
	return map[string]interface{}{"provider": IntegrationID, "notification": map[string]interface{}{"message_ref": messageRef, "department_ref": departmentRef, "provider_accepted": true, "delivery_status": "pending"}}, nil
}

func encodeDepartmentRef(connectionID string, departmentID int64) string {
	raw, _ := json.Marshal(departmentReference{Version: 1, ConnectionID: connectionID, DepartmentID: departmentID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeDepartmentRef(value, connectionID string) (int64, error) {
	var ref departmentReference
	if err := decodeRef(value, &ref); err != nil {
		return 0, err
	}
	if ref.Version != 1 || ref.ConnectionID != connectionID || ref.DepartmentID <= 0 {
		return 0, fmt.Errorf("invalid department reference")
	}
	return ref.DepartmentID, nil
}

func encodeRoleRef(connectionID string, roleID int64) string {
	raw, _ := json.Marshal(roleReference{Version: 1, ConnectionID: connectionID, RoleID: roleID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeRoleRef(value, connectionID string) (int64, error) {
	var ref roleReference
	if err := decodeRef(value, &ref); err != nil {
		return 0, err
	}
	if ref.Version != 1 || ref.ConnectionID != connectionID || ref.RoleID <= 0 {
		return 0, fmt.Errorf("invalid role reference")
	}
	return ref.RoleID, nil
}

func encodeDepartmentMessageRef(connectionID string, taskID, departmentID int64) string {
	raw, _ := json.Marshal(messageReference{Version: 1, ConnectionID: connectionID, TaskID: taskID, DepartmentID: departmentID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func departmentOutput(connectionID string, department departmentDetails) map[string]interface{} {
	return map[string]interface{}{
		"department_ref": encodeDepartmentRef(connectionID, department.ID),
		"name":           bounded(department.Name, 255),
		"parent_id":      department.ParentID,
	}
}

func memberOutput(connectionID, userID, name, title string, active *bool) map[string]interface{} {
	result := map[string]interface{}{
		"recipient_ref": encodeRecipientRef(connectionID, userID),
		"name":          bounded(name, 255),
		"title":         bounded(title, 255),
	}
	if active != nil {
		result["active"] = *active
	}
	return result
}

func extractSearchItems(response map[string]json.RawMessage) []json.RawMessage {
	for _, key := range []string{"result", "list"} {
		raw := response[key]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil {
			return items
		}
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil {
			if json.Unmarshal(nested["list"], &items) == nil {
				return items
			}
		}
	}
	return nil
}

func rawInt64(raw json.RawMessage, keys ...string) int64 {
	var number int64
	if json.Unmarshal(raw, &number) == nil {
		return number
	}
	var textValue string
	if json.Unmarshal(raw, &textValue) == nil {
		number, _ = strconv.ParseInt(textValue, 10, 64)
		return number
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return 0
	}
	for _, key := range keys {
		if value := rawInt64(object[key]); value > 0 {
			return value
		}
	}
	return 0
}

func attendanceWindow(startRaw, endRaw string) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(startRaw))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(endRaw))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !end.After(start) || end.Sub(start) > 7*24*time.Hour || start.Before(time.Now().AddDate(0, -6, 0)) {
		return time.Time{}, time.Time{}, fmt.Errorf("attendance range must be positive, at most seven days, and within six months")
	}
	return start, end, nil
}

func shanghaiLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		return location
	}
	return time.FixedZone("Asia/Shanghai", 8*60*60)
}

func millisTime(value int64) string {
	return time.UnixMilli(value).UTC().Format(time.RFC3339)
}

func boundedLimit(value, maximum, fallback int) int {
	if value < 1 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}
