package action

import (
	"context"
	"fmt"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationnotification "github.com/zgiai/zgi/api/internal/modules/automation/service/notification"
)

// NotificationExecutionResult returns normalized execution payloads for action runs.
type NotificationExecutionResult struct {
	RequestPayload  map[string]interface{}
	ResponsePayload map[string]interface{}
}

// NotificationExecutor resolves channel configuration and dispatches notifications to sinks.
type NotificationExecutor struct {
	sinks map[automationmodel.NotificationChannelType]automationnotification.Sink
}

// NewNotificationExecutor creates a notification executor.
func NewNotificationExecutor(sinks ...automationnotification.Sink) *NotificationExecutor {
	executor := &NotificationExecutor{
		sinks: make(map[automationmodel.NotificationChannelType]automationnotification.Sink),
	}
	for _, sink := range sinks {
		if sink != nil {
			executor.sinks[sink.ChannelType()] = sink
		}
	}
	return executor
}

// ActionType returns the automation action type handled by this executor.
func (e *NotificationExecutor) ActionType() automationmodel.AutomationActionType {
	return automationmodel.AutomationActionTypeSendNotification
}

// ExecuteAction adapts notification execution to the generic automation action executor interface.
func (e *NotificationExecutor) ExecuteAction(ctx context.Context, req ActionExecutionRequest) (*ActionExecutionResult, error) {
	result, err := e.Execute(ctx, req.Action)
	if err != nil {
		return nil, err
	}

	var channelType *automationmodel.NotificationChannelType
	if result != nil {
		if typed, ok := result.RequestPayload["channel_type"].(automationmodel.NotificationChannelType); ok {
			channelType = &typed
		} else if raw, ok := result.RequestPayload["channel_type"].(string); ok && raw != "" {
			typed := automationmodel.NotificationChannelType(raw)
			channelType = &typed
		}
	}

	return &ActionExecutionResult{
		RequestPayload:  result.RequestPayload,
		ResponsePayload: result.ResponsePayload,
		ChannelType:     channelType,
	}, nil
}

// Execute sends one notification action.
func (e *NotificationExecutor) Execute(ctx context.Context, action *automationmodel.AutomationTaskAction) (*NotificationExecutionResult, error) {
	if action == nil {
		return nil, fmt.Errorf("automation action is nil")
	}
	if action.ActionType != automationmodel.AutomationActionTypeSendNotification {
		return nil, fmt.Errorf("unsupported automation action type: %s", action.ActionType)
	}

	request, err := buildNotificationRequest(action.Config)
	if err != nil {
		return nil, err
	}

	sink, ok := e.sinks[request.ChannelType]
	if !ok {
		return nil, fmt.Errorf("notification sink not configured for channel %s", request.ChannelType)
	}

	result, err := sink.Send(ctx, request)
	if err != nil {
		return nil, err
	}

	responsePayload := map[string]interface{}{
		"channel_type": result.ChannelType,
		"accepted":     result.Accepted,
	}
	if result.ExternalID != nil && *result.ExternalID != "" {
		responsePayload["external_id"] = *result.ExternalID
	}

	return &NotificationExecutionResult{
		RequestPayload: map[string]interface{}{
			"channel_type":    request.ChannelType,
			"to":              request.To,
			"subject":         request.Subject,
			"body":            request.Body,
			"body_type":       request.BodyType,
			"template":        request.Template,
			"template_params": request.TemplateParams,
			"provider":        request.Provider,
		},
		ResponsePayload: responsePayload,
	}, nil
}

func buildNotificationRequest(config map[string]interface{}) (*automationnotification.Request, error) {
	if config == nil {
		return nil, fmt.Errorf("notification config is nil")
	}

	channelType, err := requiredString(config, "channel_type")
	if err != nil {
		return nil, err
	}
	to, err := requiredStringSlice(config, "to")
	if err != nil {
		return nil, err
	}

	if automationmodel.NotificationChannelType(channelType) == automationmodel.NotificationChannelTypeSMS {
		template, err := requiredString(config, "template")
		if err != nil {
			return nil, err
		}
		templateParams, err := optionalStringMap(config, "template_params")
		if err != nil {
			return nil, err
		}
		provider, _ := optionalStringValue(config, "provider")
		return &automationnotification.Request{
			ChannelType:    automationmodel.NotificationChannelType(channelType),
			To:             to,
			Template:       template,
			TemplateParams: templateParams,
			Provider:       provider,
		}, nil
	}

	subject, err := requiredString(config, "subject")
	if err != nil {
		return nil, err
	}
	body, bodyType, err := resolveBodyConfig(config)
	if err != nil {
		return nil, err
	}

	return &automationnotification.Request{
		ChannelType: automationmodel.NotificationChannelType(channelType),
		To:          to,
		Subject:     subject,
		Body:        body,
		BodyType:    bodyType,
	}, nil
}

func resolveBodyConfig(config map[string]interface{}) (string, string, error) {
	body, hasBody := optionalStringValue(config, "body")
	if hasBody {
		bodyType, hasBodyType := optionalStringValue(config, "body_type")
		if !hasBodyType {
			bodyType = "text/html"
		}
		return body, bodyType, nil
	}

	legacyHTML, err := requiredString(config, "html")
	if err != nil {
		return "", "", err
	}
	return legacyHTML, "text/html", nil
}

func requiredString(config map[string]interface{}, key string) (string, error) {
	value, ok := config[key]
	if !ok {
		return "", fmt.Errorf("notification config missing %s", key)
	}

	text, ok := value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("notification config %s must be a non-empty string", key)
	}
	return text, nil
}

func requiredStringSlice(config map[string]interface{}, key string) ([]string, error) {
	value, ok := config[key]
	if !ok {
		return nil, fmt.Errorf("notification config missing %s", key)
	}

	switch typed := value.(type) {
	case []string:
		if len(typed) == 0 {
			return nil, fmt.Errorf("notification config %s must not be empty", key)
		}
		return typed, nil
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || text == "" {
				return nil, fmt.Errorf("notification config %s contains a non-string recipient", key)
			}
			values = append(values, text)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("notification config %s must not be empty", key)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("notification config %s must be a string array", key)
	}
}

func optionalStringMap(config map[string]interface{}, key string) (map[string]string, error) {
	value, ok := config[key]
	if !ok {
		return map[string]string{}, nil
	}

	result := make(map[string]string)
	switch typed := value.(type) {
	case map[string]string:
		for k, v := range typed {
			if k == "" || v == "" {
				return nil, fmt.Errorf("notification config %s contains empty key or value", key)
			}
			result[k] = v
		}
	case map[string]interface{}:
		for k, v := range typed {
			text, ok := v.(string)
			if k == "" || !ok || text == "" {
				return nil, fmt.Errorf("notification config %s contains a non-string value", key)
			}
			result[k] = text
		}
	default:
		return nil, fmt.Errorf("notification config %s must be a string map", key)
	}
	return result, nil
}

func optionalStringValue(config map[string]interface{}, key string) (string, bool) {
	value, ok := config[key]
	if !ok {
		return "", false
	}

	text, ok := value.(string)
	if !ok || text == "" {
		return "", false
	}
	return text, true
}
