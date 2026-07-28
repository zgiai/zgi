package consolenavigation

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	navigationEventType             = "page_navigation_requested"
	navigationStatus                = "navigation_requested"
	routeAuthorizerRuntimeParameter = "_console_navigation_route_authorizer"
)

// RouteAuthorizationRequest contains only server-owned route authorization inputs.
type RouteAuthorizationRequest struct {
	Href            string
	WorkspaceID     string
	PermissionCodes []workspacemodel.WorkspacePermissionCode
}

// RouteAuthorizer validates whether the current account may open a workspace route.
// PermissionCodes use any-of semantics. An empty list still requires a trusted
// workspace membership check.
type RouteAuthorizer func(context.Context, RouteAuthorizationRequest) error

type consoleRouteAccess struct {
	label             string
	requiresWorkspace bool
	permissionCodes   []workspacemodel.WorkspacePermissionCode
}

var agentPagePermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionAgentView,
	workspacemodel.WorkspacePermissionAgentCreate,
	workspacemodel.WorkspacePermissionAgentLogsView,
	workspacemodel.WorkspacePermissionAgentUpdate,
	workspacemodel.WorkspacePermissionAgentDelete,
	workspacemodel.WorkspacePermissionAgentMove,
	workspacemodel.WorkspacePermissionAgentPublish,
	workspacemodel.WorkspacePermissionAgentRuntimeAccessManage,
}

var agentEditorPermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionAgentCreate,
	workspacemodel.WorkspacePermissionAgentUpdate,
	workspacemodel.WorkspacePermissionAgentPublish,
	workspacemodel.WorkspacePermissionAgentRuntimeAccessManage,
}

var workflowPagePermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionWorkflowCreate,
	workspacemodel.WorkspacePermissionWorkflowImport,
	workspacemodel.WorkspacePermissionWorkflowView,
	workspacemodel.WorkspacePermissionWorkflowLogsView,
	workspacemodel.WorkspacePermissionWorkflowUpdate,
	workspacemodel.WorkspacePermissionWorkflowDelete,
	workspacemodel.WorkspacePermissionWorkflowMove,
	workspacemodel.WorkspacePermissionWorkflowRunDraft,
	workspacemodel.WorkspacePermissionWorkflowPublish,
	workspacemodel.WorkspacePermissionWorkflowRuntimeAccessManage,
}

var workflowEditorPermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionWorkflowCreate,
	workspacemodel.WorkspacePermissionWorkflowImport,
	workspacemodel.WorkspacePermissionWorkflowUpdate,
	workspacemodel.WorkspacePermissionWorkflowRunDraft,
	workspacemodel.WorkspacePermissionWorkflowPublish,
	workspacemodel.WorkspacePermissionWorkflowRuntimeAccessManage,
}

var agentAssetEditorPermissions = append(
	slices.Clone(agentEditorPermissions),
	workflowEditorPermissions...,
)

var knowledgeBasePagePermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionKnowledgeBaseView,
	workspacemodel.WorkspacePermissionKnowledgeBaseCreate,
	workspacemodel.WorkspacePermissionKnowledgeBaseFolderManage,
	workspacemodel.WorkspacePermissionKnowledgeBaseRetrievalTest,
	workspacemodel.WorkspacePermissionKnowledgeBaseDocumentView,
	workspacemodel.WorkspacePermissionKnowledgeBaseGraphView,
	workspacemodel.WorkspacePermissionKnowledgeBaseUpdate,
	workspacemodel.WorkspacePermissionKnowledgeBaseDelete,
	workspacemodel.WorkspacePermissionKnowledgeBaseMove,
	workspacemodel.WorkspacePermissionKnowledgeBaseDocumentCreate,
	workspacemodel.WorkspacePermissionKnowledgeBaseDocumentUpdate,
	workspacemodel.WorkspacePermissionKnowledgeBaseDocumentDelete,
	workspacemodel.WorkspacePermissionKnowledgeBaseSegmentUpdate,
	workspacemodel.WorkspacePermissionKnowledgeBaseSegmentDelete,
	workspacemodel.WorkspacePermissionKnowledgeBaseIndexManage,
	workspacemodel.WorkspacePermissionKnowledgeBaseGraphManage,
}

var databasePagePermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionDatabaseView,
	workspacemodel.WorkspacePermissionDatabaseCreate,
	workspacemodel.WorkspacePermissionDatabaseUpdate,
	workspacemodel.WorkspacePermissionDatabaseDelete,
	workspacemodel.WorkspacePermissionDatabaseMove,
	workspacemodel.WorkspacePermissionDatabaseSchemaView,
	workspacemodel.WorkspacePermissionDatabaseSchemaManage,
	workspacemodel.WorkspacePermissionDatabaseRecordView,
	workspacemodel.WorkspacePermissionDatabaseRecordCreate,
	workspacemodel.WorkspacePermissionDatabaseRecordUpdate,
	workspacemodel.WorkspacePermissionDatabaseRecordDelete,
	workspacemodel.WorkspacePermissionDatabaseImportAnalyze,
	workspacemodel.WorkspacePermissionDatabaseImportExecute,
	workspacemodel.WorkspacePermissionDatabaseOperationLogsView,
	workspacemodel.WorkspacePermissionDatabaseSQLAuditView,
	workspacemodel.WorkspacePermissionDatabaseAIQueryRead,
}

var databaseRecordPermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionDatabaseRecordView,
	workspacemodel.WorkspacePermissionDatabaseRecordCreate,
	workspacemodel.WorkspacePermissionDatabaseRecordUpdate,
	workspacemodel.WorkspacePermissionDatabaseRecordDelete,
}

var databaseTablePermissions = []workspacemodel.WorkspacePermissionCode{
	workspacemodel.WorkspacePermissionDatabaseSchemaView,
	workspacemodel.WorkspacePermissionDatabaseSchemaManage,
	workspacemodel.WorkspacePermissionDatabaseRecordView,
	workspacemodel.WorkspacePermissionDatabaseRecordCreate,
	workspacemodel.WorkspacePermissionDatabaseRecordUpdate,
	workspacemodel.WorkspacePermissionDatabaseRecordDelete,
}

var exactConsoleRoutes = map[string]consoleRouteAccess{
	"/console":                         {label: "Home"},
	"/console/work/chat":               {label: "Conversations"},
	"/console/work/image":              {label: "Images"},
	"/console/work/app":                {label: "Apps"},
	"/console/work/task":               {label: "Scheduled Tasks", requiresWorkspace: true},
	"/console/agents":                  {label: "Agents", requiresWorkspace: true, permissionCodes: agentPagePermissions},
	"/console/workflows":               {label: "Workflows", requiresWorkspace: true, permissionCodes: workflowPagePermissions},
	"/console/dataset":                 {label: "Knowledge Bases", requiresWorkspace: true, permissionCodes: knowledgeBasePagePermissions},
	"/console/db":                      {label: "Databases", requiresWorkspace: true, permissionCodes: databasePagePermissions},
	"/console/files":                   {label: "Files", requiresWorkspace: true},
	"/console/skills":                  {label: "Skills"},
	"/console/prompts":                 {label: "Prompts", requiresWorkspace: true},
	"/console/developer/content-parse": {label: "File Recognition", requiresWorkspace: true},
	"/console/workspace":               {label: "Workspace", requiresWorkspace: true},
	"/console/workspace/members": {
		label:             "Workspace Members",
		requiresWorkspace: true,
		permissionCodes: []workspacemodel.WorkspacePermissionCode{
			workspacemodel.WorkspacePermissionWorkspaceMemberManage,
			workspacemodel.WorkspacePermissionWorkspacePermissionManage,
		},
	},
	"/console/workspace/settings": {label: "Workspace Settings", requiresWorkspace: true},
}

var dynamicConsoleRoutePatterns = []struct {
	pattern *regexp.Regexp
	access  consoleRouteAccess
}{
	{regexp.MustCompile(`^/console/agents/[A-Za-z0-9_-]+/logs$`), consoleRouteAccess{label: "Agent Logs", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionAgentLogsView, workspacemodel.WorkspacePermissionWorkflowLogsView}}},
	{regexp.MustCompile(`^/console/agents/[A-Za-z0-9_-]+/api$`), consoleRouteAccess{label: "Agent API", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionAgentRuntimeAccessManage, workspacemodel.WorkspacePermissionWorkflowRuntimeAccessManage}}},
	{regexp.MustCompile(`^/console/agents/[A-Za-z0-9_-]+/batch-test$`), consoleRouteAccess{label: "Agent Batch Test", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionWorkflowView}}},
	{regexp.MustCompile(`^/console/agents/[A-Za-z0-9_-]+/agent$`), consoleRouteAccess{label: "Agent Detail", requiresWorkspace: true, permissionCodes: agentEditorPermissions}},
	{regexp.MustCompile(`^/console/agents/[A-Za-z0-9_-]+/workflow$`), consoleRouteAccess{label: "Workflow Detail", requiresWorkspace: true, permissionCodes: workflowEditorPermissions}},
	{regexp.MustCompile(`^/console/workflows/[A-Za-z0-9_-]+/logs$`), consoleRouteAccess{label: "Workflow Logs", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionWorkflowLogsView}}},
	{regexp.MustCompile(`^/console/workflows/[A-Za-z0-9_-]+/api$`), consoleRouteAccess{label: "Workflow API", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionWorkflowRuntimeAccessManage}}},
	{regexp.MustCompile(`^/console/workflows/[A-Za-z0-9_-]+/batch-test$`), consoleRouteAccess{label: "Workflow Batch Test", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionWorkflowView}}},
	{regexp.MustCompile(`^/console/workflows/[A-Za-z0-9_-]+$`), consoleRouteAccess{label: "Workflow Detail", requiresWorkspace: true, permissionCodes: workflowEditorPermissions}},
	{regexp.MustCompile(`^/console/dataset/[A-Za-z0-9_-]+(/(documents|graph|hit-testing|batch-testing|settings))?$`), consoleRouteAccess{label: "Knowledge Base Detail", requiresWorkspace: true, permissionCodes: knowledgeBasePagePermissions}},
	{regexp.MustCompile(`^/console/db/[A-Za-z0-9_-]+/record$`), consoleRouteAccess{label: "Database Records", requiresWorkspace: true, permissionCodes: databaseRecordPermissions}},
	{regexp.MustCompile(`^/console/db/[A-Za-z0-9_-]+/search$`), consoleRouteAccess{label: "Database Search", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionDatabaseAIQueryRead}}},
	{regexp.MustCompile(`^/console/db/[A-Za-z0-9_-]+/import-excel$`), consoleRouteAccess{label: "Database Import", requiresWorkspace: true, permissionCodes: []workspacemodel.WorkspacePermissionCode{workspacemodel.WorkspacePermissionDatabaseImportAnalyze, workspacemodel.WorkspacePermissionDatabaseImportExecute}}},
	{regexp.MustCompile(`^/console/db/[A-Za-z0-9_-]+/table/[A-Za-z0-9_-]+$`), consoleRouteAccess{label: "Database Table", requiresWorkspace: true, permissionCodes: databaseTablePermissions}},
	{regexp.MustCompile(`^/console/db/[A-Za-z0-9_-]+$`), consoleRouteAccess{label: "Database Detail", requiresWorkspace: true, permissionCodes: databasePagePermissions}},
	{regexp.MustCompile(`^/console/prompts/[A-Za-z0-9_-]+$`), consoleRouteAccess{label: "Prompt Detail", requiresWorkspace: true}},
	{regexp.MustCompile(`^/console/work/app/[A-Za-z0-9_-]+$`), consoleRouteAccess{label: "App Detail"}},
}

var bareAgentDetailRoutePattern = regexp.MustCompile(`^/console/agents/[A-Za-z0-9_-]+$`)

// WithRouteAuthorizer returns a shallow copy of runtimeParameters containing a
// server-owned route authorizer. The callback must never be sourced from client
// request data.
func WithRouteAuthorizer(
	runtimeParameters map[string]interface{},
	authorizer RouteAuthorizer,
) map[string]interface{} {
	cloned := maps.Clone(runtimeParameters)
	if cloned == nil {
		cloned = make(map[string]interface{})
	}
	if authorizer == nil {
		delete(cloned, routeAuthorizerRuntimeParameter)
		return cloned
	}
	cloned[routeAuthorizerRuntimeParameter] = authorizer
	return cloned
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
				LLMDescription: "Required whitelisted internal route. Prefer exact site-map routes such as /console/files, /console/agents, /console/workflows, /console/dataset, /console/db, /console/skills, /console/prompts, /console/work/task, /console/work/chat, /console/work/image, /console/work/app, or /console/workspace.",
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

	normalizedHref, access, err := resolveConsoleRoute(href)
	if err != nil {
		return nil, err
	}
	if err := authorizeConsoleRoute(ctx, runtime, normalizedHref, access); err != nil {
		return nil, err
	}

	reason, _ := stringParam(toolParameters, "reason")
	result := map[string]interface{}{
		"status":     navigationStatus,
		"event_type": navigationEventType,
		"href":       normalizedHref,
		"label":      access.label,
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
	href, access, err := resolveConsoleRoute(rawHref)
	if err != nil {
		return "", "", err
	}
	return href, access.label, nil
}

func resolveConsoleRoute(rawHref string) (string, consoleRouteAccess, error) {
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return "", consoleRouteAccess{}, fmt.Errorf("href is required")
	}
	if strings.Contains(rawHref, "..") {
		return "", consoleRouteAccess{}, fmt.Errorf("console navigation route must not contain parent path segments: %s", rawHref)
	}

	parsed, err := url.Parse(rawHref)
	if err != nil {
		return "", consoleRouteAccess{}, fmt.Errorf("invalid console navigation route %q: %w", rawHref, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || strings.HasPrefix(rawHref, "//") {
		return "", consoleRouteAccess{}, fmt.Errorf("console navigation only supports internal /console routes")
	}

	path := strings.TrimSpace(parsed.Path)
	if path == "" {
		return "", consoleRouteAccess{}, fmt.Errorf("console navigation route path is required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}

	if access, ok := exactConsoleRoutes[path]; ok {
		return path, access, nil
	}
	if bareAgentDetailRoutePattern.MatchString(path) {
		return path + "/agent", consoleRouteAccess{
			label:             "Agent Detail",
			requiresWorkspace: true,
			permissionCodes:   agentAssetEditorPermissions,
		}, nil
	}
	for _, route := range dynamicConsoleRoutePatterns {
		if route.pattern.MatchString(path) {
			return path, route.access, nil
		}
	}
	return "", consoleRouteAccess{}, fmt.Errorf("console navigation route is not whitelisted: %s", path)
}

func authorizeConsoleRoute(
	ctx context.Context,
	runtime *tools.ToolRuntime,
	href string,
	access consoleRouteAccess,
) error {
	if !access.requiresWorkspace {
		return nil
	}
	if runtime == nil || runtime.RuntimeParameters == nil {
		return fmt.Errorf("console navigation permission denied: trusted workspace context is unavailable")
	}
	workspaceID := strings.TrimSpace(fmt.Sprint(runtime.RuntimeParameters["workspace_id"]))
	if workspaceID == "" {
		return fmt.Errorf("console navigation permission denied: select a workspace before opening %s", href)
	}
	authorizer, ok := runtime.RuntimeParameters[routeAuthorizerRuntimeParameter].(RouteAuthorizer)
	if !ok || authorizer == nil {
		return fmt.Errorf("console navigation permission denied: route authorizer is unavailable")
	}
	if err := authorizer(ctx, RouteAuthorizationRequest{
		Href:            href,
		WorkspaceID:     workspaceID,
		PermissionCodes: slices.Clone(access.permissionCodes),
	}); err != nil {
		return fmt.Errorf("console navigation permission denied for %s: %w", href, err)
	}
	return nil
}

var _ tools.Tool = (*NavigateTool)(nil)
