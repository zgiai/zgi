package digitalhuman

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type ManualInvocationRequest struct {
	RequestID      uuid.UUID  `json:"request_id"`
	Query          string     `json:"query"`
	ConversationID *uuid.UUID `json:"conversation_id,omitempty"`
}

type RunListResult struct {
	Items    []*Run `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	HasMore  bool   `json:"has_more"`
}

type RunCancelResult struct {
	Run *Run `json:"run"`
}

func publicRun(run *Run) *Run {
	if run == nil {
		return nil
	}
	cloned := *run
	cloned.RunSpec = redactRunJSON(run.RunSpec)
	cloned.ExecutionSnapshot = redactRunJSON(run.ExecutionSnapshot)
	cloned.ExecutionTrace = redactRunJSON(run.ExecutionTrace)
	return &cloned
}

func publicRunList(result *RunListResult) *RunListResult {
	if result == nil {
		return nil
	}
	cloned := *result
	cloned.Items = make([]*Run, 0, len(result.Items))
	for _, run := range result.Items {
		cloned.Items = append(cloned.Items, publicRun(run))
	}
	return &cloned
}

func redactRunJSON(value datatypes.JSON) datatypes.JSON {
	if len(value) == 0 {
		return value
	}
	var decoded interface{}
	if json.Unmarshal(value, &decoded) != nil {
		return datatypes.JSON(`{}`)
	}
	redactRunValue(decoded)
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return datatypes.JSON(`{}`)
	}
	return datatypes.JSON(encoded)
}

func redactRunValue(value interface{}) {
	forbidden := map[string]struct{}{
		"credential_ref": {}, "secret": {}, "password": {}, "api_key": {}, "apikey": {},
		"access_token": {}, "refresh_token": {}, "cookie": {}, "token": {},
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if _, blocked := forbidden[strings.ToLower(strings.TrimSpace(key))]; blocked {
				delete(typed, key)
				continue
			}
			redactRunValue(item)
		}
	case []interface{}:
		for _, item := range typed {
			redactRunValue(item)
		}
	}
}
