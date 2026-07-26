package suggestedquestions

import (
	"strings"
)

const maxConversationRoutes = 8

type workflowGraphNode struct {
	ID       string
	Type     string
	Title    string
	ParentID string
	Data     map[string]interface{}
}

type workflowGraphEdge struct {
	Source       string
	Target       string
	SourceHandle string
}

type workflowGraphAnalysis struct {
	Nodes       []workflowGraphNode
	NodeByID    map[string]workflowGraphNode
	Outgoing    map[string][]workflowGraphEdge
	Incoming    map[string][]workflowGraphEdge
	Effective   map[string]struct{}
	GraphStable bool
}

func normalizeWorkflowType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "chat", "advanced_chat", "conversation_workflow", "conversational_workflow":
		return "conversational_workflow"
	case "workflow", "task_workflow":
		return "workflow"
	case "agent":
		return "agent"
	default:
		return normalized
	}
}

func isConversationalWorkflow(workflowType string) bool {
	return normalizeWorkflowType(workflowType) == "conversational_workflow"
}

func analyzeWorkflowGraph(graph map[string]interface{}) workflowGraphAnalysis {
	analysis := workflowGraphAnalysis{
		NodeByID:  make(map[string]workflowGraphNode),
		Outgoing:  make(map[string][]workflowGraphEdge),
		Incoming:  make(map[string][]workflowGraphEdge),
		Effective: make(map[string]struct{}),
	}

	for _, item := range sliceValue(graph["nodes"]) {
		nodeMap := mapValue(item)
		if nodeMap == nil {
			continue
		}
		data := mapValue(nodeMap["data"])
		if data == nil {
			data = map[string]interface{}{}
		}
		id := strings.TrimSpace(stringValue(nodeMap["id"]))
		nodeType := normalizeNodeType(firstString(data["type"], nodeMap["type"]))
		if id == "" || nodeType == "" {
			continue
		}
		node := workflowGraphNode{
			ID:       id,
			Type:     nodeType,
			Title:    firstString(data["title"], data["label"]),
			ParentID: firstString(nodeMap["parentId"], nodeMap["parent_id"], data["iteration_id"], data["loop_id"]),
			Data:     data,
		}
		analysis.Nodes = append(analysis.Nodes, node)
		analysis.NodeByID[id] = node
	}

	for _, item := range sliceValue(graph["edges"]) {
		edgeMap := mapValue(item)
		if edgeMap == nil {
			continue
		}
		edge := workflowGraphEdge{
			Source:       firstString(edgeMap["source"]),
			Target:       firstString(edgeMap["target"]),
			SourceHandle: firstString(edgeMap["sourceHandle"], edgeMap["source_handle"]),
		}
		if edge.Source == "" || edge.Target == "" {
			continue
		}
		if _, ok := analysis.NodeByID[edge.Source]; !ok {
			continue
		}
		if _, ok := analysis.NodeByID[edge.Target]; !ok {
			continue
		}
		analysis.Outgoing[edge.Source] = append(analysis.Outgoing[edge.Source], edge)
		analysis.Incoming[edge.Target] = append(analysis.Incoming[edge.Target], edge)
	}

	starts := make([]string, 0, 1)
	terminals := make([]string, 0, 2)
	for _, node := range analysis.Nodes {
		switch node.Type {
		case "start":
			starts = append(starts, node.ID)
		case "answer", "end":
			terminals = append(terminals, node.ID)
		}
	}

	hasEdges := len(analysis.Outgoing) > 0
	analysis.GraphStable = hasEdges && len(starts) > 0 && len(terminals) > 0
	if !analysis.GraphStable {
		// Old drafts and small unit fixtures may not contain complete edge data.
		// Preserve useful context, but never conclude that their query is unused.
		for _, node := range analysis.Nodes {
			analysis.Effective[node.ID] = struct{}{}
		}
		return analysis
	}

	reachable := walkGraph(starts, analysis.Outgoing)
	canReachTerminal := walkGraph(terminals, reverseEdges(analysis.Incoming))
	for nodeID := range reachable {
		if _, ok := canReachTerminal[nodeID]; ok {
			analysis.Effective[nodeID] = struct{}{}
		}
	}

	// Container children are part of an effective container even though their
	// internal dependency graph does not always connect directly to the outer
	// start/end edges.
	changed := true
	for changed {
		changed = false
		for _, node := range analysis.Nodes {
			if node.ParentID == "" {
				continue
			}
			if _, parentEffective := analysis.Effective[node.ParentID]; !parentEffective {
				continue
			}
			if _, alreadyEffective := analysis.Effective[node.ID]; alreadyEffective {
				continue
			}
			analysis.Effective[node.ID] = struct{}{}
			changed = true
		}
	}

	return analysis
}

func normalizeNodeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.ReplaceAll(value, "_", "-")
}

func walkGraph(starts []string, outgoing map[string][]workflowGraphEdge) map[string]struct{} {
	visited := make(map[string]struct{}, len(starts))
	queue := append([]string(nil), starts...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		for _, edge := range outgoing[current] {
			queue = append(queue, edge.Target)
		}
	}
	return visited
}

func reverseEdges(incoming map[string][]workflowGraphEdge) map[string][]workflowGraphEdge {
	reversed := make(map[string][]workflowGraphEdge, len(incoming))
	for target, edges := range incoming {
		for _, edge := range edges {
			reversed[target] = append(reversed[target], workflowGraphEdge{Target: edge.Source})
		}
	}
	return reversed
}

func (analysis workflowGraphAnalysis) isEffective(nodeID string) bool {
	_, ok := analysis.Effective[nodeID]
	return ok
}

func analyzeConversation(analysis workflowGraphAnalysis) ConversationSummary {
	summary := ConversationSummary{QueryRole: QueryRoleUnknown}
	if !analysis.GraphStable {
		return summary
	}

	directQueryNodes := make(map[string]struct{})
	dependentNodes := make(map[string]struct{})
	for _, node := range analysis.Nodes {
		if !analysis.isEffective(node.ID) || node.Type == "start" {
			continue
		}
		if containsImplicitQueryReference(node.Data) {
			directQueryNodes[node.ID] = struct{}{}
			dependentNodes[node.ID] = struct{}{}
		}
	}

	if len(directQueryNodes) == 0 {
		summary.QueryRole = QueryRoleUnused
		return summary
	}

	// Propagate query dependence through variable selectors so an extractor or
	// classifier feeding a branch is recognized even when the branch does not
	// directly mention sys.query.
	changed := true
	for changed {
		changed = false
		for _, node := range analysis.Nodes {
			if !analysis.isEffective(node.ID) {
				continue
			}
			if _, ok := dependentNodes[node.ID]; ok {
				continue
			}
			if referencesAnyNode(node.Data, dependentNodes) {
				dependentNodes[node.ID] = struct{}{}
				changed = true
			}
		}
	}

	var routeUse, extractionUse, contentUse bool
	for nodeID := range directQueryNodes {
		node := analysis.NodeByID[nodeID]
		switch node.Type {
		case "question-classifier", "if-else":
			routeUse = true
		case "parameter-extractor":
			extractionUse = true
			summary.RequiredContext = append(summary.RequiredContext, extractParameterContext(node.Data)...)
		default:
			if hasDependentBranchDescendant(analysis, node.ID, dependentNodes) {
				routeUse = true
			} else {
				contentUse = true
			}
		}
	}

	useKinds := 0
	for _, used := range []bool{routeUse, extractionUse, contentUse} {
		if used {
			useKinds++
		}
	}
	switch {
	case useKinds > 1:
		summary.QueryRole = QueryRoleMixed
	case routeUse:
		summary.QueryRole = QueryRoleRouteSelector
	case extractionUse:
		summary.QueryRole = QueryRoleExtractionSource
	case contentUse:
		summary.QueryRole = QueryRoleContentInput
	default:
		summary.QueryRole = QueryRoleUnknown
	}

	summary.RequiredContext = uniqueVariables(summary.RequiredContext, 12)
	summary.Routes = extractConversationRoutes(analysis, dependentNodes)
	return summary
}

func containsImplicitQueryReference(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(typed), " ", ""))
		return strings.Contains(normalized, "sys.query") || strings.Contains(normalized, "#sys.query#")
	case []interface{}:
		if len(typed) >= 2 && strings.EqualFold(stringValue(typed[0]), "sys") && strings.EqualFold(stringValue(typed[1]), "query") {
			return true
		}
		for _, item := range typed {
			if containsImplicitQueryReference(item) {
				return true
			}
		}
	case map[string]interface{}:
		for key, item := range typed {
			if isSensitiveKey(key) {
				continue
			}
			if containsImplicitQueryReference(item) {
				return true
			}
		}
	}
	return false
}

func referencesAnyNode(value interface{}, nodeIDs map[string]struct{}) bool {
	switch typed := value.(type) {
	case string:
		for nodeID := range nodeIDs {
			if strings.Contains(typed, "#"+nodeID+".") || strings.Contains(typed, "#"+nodeID+"#") {
				return true
			}
		}
	case []interface{}:
		if len(typed) >= 2 {
			if _, ok := nodeIDs[stringValue(typed[0])]; ok {
				return true
			}
		}
		for _, item := range typed {
			if referencesAnyNode(item, nodeIDs) {
				return true
			}
		}
	case map[string]interface{}:
		for key, item := range typed {
			if isSensitiveKey(key) {
				continue
			}
			if referencesAnyNode(item, nodeIDs) {
				return true
			}
		}
	}
	return false
}

func hasDependentBranchDescendant(analysis workflowGraphAnalysis, start string, dependent map[string]struct{}) bool {
	visited := map[string]struct{}{start: {}}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range analysis.Outgoing[current] {
			if _, ok := visited[edge.Target]; ok {
				continue
			}
			visited[edge.Target] = struct{}{}
			node := analysis.NodeByID[edge.Target]
			if _, queryDependent := dependent[node.ID]; queryDependent && (node.Type == "if-else" || node.Type == "question-classifier") {
				return true
			}
			queue = append(queue, edge.Target)
		}
	}
	return false
}

func extractParameterContext(data map[string]interface{}) []VariableSummary {
	items := sliceValue(data["parameters"])
	result := make([]VariableSummary, 0, len(items))
	for _, item := range items {
		parameter := mapValue(item)
		if parameter == nil {
			continue
		}
		name := firstString(parameter["name"], parameter["variable"], parameter["label"])
		if name == "" || isSensitiveKey(name) {
			continue
		}
		result = append(result, VariableSummary{
			Name:        name,
			Type:        firstString(parameter["type"], parameter["value_type"]),
			Description: cleanText(firstString(parameter["description"], parameter["desc"]), 220),
			Required:    boolValue(parameter["required"]),
		})
	}
	return result
}

func uniqueVariables(values []VariableSummary, limit int) []VariableSummary {
	seen := make(map[string]struct{}, len(values))
	result := make([]VariableSummary, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value.Name))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func extractConversationRoutes(analysis workflowGraphAnalysis, dependent map[string]struct{}) []ConversationRouteSummary {
	routes := make([]ConversationRouteSummary, 0, 4)
	for _, node := range analysis.Nodes {
		if len(routes) >= maxConversationRoutes {
			break
		}
		if !analysis.isEffective(node.ID) {
			continue
		}
		if _, ok := dependent[node.ID]; !ok {
			continue
		}
		if node.Type != "if-else" && node.Type != "question-classifier" {
			continue
		}

		branchNames := branchNamesByHandle(node.Data)
		caseIntents := caseIntentsByHandle(node.Data)
		for _, edge := range analysis.Outgoing[node.ID] {
			if !analysis.isEffective(edge.Target) {
				continue
			}
			intent := firstMeaningfulRouteText(caseIntents[edge.SourceHandle], branchNames[edge.SourceHandle])
			capabilities := downstreamCapabilityTitles(analysis, edge.Target, 4)
			if intent == "" && len(capabilities) == 0 {
				continue
			}
			description := strings.Join(capabilities, ", ")
			if intent == "" {
				intent = description
			}
			routes = append(routes, ConversationRouteSummary{
				Intent:       cleanText(intent, 180),
				Description:  cleanText(description, 240),
				Capabilities: capabilities,
			})
			if len(routes) >= maxConversationRoutes {
				break
			}
		}
	}
	return uniqueRoutes(routes, maxConversationRoutes)
}

func branchNamesByHandle(data map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for _, item := range sliceValue(data["targetBranches"]) {
		branch := mapValue(item)
		if branch == nil {
			continue
		}
		id := firstString(branch["id"], branch["case_id"])
		if id != "" {
			result[id] = firstString(branch["name"], branch["title"], branch["label"], branch["description"])
		}
	}
	return result
}

func caseIntentsByHandle(data map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for _, item := range sliceValue(data["cases"]) {
		caseData := mapValue(item)
		if caseData == nil {
			continue
		}
		handle := firstString(caseData["case_id"], caseData["id"])
		if handle == "" {
			continue
		}
		intent := firstString(caseData["name"], caseData["title"], caseData["label"], caseData["description"])
		if intent == "" {
			intent = conditionValueSummary(caseData["conditions"])
		}
		result[handle] = intent
	}
	for _, item := range sliceValue(data["classes"]) {
		classData := mapValue(item)
		if classData == nil {
			continue
		}
		handle := firstString(classData["id"], classData["class_id"], classData["case_id"])
		if handle != "" {
			result[handle] = firstString(classData["name"], classData["title"], classData["label"], classData["description"])
		}
	}
	return result
}

func conditionValueSummary(value interface{}) string {
	values := make([]string, 0, 3)
	var visit func(interface{})
	visit = func(current interface{}) {
		if len(values) >= 3 {
			return
		}
		switch typed := current.(type) {
		case map[string]interface{}:
			if valueText := firstString(typed["value"], typed["expected_value"], typed["label"]); valueText != "" {
				values = append(values, valueText)
			}
			for _, item := range typed {
				visit(item)
			}
		case []interface{}:
			for _, item := range typed {
				visit(item)
			}
		}
	}
	visit(value)
	return strings.Join(uniqueTrimmed(values, 3), ", ")
}

func firstMeaningfulRouteText(values ...string) string {
	for _, value := range values {
		value = cleanShortText(value)
		if value == "" {
			continue
		}
		normalized := strings.ToLower(strings.ReplaceAll(value, " ", ""))
		if normalized == "if" || normalized == "else" || strings.HasPrefix(normalized, "case") || strings.HasPrefix(normalized, "分支") {
			continue
		}
		return value
	}
	return ""
}

func downstreamCapabilityTitles(analysis workflowGraphAnalysis, start string, limit int) []string {
	visited := make(map[string]struct{})
	queue := []string{start}
	titles := make([]string, 0, limit)
	for len(queue) > 0 && len(titles) < limit {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		node, ok := analysis.NodeByID[current]
		if !ok || !analysis.isEffective(current) {
			continue
		}
		if isPublicCapabilityType(node.Type) && node.Title != "" {
			titles = append(titles, node.Title)
		}
		if node.Type == "answer" || node.Type == "end" || node.Type == "if-else" || node.Type == "question-classifier" {
			continue
		}
		for _, edge := range analysis.Outgoing[current] {
			queue = append(queue, edge.Target)
		}
	}
	return uniqueTrimmed(titles, limit)
}

func isPublicCapabilityType(nodeType string) bool {
	switch nodeType {
	case "start", "end", "answer", "note", "if-else", "question-classifier", "iteration-start", "loop-start", "loop-end":
		return false
	default:
		return true
	}
}

func uniqueRoutes(routes []ConversationRouteSummary, limit int) []ConversationRouteSummary {
	seen := make(map[string]struct{}, len(routes))
	result := make([]ConversationRouteSummary, 0, len(routes))
	for _, route := range routes {
		key := strings.ToLower(strings.TrimSpace(route.Intent + "|" + strings.Join(route.Capabilities, ",")))
		if key == "|" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, route)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}
