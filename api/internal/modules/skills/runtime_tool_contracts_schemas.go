package skills

func databaseListContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "list_accessible_databases",
		Description: "List databases accessible to the current user or bound to the current Agent.",
		Schema: objectSchema(
			map[string]interface{}{
				"query": stringValueSchema("Optional search text for narrowing candidate databases."),
				"limit": numberSchema("Maximum number of databases to list. Defaults to 20."),
			},
			nil,
		),
		Example: map[string]interface{}{"query": "customers", "limit": 10},
	}
}

func databaseTablesContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "list_database_tables",
		Description: "List tables in an accessible database.",
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id": stringValueSchema("Database ID returned by list_accessible_databases."),
				"query":          stringValueSchema("Optional search text for narrowing tables by name or description."),
				"limit":          numberSchema("Maximum number of tables to list. Defaults to 50."),
			},
			[]string{"data_source_id"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "query": "orders"},
	}
}

func databaseDescribeTableContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "describe_database_table",
		Description: "Describe a database table and its columns.",
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id":        stringValueSchema("Database ID returned by list_accessible_databases."),
				"table_id":              stringValueSchema("Table metadata ID returned by list_database_tables."),
				"include_system_fields": booleanSchema("Whether to include system fields such as id and timestamps. Defaults to false."),
			},
			[]string{"data_source_id", "table_id"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "table_id": "table-id"},
	}
}

func databaseQueryRecordsContract(skillID string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    "query_table_records",
		Description: "Query table records with pagination and a safe order clause.",
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id": stringValueSchema("Database ID returned by list_accessible_databases."),
				"table_id":       stringValueSchema("Table metadata ID returned by list_database_tables."),
				"limit":          numberSchema("Maximum number of records. Defaults to 20 and is capped by the backend."),
				"offset":         numberSchema("Pagination offset. Defaults to 0."),
				"order":          stringValueSchema("Optional safe order clause such as id DESC."),
			},
			[]string{"data_source_id", "table_id"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "table_id": "table-id", "limit": 20},
	}
}

func databaseMutateRecordsContract(skillID string, toolName string, description string) SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     skillID,
		ToolName:    toolName,
		Description: description,
		Schema: objectSchema(
			map[string]interface{}{
				"data_source_id": stringValueSchema("Database ID returned by list_accessible_databases."),
				"table_id":       stringValueSchema("Table metadata ID returned by list_database_tables."),
				"records": map[string]interface{}{
					"type":        "array",
					"description": "Records to mutate. For update and delete, each record must include id.",
					"items": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": true,
					},
				},
			},
			[]string{"data_source_id", "table_id", "records"},
		),
		Example: map[string]interface{}{"data_source_id": "database-id", "table_id": "table-id", "records": []map[string]interface{}{{"id": "record-id"}}},
	}
}

func workflowListContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentWorkflow,
		ToolName:    "list_agent_workflows",
		Description: "Fallback/debug list of workflows bound to the current Agent. Prefer the injected available_workflows context when it is present.",
		Schema:      objectSchema(map[string]interface{}{}, nil),
		Example:     map[string]interface{}{},
	}
}

func workflowRunContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentWorkflow,
		ToolName:    "run_agent_workflow",
		Description: "Run an Agent-bound workflow by binding_id. Do not pass workflow_id directly. Set inputs.query to the user's current request. After a succeeded run, final answers must use primary_output or outputs and must not invent workflow output.",
		Schema: objectSchema(
			map[string]interface{}{
				"binding_id": stringValueSchema("Workflow binding ID from injected available_workflows, or from list_agent_workflows if the injected list is missing or ambiguous."),
				"inputs": map[string]interface{}{
					"type":                 "object",
					"description":          "Workflow input object. Include query with the user's current request unless the binding's input_schema, required_inputs, or default_input_key says otherwise; the runtime also forwards query as sys.query.",
					"additionalProperties": true,
					"properties": map[string]interface{}{
						"query": stringValueSchema("The user's current request or instruction to pass into the workflow."),
					},
					"required": []string{"query"},
				},
			},
			[]string{"binding_id", "inputs"},
		),
		Example: map[string]interface{}{"binding_id": "approval-flow", "inputs": map[string]interface{}{"query": "Approve refund request #123"}},
	}
}

func workflowRunStatusContract() SkillToolArgumentContract {
	return SkillToolArgumentContract{
		SkillID:     SkillAgentWorkflow,
		ToolName:    "get_workflow_run_status",
		Description: "Query the status and available outputs for an Agent-bound workflow run.",
		Schema: objectSchema(
			map[string]interface{}{
				"workflow_run_id": stringValueSchema("Workflow run ID returned by run_agent_workflow."),
			},
			[]string{"workflow_run_id"},
		),
		Example: map[string]interface{}{"workflow_run_id": "workflow-run-id"},
	}
}

func objectSchema(properties map[string]interface{}, required []string) map[string]interface{} {
	if required == nil {
		required = []string{}
	}
	return map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func numberSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
	}
}

func stringValueSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": description,
	}
}

func enumStringSchema(description string, values []string) map[string]interface{} {
	schema := stringValueSchema(description)
	if len(values) > 0 {
		schema["enum"] = values
	}
	return schema
}

func arraySchema(description string, items map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": description,
		"items":       items,
	}
}

func chartDataSchema() map[string]interface{} {
	series := arraySchema(
		"Chart data series. Radar supports 1-2 series; bar and line support 1-8 series.",
		objectSchema(
			map[string]interface{}{
				"name":   stringValueSchema("Series label."),
				"values": arraySchema("Numeric values matching the selected chart labels length.", numberSchema("Score or metric value.")),
				"color":  stringValueSchema("Optional #RRGGBB color."),
			},
			[]string{"name", "values"},
		),
	)
	pieItems := arraySchema(
		"Pie or doughnut chart items.",
		objectSchema(
			map[string]interface{}{
				"label": stringValueSchema("Slice label."),
				"value": numberSchema("Slice value."),
				"color": stringValueSchema("Optional #RRGGBB color."),
			},
			[]string{"label", "value"},
		),
	)
	scatterPoints := arraySchema(
		"Scatter chart points.",
		objectSchema(
			map[string]interface{}{
				"x":     numberSchema("X-axis value."),
				"y":     numberSchema("Y-axis value."),
				"label": stringValueSchema("Optional point label."),
				"color": stringValueSchema("Optional #RRGGBB color."),
			},
			[]string{"x", "y"},
		),
	)
	scoreCountBands := arraySchema(
		"Precomputed score distribution bands.",
		objectSchema(
			map[string]interface{}{
				"label": stringValueSchema("Band label such as 90-100."),
				"count": numberSchema("Precomputed count for this band."),
			},
			[]string{"label", "count"},
		),
	)
	scoreRangeBands := arraySchema(
		"Score distribution bands used to count raw scores.",
		objectSchema(
			map[string]interface{}{
				"label": stringValueSchema("Band label such as 90-100."),
				"min":   numberSchema("Inclusive minimum score when calculating from raw scores."),
				"max":   numberSchema("Inclusive maximum score when calculating from raw scores."),
			},
			[]string{"label", "min", "max"},
		),
	)
	common := map[string]interface{}{
		"max_value": numberSchema("Optional shared maximum value. Radar defaults to 100; bar and line auto-scale when omitted."),
		"series":    series,
	}
	radarProps := copySchemaProperties(common)
	radarProps["dimensions"] = stringArrayOrCSVSchema("Radar axis labels, such as subject names. Required for radar charts.")
	barProps := copySchemaProperties(common)
	barProps["categories"] = stringArrayOrCSVSchema("Bar chart category labels.")
	lineProps := copySchemaProperties(common)
	lineProps["x_axis"] = stringArrayOrCSVSchema("Line chart x-axis labels.")
	lineProps["categories"] = stringArrayOrCSVSchema("Line chart x-axis labels alias.")
	pieProps := map[string]interface{}{
		"items": pieItems,
	}
	scatterProps := map[string]interface{}{
		"x_label": stringValueSchema("Optional x-axis label."),
		"y_label": stringValueSchema("Optional y-axis label."),
		"x_min":   numberSchema("Optional x-axis minimum."),
		"x_max":   numberSchema("Optional x-axis maximum."),
		"y_min":   numberSchema("Optional y-axis minimum."),
		"y_max":   numberSchema("Optional y-axis maximum."),
		"points":  scatterPoints,
	}
	distributionCountProps := map[string]interface{}{
		"bands":     scoreCountBands,
		"max_value": numberSchema("Optional y-axis maximum for distribution counts."),
	}
	distributionRangeProps := map[string]interface{}{
		"bands": scoreRangeBands,
		"scores": arraySchema("Raw score values or objects with value.", map[string]interface{}{"oneOf": []interface{}{
			numberSchema("Raw score value."),
			objectSchema(map[string]interface{}{
				"label": stringValueSchema("Optional score label."),
				"value": numberSchema("Raw score value."),
			}, []string{"value"}),
		}}),
		"max_value": numberSchema("Optional y-axis maximum for distribution counts."),
	}

	return map[string]interface{}{
		"description": "Chart-specific data. Use dimensions for radar, categories for bar, x_axis or categories for line, items for pie/doughnut, points for scatter, and bands for score_distribution.",
		"anyOf": []interface{}{
			objectSchema(radarProps, []string{"dimensions", "series"}),
			objectSchema(barProps, []string{"categories", "series"}),
			objectSchema(lineProps, []string{"x_axis", "series"}),
			objectSchema(lineProps, []string{"categories", "series"}),
			objectSchema(pieProps, []string{"items"}),
			objectSchema(scatterProps, []string{"points"}),
			objectSchema(distributionCountProps, []string{"bands"}),
			objectSchema(distributionRangeProps, []string{"bands", "scores"}),
		},
	}
}

func architectureDiagramDataSchema() map[string]interface{} {
	node := objectSchema(map[string]interface{}{
		"id":    stringValueSchema("Stable node ID. Edges must reference this value."),
		"label": stringValueSchema("Human-readable node label."),
		"type":  stringValueSchema("Optional node type such as frontend, service, database, agent, model, tool, memory, input, output, or approval."),
		"group": stringValueSchema("Optional logical group."),
		"layer": stringValueSchema("Optional layout layer used to order nodes."),
	}, []string{"id"})
	edge := objectSchema(map[string]interface{}{
		"from":  stringValueSchema("Source node ID, participant name, state ID, or entity ID."),
		"to":    stringValueSchema("Target node ID, participant name, state ID, or entity ID."),
		"label": stringValueSchema("Optional relationship, transition, message, or data-flow label."),
	}, []string{"from", "to"})
	group := objectSchema(map[string]interface{}{
		"id":    stringValueSchema("Group ID."),
		"label": stringValueSchema("Group label."),
	}, []string{"id"})
	entity := objectSchema(map[string]interface{}{
		"id":     stringValueSchema("Stable entity ID. Relationships must reference this value."),
		"label":  stringValueSchema("Entity label."),
		"fields": stringArrayOrCSVSchema("Optional entity fields such as id PK, user_id FK, status."),
	}, []string{"id"})
	matrixCells := arraySchema("Matrix cell rows. Must align with rows and columns.", map[string]interface{}{
		"type":  "array",
		"items": map[string]interface{}{"type": "string"},
	})
	nodeEdgeProps := map[string]interface{}{
		"nodes":  arraySchema("Diagram nodes.", node),
		"edges":  arraySchema("Diagram edges.", edge),
		"groups": arraySchema("Optional visual or logical groups.", group),
	}
	return map[string]interface{}{
		"description": "Diagram-specific data. Node-edge diagrams use nodes, edges, and optional groups; comparison_matrix uses rows, columns, and cells; sequence uses participants and messages; state uses states and transitions; er uses entities and relationships.",
		"anyOf": []interface{}{
			objectSchema(nodeEdgeProps, []string{"nodes", "edges"}),
			objectSchema(map[string]interface{}{
				"columns": stringArrayOrCSVSchema("Compared products, vendors, options, or plans."),
				"rows":    stringArrayOrCSVSchema("Comparison criteria, features, metrics, or factors."),
				"cells":   matrixCells,
			}, []string{"columns", "rows", "cells"}),
			objectSchema(map[string]interface{}{
				"participants": stringArrayOrCSVSchema("Ordered sequence participants."),
				"messages":     arraySchema("Ordered sequence messages.", edge),
			}, []string{"participants", "messages"}),
			objectSchema(map[string]interface{}{
				"states":      arraySchema("State nodes.", node),
				"transitions": arraySchema("State transitions.", edge),
			}, []string{"states", "transitions"}),
			objectSchema(map[string]interface{}{
				"entities":      arraySchema("ER entities.", entity),
				"relationships": arraySchema("ER relationships.", edge),
			}, []string{"entities", "relationships"}),
		},
	}
}

func copySchemaProperties(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func imageStyleSchema() map[string]interface{} {
	return enumStringSchema("Visual style. Defaults to auto.", []string{"auto", "realistic", "illustration", "flat", "3d", "guofeng", "tech", "poster", "product", "icon", "cover"})
}

func imageAspectRatioSchema() map[string]interface{} {
	return enumStringSchema("Image aspect ratio. Defaults to 1:1.", []string{"1:1", "16:9", "9:16", "4:3"})
}

func imageFileObjectSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"description": description + " Supported formats: PNG, JPG, JPEG, WEBP.",
		"anyOf": []interface{}{
			stringValueSchema("File object encoded as JSON, or a file ID string."),
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"upload_file_id": stringValueSchema("Uploaded file ID."),
					"file_id":        stringValueSchema("Uploaded file ID."),
					"id":             stringValueSchema("Uploaded file ID."),
					"related_id":     stringValueSchema("Related uploaded file ID."),
					"name":           stringValueSchema("Optional filename."),
					"mime_type":      stringValueSchema("Optional MIME type."),
				},
				"additionalProperties": true,
			},
		},
	}
}

func booleanSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "boolean",
		"description": description,
	}
}

func stringArrayOrCSVSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"description": description,
		"oneOf": []interface{}{
			map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			map[string]interface{}{
				"type": "string",
			},
		},
	}
}

func intentStringArraySchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"description": description,
		"oneOf": []interface{}{
			map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			map[string]interface{}{
				"type":        "string",
				"description": "JSON array string of strings.",
			},
		},
	}
}

func boundedNumberSchema(description string, minimum float64, maximum float64) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
		"minimum":     minimum,
		"maximum":     maximum,
	}
}

func intentTaskTypes() []string {
	return []string{
		"general_qa",
		"knowledge_retrieval",
		"database_query",
		"database_mutation",
		"workflow_execution",
		"file_generation",
		"chart_generation",
		"report_generation",
		"schedule_planning",
		"calculation",
		"code_or_debugging",
		"data_analysis",
		"clarification_required",
		"unsupported",
	}
}

func intentRecommendedActions() []string {
	return []string{
		"answer_directly",
		"call_skill",
		"call_tool",
		"run_workflow",
		"query_database",
		"mutate_database",
		"retrieve_knowledge",
		"request_user_input",
		"reject_or_escalate",
	}
}

func intentRoutingHintsSchema() map[string]interface{} {
	return objectSchema(
		map[string]interface{}{
			"needs_context":             booleanSchema("Whether more conversation or domain context is needed."),
			"uses_uploaded_files":       booleanSchema("Whether uploaded files are required for execution."),
			"requires_database":         booleanSchema("Whether database access is required."),
			"requires_knowledge_base":   booleanSchema("Whether knowledge retrieval is required."),
			"requires_workflow":         booleanSchema("Whether workflow execution or inspection is required."),
			"requires_file_generation":  booleanSchema("Whether file generation is required."),
			"requires_chart_generation": booleanSchema("Whether chart generation is required."),
			"requires_confirmation":     booleanSchema("Whether explicit user confirmation is required."),
			"is_high_impact":            booleanSchema("Whether the next action is high impact."),
			"is_multi_intent":           booleanSchema("Whether the request contains multiple task intents."),
		},
		nil,
	)
}

func intentMissingInfoSchema() map[string]interface{} {
	return arraySchema(
		"Missing information that blocks reliable execution.",
		objectSchema(
			map[string]interface{}{
				"field":    stringValueSchema("Stable missing field name such as chart_type, file_format, database_table, workflow_binding_id, or confirmation."),
				"reason":   stringValueSchema("Why this field is required."),
				"question": stringValueSchema("Concise user-facing question that resolves this blocker."),
				"options":  intentStringArraySchema("Optional concrete quick-reply options, maximum five."),
			},
			[]string{"field", "reason", "question"},
		),
	)
}

func intentUploadedFilesSchema() map[string]interface{} {
	return arraySchema(
		"Uploaded file metadata relevant to routing. Do not include raw file contents.",
		map[string]interface{}{
			"anyOf": []interface{}{
				objectSchema(intentUploadedFileProperties(), []string{"file_id"}),
				objectSchema(intentUploadedFileProperties(), []string{"filename"}),
			},
		},
	)
}

func intentUploadedFileProperties() map[string]interface{} {
	return map[string]interface{}{
		"file_id":   stringValueSchema("Optional file identifier."),
		"filename":  stringValueSchema("Optional filename."),
		"mime_type": stringValueSchema("Optional MIME type."),
		"format":    stringValueSchema("Optional file format or extension."),
		"role":      stringValueSchema("Optional role such as source, reference, attachment, or output_template."),
		"summary":   stringValueSchema("Optional short file summary."),
	}
}

func intentAlternateIntentsSchema() map[string]interface{} {
	return arraySchema(
		"Optional secondary plausible intents.",
		objectSchema(
			map[string]interface{}{
				"intent_id":            stringValueSchema("Stable dotted lowercase identifier for the alternate intent."),
				"task_type":            enumStringSchema("Alternate task type.", intentTaskTypes()),
				"confidence":           boundedNumberSchema("Alternate confidence from 0 to 1.", 0, 1),
				"recommended_action":   enumStringSchema("Optional recommended action for the alternate intent.", intentRecommendedActions()),
				"recommended_skill_id": stringValueSchema("Optional target skill for the alternate intent."),
			},
			[]string{"intent_id", "task_type", "confidence"},
		),
	)
}

func precisionSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": "Optional decimal places to round the result to. Defaults to 6 and must be between 0 and 12.",
		"minimum":     0,
		"maximum":     12,
	}
}
