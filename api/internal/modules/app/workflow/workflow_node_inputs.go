package workflow

import (
	"fmt"
	"strings"

	graph_entities "github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/pkg/logger"
)

// getNodeInputs returns the appropriate inputs for a node based on its type and configuration
func (h *WorkflowHandler) getNodeInputs(nodeID, nodeType string, nodeData map[string]interface{}, systemInputs map[string]interface{}, userInputs map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	nodeInputs := make(map[string]interface{})

	// Debug: Print variable pool state
	if variablePool != nil {
		logger.Info("Node variable pool state", "nodeID", nodeID, "nodeType", nodeType)
		// Print all variables in the pool
		for nodeIDKey, nodeVars := range variablePool.VariableDictionary {
			logger.Info("Node variables", "nodeID", nodeIDKey, "variables", nodeVars)
		}
	}

	switch nodeType {
	case "start":
		// Start node gets user inputs + system variables
		// This is the entry point, so it gets all user inputs and system variables
		for k, v := range systemInputs {
			nodeInputs[k] = v
		}
		for k, v := range userInputs {
			nodeInputs[k] = v
		}
		logger.Debug("start node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "llm":
		nodeInputs = buildLLMNodeFrontendInputs(nodeData, variablePool)
		logger.Debug("LLM node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "if-else":
		for k, v := range systemInputs {
			nodeInputs[k] = v
		}
		if variablePool != nil {
			for prevNodeID, nodeVars := range variablePool.VariableDictionary {
				if prevNodeID != nodeID {
					for varKey, varValue := range nodeVars {
						nodeInputs[varKey] = varValue.ToObject()
					}
				}
			}
		}
		if cases, exists := nodeData["cases"]; exists {
			nodeInputs["cases"] = cases
		}
		if conditions, exists := nodeData["conditions"]; exists {
			nodeInputs["conditions"] = conditions
		}
		if logicalOperator, exists := nodeData["logical_operator"]; exists {
			nodeInputs["logical_operator"] = logicalOperator
		}

	case "end":
		// End node gets variables based on its output configuration
		// Get output variables from previous nodes based on end node configuration
		if outputsData, exists := nodeData["outputs"]; exists {
			if outputsList, ok := outputsData.([]interface{}); ok {
				for _, outputData := range outputsList {
					if outputMap, ok := outputData.(map[string]interface{}); ok {
						if valueSelector, ok := outputMap["value_selector"].([]interface{}); ok {
							if variable, ok := outputMap["variable"].(string); ok {
								// Convert []interface{} to []string
								selector := make([]string, len(valueSelector))
								for i, v := range valueSelector {
									if s, ok := v.(string); ok {
										selector[i] = s
									}
								}

								// Get variable from variable pool
								if variablePool != nil {
									if varValue := variablePool.GetWithPath(selector); varValue != nil {
										nodeInputs[variable] = varValue.ToObject()
										logger.Debug("end node got output variable from pool",
											"node_id", nodeID,
											"variable", variable,
											"selector_length", len(selector),
										)
									} else {
										logger.Debug("end node output variable not found in pool",
											"node_id", nodeID,
											"variable", variable,
											"selector_length", len(selector),
										)
									}
								}
							}
						}
					}
				}
			}
		} else {
			// If no specific output configuration, get all available variables from previous nodes
			if variablePool != nil {
				for prevNodeID, nodeVars := range variablePool.VariableDictionary {
					if prevNodeID != nodeID { // Don't include self
						for varKey, varValue := range nodeVars {
							nodeInputs[varKey] = varValue.ToObject()
						}
					}
				}
			}
		}

		logger.Debug("end node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "parameter-extractor":
		// Parameter extractor only needs query and optional files.
		// Do NOT dump system variables or previous node variables.

		// 1. Resolve query variable selector
		if querySelector, exists := nodeData["query"]; exists {
			if selectorList, ok := querySelector.([]interface{}); ok {
				selector := make([]string, len(selectorList))
				for i, v := range selectorList {
					if s, ok := v.(string); ok {
						selector[i] = s
					}
				}
				if len(selector) >= 2 && variablePool != nil {
					if varValue := variablePool.GetWithPath(selector); varValue != nil {
						nodeInputs[selector[1]] = varValue.ToObject()
					}
				}
			}
		}

		// 2. Resolve files if vision is enabled
		if visionData, exists := nodeData["vision"]; exists {
			if visionMap, ok := visionData.(map[string]interface{}); ok {
				if enabled, ok := visionMap["enabled"].(bool); ok && enabled {
					if configs, ok := visionMap["configs"].(map[string]interface{}); ok {
						if varSelector, ok := configs["variable_selector"].([]interface{}); ok {
							selector := make([]string, len(varSelector))
							for i, v := range varSelector {
								if s, ok := v.(string); ok {
									selector[i] = s
								}
							}
							if len(selector) >= 2 && variablePool != nil {
								if varValue := variablePool.GetWithPath(selector); varValue != nil {
									nodeInputs[selector[1]] = varValue.ToObject()
								}
							}
						}
					}
				}
			}
		}

		logger.Debug("parameter-extractor node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "knowledge-retrieval":
		// Knowledge retrieval only needs the query variable from the pool.
		// Do NOT dump system variables or previous node variables.

		if querySelector, exists := nodeData["query_variable_selector"]; exists {
			if selectorList, ok := querySelector.([]interface{}); ok {
				selector := make([]string, len(selectorList))
				for i, v := range selectorList {
					if s, ok := v.(string); ok {
						selector[i] = s
					}
				}
				if len(selector) >= 2 && variablePool != nil {
					if varValue := variablePool.GetWithPath(selector); varValue != nil {
						nodeInputs[selector[1]] = varValue.ToObject()
					}
				}
			}
		}

		logger.Debug("knowledge-retrieval node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "json-parser":
		// JSON parser only needs the input variable from the pool.
		// Do NOT dump system variables or previous node variables.

		// Primary: resolve from input_selector
		if inputSelector, exists := nodeData["input_selector"]; exists {
			if selectorList, ok := inputSelector.([]interface{}); ok {
				selector := make([]string, len(selectorList))
				for i, v := range selectorList {
					if s, ok := v.(string); ok {
						selector[i] = s
					}
				}
				if len(selector) >= 2 && variablePool != nil {
					if varValue := variablePool.GetWithPath(selector); varValue != nil {
						nodeInputs[selector[1]] = varValue.ToObject()
					}
				}
			}
		} else if variables, exists := nodeData["variables"]; exists {
			// Fallback: use first variable's value_selector and variable name
			if varsList, ok := variables.([]interface{}); ok && len(varsList) > 0 {
				if varMap, ok := varsList[0].(map[string]interface{}); ok {
					if valueSelector, ok := varMap["value_selector"].([]interface{}); ok {
						selector := make([]string, len(valueSelector))
						for i, v := range valueSelector {
							if s, ok := v.(string); ok {
								selector[i] = s
							}
						}
						key := "input"
						if varName, ok := varMap["variable"].(string); ok && varName != "" {
							key = varName
						}
						if len(selector) >= 2 && variablePool != nil {
							if varValue := variablePool.GetWithPath(selector); varValue != nil {
								nodeInputs[key] = varValue.ToObject()
							}
						}
					}
				}
			}
		}

		logger.Debug("json-parser node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "variable-aggregator":
		// Variable aggregator only needs the configured variable selectors.
		// Do NOT dump system variables or previous node variables.

		// Helper to convert selector to dot-joined string (matches executeRun)
		selectorToStr := func(s []string) string {
			if len(s) == 0 {
				return ""
			}
			result := s[0]
			for i := 1; i < len(s); i++ {
				result += "." + s[i]
			}
			return result
		}

		// Check if multi-group mode is enabled
		isGroupMode := false
		if advSettings, exists := nodeData["advanced_settings"]; exists {
			if advMap, ok := advSettings.(map[string]interface{}); ok {
				if enabled, ok := advMap["group_enabled"].(bool); ok && enabled {
					isGroupMode = true
					// Multi-group mode: iterate groups
					if groups, ok := advMap["groups"].([]interface{}); ok {
						for _, g := range groups {
							if groupMap, ok := g.(map[string]interface{}); ok {
								groupName, _ := groupMap["group_name"].(string)
								if groupVars, ok := groupMap["variables"].([]interface{}); ok {
									for _, v := range groupVars {
										if selList, ok := v.([]interface{}); ok {
											selector := make([]string, len(selList))
											for i, sv := range selList {
												if s, ok := sv.(string); ok {
													selector[i] = s
												}
											}
											if len(selector) >= 2 && variablePool != nil {
												if varValue := variablePool.GetWithPath(selector); varValue != nil {
													key := selectorToStr(selector)
													if groupName != "" {
														key = groupName + "." + key
													}
													nodeInputs[key] = varValue.ToObject()
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// Single-group mode
		if !isGroupMode {
			if variables, exists := nodeData["variables"]; exists {
				if varsList, ok := variables.([]interface{}); ok {
					for _, v := range varsList {
						if selList, ok := v.([]interface{}); ok {
							selector := make([]string, len(selList))
							for i, sv := range selList {
								if s, ok := sv.(string); ok {
									selector[i] = s
								}
							}
							if len(selector) >= 2 && variablePool != nil {
								if varValue := variablePool.GetWithPath(selector); varValue != nil {
									nodeInputs[selectorToStr(selector)] = varValue.ToObject()
								}
							}
						}
					}
				}
			}
		}

		logger.Debug("variable-aggregator node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "code":
		// Code node only needs its configured input variables.
		// Do NOT dump system variables or previous node variables.

		if variables, exists := nodeData["variables"]; exists {
			if varsList, ok := variables.([]interface{}); ok {
				for _, v := range varsList {
					if varMap, ok := v.(map[string]interface{}); ok {
						varName, _ := varMap["variable"].(string)
						if varName == "" {
							continue
						}
						if valueSelector, ok := varMap["value_selector"].([]interface{}); ok {
							selector := make([]string, len(valueSelector))
							for i, sv := range valueSelector {
								if s, ok := sv.(string); ok {
									selector[i] = s
								}
							}
							if len(selector) >= 2 && variablePool != nil {
								if varValue := variablePool.GetWithPath(selector); varValue != nil {
									nodeInputs[varName] = varValue.ToObject()
								}
							}
						}
					}
				}
			}
		}

		logger.Debug("code node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "http-request":
		nodeInputs = buildHTTPNodeFrontendInputs(nodeData)
		logger.Debug("http-request node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "call-database":
		nodeInputs = buildCallDatabaseNodeFrontendInputs(nodeData)
		logger.Debug("call-database node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "sql-generator":
		nodeInputs = buildSQLGeneratorNodeFrontendInputs(nodeData, variablePool)
		logger.Debug("sql-generator node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "image-gen":
		nodeInputs = buildImageGenNodeFrontendInputs(nodeData, variablePool)
		logger.Debug("image-gen node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "loop":
		nodeInputs = buildLoopNodeFrontendInputs(nodeData, variablePool)
		logger.Debug("loop node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "iteration":
		nodeInputs = buildIterationNodeFrontendInputs(nodeData, variablePool)
		logger.Debug("iteration node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "assigner":
		nodeInputs = buildVariableAssignerNodeFrontendInputs(nodeData)
		logger.Debug("assigner node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	case "answer":
		nodeInputs = buildAnswerNodeFrontendInputs(nodeData, variablePool)
		logger.Debug("answer node inputs prepared",
			"node_id", nodeID,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)

	default:
		// For other node types, get variables based on their specific configuration
		// Add system variables
		for k, v := range systemInputs {
			nodeInputs[k] = v
		}

		// Try to get variables from previous nodes
		if variablePool != nil {
			for prevNodeID, nodeVars := range variablePool.VariableDictionary {
				if prevNodeID != nodeID { // Don't include self
					for varKey, varValue := range nodeVars {
						nodeInputs[varKey] = varValue.ToObject()
					}
				}
			}
		}

		logger.Debug("node inputs prepared",
			"node_id", nodeID,
			"node_type", nodeType,
			"input_count", len(nodeInputs),
			"input_keys", stringMapKeys(nodeInputs),
		)
	}

	return nodeInputs
}

// extractVariablesFromTemplate extracts variable names from template text
func (h *WorkflowHandler) extractVariablesFromTemplate(template string) []string {
	var variables []string
	// Simple regex-like extraction for {{variable}} patterns
	// This is a basic implementation, in production you might want to use a proper template parser
	start := 0
	for {
		startIndex := strings.Index(template[start:], "{{")
		if startIndex == -1 {
			break
		}
		startIndex += start
		endIndex := strings.Index(template[startIndex:], "}}")
		if endIndex == -1 {
			break
		}
		endIndex += startIndex
		variable := strings.TrimSpace(template[startIndex+2 : endIndex])
		if variable != "" {
			variables = append(variables, variable)
		}
		start = endIndex + 2
	}
	return variables
}

func buildHTTPNodeFrontendInputs(nodeData map[string]interface{}) map[string]interface{} {
	inputs := map[string]interface{}{
		"url":    stringValue(firstExisting(nodeData, "url")),
		"method": strings.ToUpper(defaultString(stringValue(firstExisting(nodeData, "method")), "GET")),
		"header": httpKeyValueConfigToMap(firstExisting(nodeData, "header", "headers")),
		"param":  httpKeyValueConfigToMap(firstExisting(nodeData, "param", "params")),
		"body":   httpBodyConfig(firstExisting(nodeData, "body")),
		"auth":   httpAuthConfig(firstExisting(nodeData, "auth", "authorization")),
	}
	return inputs
}

func firstExisting(data map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, exists := data[key]; exists {
			return value
		}
		if nested, ok := data["data"].(map[string]interface{}); ok {
			if value, exists := nested[key]; exists {
				return value
			}
		}
	}
	return nil
}

func stringValue(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func httpKeyValueConfigToMap(value interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	switch typed := value.(type) {
	case string:
		for _, line := range strings.Split(typed, "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			if key == "" {
				continue
			}
			result[key] = strings.TrimSpace(parts[1])
		}
	case map[string]interface{}:
		for key, item := range typed {
			result[key] = item
		}
	case []interface{}:
		for _, item := range typed {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			key := stringValue(itemMap["key"])
			if key == "" {
				continue
			}
			result[key] = itemMap["value"]
		}
	}
	return result
}

func httpBodyConfig(value interface{}) interface{} {
	body, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	if bodyType := strings.TrimSpace(stringValue(body["type"])); bodyType == "" || bodyType == "none" {
		return nil
	}
	return body
}

func httpAuthConfig(value interface{}) interface{} {
	auth, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	authType := strings.TrimSpace(stringValue(auth["type"]))
	if authType == "" || authType == "no-auth" {
		return nil
	}
	result := map[string]interface{}{"type": authType}
	if config, ok := auth["config"].(map[string]interface{}); ok {
		configSnapshot := map[string]interface{}{}
		if configType := stringValue(config["type"]); configType != "" {
			configSnapshot["type"] = configType
		}
		if header := stringValue(config["header"]); header != "" {
			configSnapshot["header"] = header
		}
		if len(configSnapshot) > 0 {
			result["config"] = configSnapshot
		}
	}
	return result
}

func buildCallDatabaseNodeFrontendInputs(nodeData map[string]interface{}) map[string]interface{} {
	inputs := map[string]interface{}{}
	if sql := firstExisting(nodeData, "manual_sql", "sql", "query"); sql != nil {
		inputs["sql"] = sql
	}
	return inputs
}

func buildSQLGeneratorNodeFrontendInputs(nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	prompt := buildPromptSnapshot(firstExisting(nodeData, "prompt", "prompt_template", "prompt_templates"), nodeData, variablePool)
	inputs := map[string]interface{}{}
	if prompt != nil {
		inputs["prompt"] = prompt
	}
	return inputs
}

func buildImageGenNodeFrontendInputs(nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	inputs := map[string]interface{}{
		"prompt_variables": resolvePromptVariablesFromNodeData(nodeData, variablePool),
	}
	if prompt := stringValue(firstExisting(nodeData, "prompt", "prompt_template")); prompt != "" {
		inputs["prompt"] = renderConfiguredTemplateText(prompt, nodeData, variablePool)
	}
	return inputs
}

func buildLLMNodeFrontendInputs(nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	inputs := map[string]interface{}{}
	if prompt := buildPromptSnapshot(firstExisting(nodeData, "prompt_template", "prompt", "prompt_templates"), nodeData, variablePool); prompt != nil {
		inputs["prompt"] = prompt
	}
	if contextData := firstExisting(nodeData, "context"); contextData != nil {
		if contextMap, ok := contextData.(map[string]interface{}); ok {
			if enabled, ok := contextMap["enabled"].(bool); ok && enabled {
				resolveVariableSelectorsInto(inputs, contextMap["variable_selector"], variablePool)
			}
		}
	}
	return inputs
}

func buildLoopNodeFrontendInputs(nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	inputs := map[string]interface{}{}
	if loopCount := firstExisting(nodeData, "loop_count"); loopCount != nil {
		inputs["loop_count"] = loopCount
	}
	if loopVariables := firstExisting(nodeData, "loop_variables"); loopVariables != nil {
		inputs["loop_variables"] = resolveLoopVariables(loopVariables, variablePool)
	}
	if breakConditions := firstExisting(nodeData, "break_conditions"); breakConditions != nil {
		inputs["break_conditions"] = breakConditions
	}
	return inputs
}

func buildIterationNodeFrontendInputs(nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	inputs := map[string]interface{}{}
	iteratorSelector := interfaceToStringSlice(firstExisting(nodeData, "iterator_selector", "input_selector"))
	if len(iteratorSelector) > 0 {
		if value, ok := resolveSelectorValue(variablePool, iteratorSelector); ok {
			inputs[selectorDisplayKey(iteratorSelector)] = value
		}
	}
	outputSelector := interfaceToStringSlice(firstExisting(nodeData, "output_selector"))
	if len(outputSelector) > 0 {
		if value, ok := resolveSelectorValue(variablePool, outputSelector); ok {
			inputs[selectorDisplayKey(outputSelector)] = value
		}
	}
	return inputs
}

func buildVariableAssignerNodeFrontendInputs(nodeData map[string]interface{}) map[string]interface{} {
	inputs := map[string]interface{}{}
	if items := firstExisting(nodeData, "items"); items != nil {
		inputs["items"] = items
	}
	return inputs
}

func buildAnswerNodeFrontendInputs(nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	inputs := map[string]interface{}{}
	for _, key := range []string{"answer", "template", "content"} {
		if text := stringValue(firstExisting(nodeData, key)); text != "" {
			for varKey, value := range resolveTemplateVariables(text, variablePool) {
				inputs[varKey] = value
			}
		}
	}
	return inputs
}

func buildSchemaTablesSnapshot(dataSource interface{}, tables interface{}) []interface{} {
	dataSourceName := displayDataSourceName(dataSource)
	tableItems := tableItemsFromValue(tables)
	result := make([]interface{}, 0, len(tableItems))
	for _, table := range tableItems {
		tableName := displayTableName(table)
		if tableName == "" {
			continue
		}
		if dataSourceName != "" {
			result = append(result, dataSourceName+"."+tableName)
			continue
		}
		result = append(result, tableName)
	}
	return result
}

func buildTableSchemaSnapshot(value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []interface{}{map[string]interface{}{
			"id":      "",
			"content": typed,
		}}
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, tableSchemaItemSnapshot(item))
		}
		return result
	default:
		if tables := tableItemsFromValue(value); len(tables) > 0 {
			result := make([]interface{}, 0, len(tables))
			for _, table := range tables {
				result = append(result, tableSchemaItemSnapshot(table))
			}
			return result
		}
		return value
	}
}

func tableSchemaItemSnapshot(value interface{}) interface{} {
	if m, ok := value.(map[string]interface{}); ok {
		result := make(map[string]interface{}, len(m)+1)
		result["id"] = tableIDValue(m)
		for key, item := range m {
			result[key] = item
		}
		return result
	}
	return value
}

func displayDataSourceName(value interface{}) string {
	if m, ok := value.(map[string]interface{}); ok {
		if source, ok := m["source"].(map[string]interface{}); ok {
			return firstNonEmptyString(source["name"], source["id"])
		}
		return firstNonEmptyString(m["name"], m["id"])
	}
	return ""
}

func tableItemsFromValue(value interface{}) []interface{} {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]interface{}); ok {
		if tables, ok := m["tables"].([]interface{}); ok {
			return tables
		}
		return []interface{}{m}
	}
	if items, ok := value.([]interface{}); ok {
		return items
	}
	return nil
}

func displayTableName(value interface{}) string {
	if m, ok := value.(map[string]interface{}); ok {
		return firstNonEmptyString(m["name"], m["table_name"], m["id"])
	}
	return stringValue(value)
}

func tableIDValue(value map[string]interface{}) interface{} {
	for _, key := range []string{"id", "table_id", "name"} {
		if item, exists := value[key]; exists {
			return item
		}
	}
	return ""
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func resolvePromptVariablesFromNodeData(nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	result := map[string]interface{}{}
	for _, key := range []string{"prompt", "prompt_template"} {
		raw := firstExisting(nodeData, key)
		if text := stringValue(raw); text != "" {
			for varKey, value := range resolveTemplateVariables(text, variablePool) {
				result[varKey] = value
			}
			continue
		}
		if promptMap, ok := raw.(map[string]interface{}); ok {
			for _, textKey := range []string{"user", "system", "text"} {
				for varKey, value := range resolveTemplateVariables(stringValue(promptMap[textKey]), variablePool) {
					result[varKey] = value
				}
			}
			resolveVariableSelectorsInto(result, promptMap["quick_bindings"], variablePool)
		}
	}
	if promptConfig, ok := firstExisting(nodeData, "prompt_config").(map[string]interface{}); ok {
		resolveVariableSelectorsInto(result, promptConfig["template_variables"], variablePool)
	}
	return result
}

func buildPromptSnapshot(raw interface{}, nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) interface{} {
	switch typed := raw.(type) {
	case string:
		return renderConfiguredTemplateText(typed, nodeData, variablePool)
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			if text, ok := value.(string); ok {
				result[key] = renderConfiguredTemplateText(text, nodeData, variablePool)
				continue
			}
			result[key] = buildPromptSnapshot(value, nodeData, variablePool)
		}
		return result
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, buildPromptSnapshot(item, nodeData, variablePool))
		}
		return result
	default:
		return raw
	}
}

func resolveLoopVariables(raw interface{}, variablePool *graph_entities.VariablePool) map[string]interface{} {
	result := map[string]interface{}{}
	switch variables := raw.(type) {
	case map[string]interface{}:
		for key, value := range variables {
			result[key] = value
		}
	case []interface{}:
		for _, item := range variables {
			spec, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			label := strings.TrimSpace(stringValue(spec["label"]))
			if label == "" {
				continue
			}
			if stringValue(spec["value_type"]) == "variable" {
				selector := interfaceToStringSlice(spec["value"])
				if value, ok := resolveSelectorValue(variablePool, selector); ok {
					result[label] = value
					continue
				}
			}
			result[label] = spec["value"]
		}
	}
	return result
}

func resolveTemplateVariables(text string, variablePool *graph_entities.VariablePool) map[string]interface{} {
	result := map[string]interface{}{}
	if strings.TrimSpace(text) == "" {
		return result
	}
	start := 0
	for {
		startIndex := strings.Index(text[start:], "{{")
		if startIndex == -1 {
			break
		}
		startIndex += start
		endIndex := strings.Index(text[startIndex:], "}}")
		if endIndex == -1 {
			break
		}
		endIndex += startIndex
		token := strings.TrimSpace(text[startIndex+2 : endIndex])
		selector := templateTokenToSelector(token)
		if len(selector) >= 2 {
			if value, ok := resolveSelectorValue(variablePool, selector); ok {
				result[selectorDisplayKey(selector)] = value
			}
		}
		start = endIndex + 2
	}
	return result
}

func renderTemplateText(text string, variablePool *graph_entities.VariablePool) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	result := text
	for token, value := range resolveTemplateVariableTokens(text, variablePool) {
		result = strings.ReplaceAll(result, "{{"+token+"}}", stringValueForTemplate(value))
		result = strings.ReplaceAll(result, "{{ "+token+" }}", stringValueForTemplate(value))
	}
	return result
}

func renderConfiguredTemplateText(text string, nodeData map[string]interface{}, variablePool *graph_entities.VariablePool) string {
	result := renderTemplateText(text, variablePool)
	if promptConfig, ok := firstExisting(nodeData, "prompt_config").(map[string]interface{}); ok {
		result = renderNamedTemplateVariables(result, promptConfig["template_variables"], variablePool)
	}
	return result
}

func renderNamedTemplateVariables(text string, raw interface{}, variablePool *graph_entities.VariablePool) string {
	selectors, ok := raw.([]interface{})
	if !ok {
		return text
	}
	result := text
	for _, item := range selectors {
		selectorMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringValue(selectorMap["variable"]))
		if name == "" {
			continue
		}
		selector := interfaceToStringSlice(selectorMap["value_selector"])
		value, ok := resolveSelectorValue(variablePool, selector)
		if !ok {
			value = ""
		}
		for _, token := range []string{"{{" + name + "}}", "{{ " + name + " }}"} {
			result = strings.ReplaceAll(result, token, stringValueForTemplate(value))
		}
	}
	return result
}

func resolveTemplateVariableTokens(text string, variablePool *graph_entities.VariablePool) map[string]interface{} {
	result := map[string]interface{}{}
	start := 0
	for {
		startIndex := strings.Index(text[start:], "{{")
		if startIndex == -1 {
			break
		}
		startIndex += start
		endIndex := strings.Index(text[startIndex:], "}}")
		if endIndex == -1 {
			break
		}
		endIndex += startIndex
		token := strings.TrimSpace(text[startIndex+2 : endIndex])
		selector := templateTokenToSelector(token)
		if len(selector) >= 2 {
			if value, ok := resolveSelectorValue(variablePool, selector); ok {
				result[token] = value
			}
		}
		start = endIndex + 2
	}
	return result
}

func stringValueForTemplate(value interface{}) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func resolveVariableSelectorsInto(result map[string]interface{}, raw interface{}, variablePool *graph_entities.VariablePool) {
	selectors, ok := raw.([]interface{})
	if !ok {
		return
	}
	for _, item := range selectors {
		selectorMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		selector := interfaceToStringSlice(selectorMap["value_selector"])
		if len(selector) < 2 {
			continue
		}
		key := strings.TrimSpace(stringValue(selectorMap["variable"]))
		if key == "" {
			key = selectorDisplayKey(selector)
		}
		if value, ok := resolveSelectorValue(variablePool, selector); ok {
			result[key] = value
		}
	}
}

func templateTokenToSelector(token string) []string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "#")
	if token == "" {
		return nil
	}
	parts := strings.Split(token, ".")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil
		}
	}
	return parts
}

func interfaceToStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			itemString, ok := item.(string)
			if !ok {
				return nil
			}
			result = append(result, itemString)
		}
		return result
	default:
		return nil
	}
}

func resolveSelectorValue(variablePool *graph_entities.VariablePool, selector []string) (interface{}, bool) {
	if variablePool == nil || len(selector) < 2 {
		return nil, false
	}
	variable := variablePool.GetWithPath(selector)
	if variable == nil {
		return nil, false
	}
	return variable.ToObject(), true
}

func selectorDisplayKey(selector []string) string {
	if len(selector) == 0 {
		return ""
	}
	key := strings.TrimSpace(selector[len(selector)-1])
	if key == "" {
		return strings.Join(selector, ".")
	}
	return key
}
