package consolenavigation

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const (
	navigationEventType = "page_navigation_requested"
	navigationStatus    = "navigation_requested"
)

var exactConsoleRoutes = map[string]string{
	"/console":                         "Home",
	"/console/work/chat":               "Conversations",
	"/console/work/image":              "Images",
	"/console/work/app":                "Apps",
	"/console/work/task":               "Scheduled Tasks",
	"/console/agents":                  "Agents",
	"/console/dataset":                 "Knowledge Bases",
	"/console/db":                      "Databases",
	"/console/files":                   "Files",
	"/console/prompts":                 "Prompts",
	"/console/developer/content-parse": "File Recognition",
	"/console/workspace":               "Workspace",
	"/console/workspace/members":       "Workspace Members",
	"/console/workspace/settings":      "Workspace Settings",
	"/console/settings":                "System Settings",
}

var dynamicConsoleRoutePatterns = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`^/console/agents/[A-Za-z0-9_-]+/(agent|workflow|logs|api|batch-test)$`), "Agent Detail"},
	{regexp.MustCompile(`^/console/dataset/[A-Za-z0-9_-]+(/(documents|graph|hit-testing|batch-testing|settings))?$`), "Knowledge Base Detail"},
	{regexp.MustCompile(`^/console/db/[A-Za-z0-9_-]+(/(record|search|table|import-excel))?$`), "Database Detail"},
	{regexp.MustCompile(`^/console/db/[A-Za-z0-9_-]+/table/[A-Za-z0-9_-]+$`), "Database Table"},
	{regexp.MustCompile(`^/console/prompts/[A-Za-z0-9_-]+$`), "Prompt Detail"},
	{regexp.MustCompile(`^/console/work/app/[A-Za-z0-9_-]+$`), "App Detail"},
}

// NavigateTool emits a frontend-readable request to switch to a safe internal console route.
type NavigateTool struct {
	*builtin.BuiltinTool
}

func NewNavigateTool(tenantID string) *NavigateTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "navigate",
			Author:   "System",
			Provider: "console_navigation",
			Label: tools.I18nText{
				"en_US":   "Navigate Console",
				"zh_Hans": "Navigate Console",
			},
			Icon: "route",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Request navigation to a whitelisted internal ZGI console page.",
				"zh_Hans": "Request navigation to a whitelisted internal ZGI console page.",
			},
			LLM: "Request navigation to a whitelisted internal ZGI console page. Use only for internal /console routes from the ZGI site map; never for external URLs or asset mutation. A successful tool result only means the route request was accepted; wait for client action/page-context evidence before using the destination page. If the current page already matches the destination, do not call navigate.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name: "href",
				Label: tools.I18nText{
					"en_US":   "Console route",
					"zh_Hans": "Console route",
				},
				HumanDescription: tools.I18nText{
					"en_US":   "Whitelisted internal console route, for example /console/files.",
					"zh_Hans": "Whitelisted internal console route, for example /console/files.",
				},
				LLMDescription: "Required whitelisted internal route. Prefer exact site-map routes such as /console/files, /console/agents, /console/dataset, /console/db, /console/work/task, /console/prompts, /console/work/chat, /console/work/image, /console/work/app, /console/workspace, or /console/settings.",
				Type:           tools.ToolParameterTypeString,
				Form:           tools.ToolParameterFormLLM,
				Required:       true,
			},
			{
				Name: "reason",
				Label: tools.I18nText{
					"en_US":   "Reason",
					"zh_Hans": "Reason",
				},
				HumanDescription: tools.I18nText{
					"en_US":   "Short reason for the route switch.",
					"zh_Hans": "Short reason for the route switch.",
				},
				LLMDescription: "Short user-facing reason for why this page is relevant.",
				Type:           tools.ToolParameterTypeString,
				Form:           tools.ToolParameterFormLLM,
				Required:       false,
			},
		},
		OutputType: "json",
		Tags:       []string{"console", "navigation"},
	}

	return &NavigateTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, tenantID),
	}
}

func (t *NavigateTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	runtime := t.Runtime()
	if runtime == nil || runtime.InvokeFrom != tools.ToolInvokeFromAIChat {
		return nil, fmt.Errorf("console navigation is only available from AIChat runtime")
	}

	href, ok := stringParam(toolParameters, "href")
	if !ok {
		return nil, fmt.Errorf("href is required")
	}

	normalizedHref, label, err := normalizeConsoleRoute(href)
	if err != nil {
		return nil, err
	}

	reason, _ := stringParam(toolParameters, "reason")
	result := map[string]interface{}{
		"status":     navigationStatus,
		"event_type": navigationEventType,
		"href":       normalizedHref,
		"label":      label,
	}
	if reason != "" {
		result["reason"] = reason
	}

	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(result)}, nil
}

func (t *NavigateTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &NavigateTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
	}
}

func stringParam(parameters map[string]interface{}, key string) (string, bool) {
	value, ok := parameters[key]
	if !ok || value == nil {
		return "", false
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	return text, text != ""
}

func normalizeConsoleRoute(rawHref string) (string, string, error) {
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return "", "", fmt.Errorf("href is required")
	}
	if strings.Contains(rawHref, "..") {
		return "", "", fmt.Errorf("console navigation route must not contain parent path segments: %s", rawHref)
	}

	parsed, err := url.Parse(rawHref)
	if err != nil {
		return "", "", fmt.Errorf("invalid console navigation route %q: %w", rawHref, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || strings.HasPrefix(rawHref, "//") {
		return "", "", fmt.Errorf("console navigation only supports internal /console routes")
	}

	path := strings.TrimSpace(parsed.Path)
	if path == "" {
		return "", "", fmt.Errorf("console navigation route path is required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}

	if label, ok := exactConsoleRoutes[path]; ok {
		return path, label, nil
	}
	for _, route := range dynamicConsoleRoutePatterns {
		if route.pattern.MatchString(path) {
			return path, route.label, nil
		}
	}
	return "", "", fmt.Errorf("console navigation route is not whitelisted: %s", path)
}

var _ tools.Tool = (*NavigateTool)(nil)
