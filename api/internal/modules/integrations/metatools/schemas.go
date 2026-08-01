package metatools

func strictObject(properties map[string]interface{}, required ...string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func identifierSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "minLength": 1, "maxLength": 128,
		"pattern": "^[a-z0-9][a-z0-9._-]*$", "description": description,
	}
}

func connectionIDSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "format": "uuid", "readOnly": true,
		"description": "An explicit internal connection ID retained for compatibility. Prefer omitting this field so the server resolves the chat's selected preferred connection.",
	}
}

func connectionSelectorSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "enum": []string{"preferred"},
		"description": "Resolve the preferred connection selected for this integration in the current chat. This is also the default when both connection fields are omitted.",
	}
}

func readOnlyNameSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "maxLength": 128, "readOnly": true,
	}
}

func readOnlyLocalizedNameSchema() map[string]interface{} {
	schema := localizedTextSchema(128)
	schema["readOnly"] = true
	return schema
}

func readOnlyArgumentLabelsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"maxProperties":        64,
		"propertyNames":        map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 128},
		"additionalProperties": localizedTextSchema(128),
		"readOnly":             true,
	}
}

func readOnlyArgumentValueLabelsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":          "object",
		"maxProperties": 64,
		"propertyNames": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 128},
		"additionalProperties": map[string]interface{}{
			"type":          "object",
			"maxProperties": 16,
			"propertyNames": map[string]interface{}{"type": "string", "minLength": 2, "maxLength": 35},
			"additionalProperties": map[string]interface{}{
				"type":                 "object",
				"maxProperties":        64,
				"propertyNames":        map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 256},
				"additionalProperties": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 128},
			},
		},
		"readOnly": true,
	}
}

func listConnectionsInputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"integration_id": identifierSchema("Optional integration ID to filter the selected connections."),
	})
}

func searchActionsInputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"query": map[string]interface{}{
			"type": "string", "maxLength": 256,
			"description": "Optional capability, object, or operation keywords. Leave empty to list available actions.",
		},
		"integration_id": identifierSchema("Optional integration ID to restrict the search."),
		"limit": map[string]interface{}{
			"type": "integer", "minimum": 1, "maximum": 20, "default": 10,
		},
	})
}

func actionGuideInputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"integration_id": identifierSchema("The integration that owns the action."),
		"action_id":      identifierSchema("The stable action ID returned by search_actions."),
	}, "integration_id", "action_id")
}

func executeActionInputSchema() map[string]interface{} {
	schema := strictObject(map[string]interface{}{
		"integration_id":             identifierSchema("The integration that owns the action."),
		"integration_name":           readOnlyNameSchema(),
		"integration_name_i18n":      readOnlyLocalizedNameSchema(),
		"action_id":                  identifierSchema("The stable action ID returned by search_actions."),
		"action_name":                readOnlyNameSchema(),
		"action_name_i18n":           readOnlyLocalizedNameSchema(),
		"argument_labels_i18n":       readOnlyArgumentLabelsSchema(),
		"argument_value_labels_i18n": readOnlyArgumentValueLabelsSchema(),
		"connection_id":              connectionIDSchema(),
		"connection_selector":        connectionSelectorSchema(),
		"connection_name": map[string]interface{}{
			"type": "string", "maxLength": 128, "readOnly": true,
		},
		"connection_display_name": map[string]interface{}{
			"type": "string", "maxLength": 255, "readOnly": true,
		},
		"connection_selection": map[string]interface{}{
			"type": "string", "enum": []string{"preferred", "explicit"}, "readOnly": true,
		},
		"arguments": map[string]interface{}{
			"type": "object", "description": "Arguments that satisfy the input_schema returned by get_action_guide.",
			"additionalProperties": true,
		},
		// These annotations are populated by the provider when absent before
		// governance freezes an invocation. Any supplied value must still match
		// the current catalog, so a stale frozen call fails closed.
		"action_schema_hash": map[string]interface{}{
			"type": "string", "maxLength": 128, "readOnly": true,
		},
		"action_schema_revision": map[string]interface{}{
			"type": "string", "maxLength": 128, "readOnly": true,
		},
		"catalog_revision": map[string]interface{}{
			"type": "string", "maxLength": 128, "readOnly": true,
		},
	}, "integration_id", "action_id", "arguments")
	// An explicit UUID and a server-side selector are mutually exclusive. Both
	// may be omitted because the server then applies the preferred selector.
	schema["allOf"] = []interface{}{
		map[string]interface{}{
			"not": map[string]interface{}{"required": []string{"connection_id", "connection_selector"}},
		},
	}
	return schema
}

func connectionSummarySchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"integration_id": identifierSchema("Integration ID."),
		"driver_id":      identifierSchema("Provider driver ID."),
		"name":           map[string]interface{}{"type": "string", "maxLength": 128},
		"display_name":   map[string]interface{}{"type": "string", "maxLength": 255},
		"selection":      map[string]interface{}{"type": "string", "enum": []string{"preferred", "selected"}},
		"status":         map[string]interface{}{"type": "string"},
		"health_status":  map[string]interface{}{"type": "string"},
		"auth_status":    map[string]interface{}{"type": "string"},
		"scope_status":   map[string]interface{}{"type": "string"},
		"attention_code": map[string]interface{}{"type": "string"},
	}, "integration_id", "driver_id", "name", "selection", "status", "health_status", "auth_status", "scope_status")
}

func listConnectionsOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"connections": map[string]interface{}{
			"type": "array", "maxItems": maxSelectedConnections, "items": connectionSummarySchema(),
		},
		"count": map[string]interface{}{"type": "integer", "minimum": 0, "maximum": maxSelectedConnections},
	}, "connections", "count")
}

func actionSummarySchema() map[string]interface{} {
	compactArgumentSchema := strictObject(map[string]interface{}{
		"name": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 64},
		"type": map[string]interface{}{
			"type":  []string{"string", "array"},
			"items": map[string]interface{}{"type": "string", "maxLength": 32},
		},
	}, "name", "type")
	preparationHintSchema := strictObject(map[string]interface{}{
		"action_id":        identifierSchema("Preparation action ID."),
		"relation":         map[string]interface{}{"type": "string", "enum": []string{"resolve_target", "inspect"}},
		"target_arguments": map[string]interface{}{"type": "array", "maxItems": 8, "items": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 128}},
		"result_paths":     map[string]interface{}{"type": "array", "maxItems": 16, "items": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 256}},
		"description":      map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 1000},
		"description_i18n": localizedTextSchema(1000),
	}, "action_id", "relation", "target_arguments", "result_paths", "description")
	return strictObject(map[string]interface{}{
		"integration_id":          identifierSchema("Integration ID."),
		"action_id":               identifierSchema("Stable action ID."),
		"name":                    map[string]interface{}{"type": "string", "maxLength": 128},
		"name_i18n":               localizedTextSchema(128),
		"description":             map[string]interface{}{"type": "string", "maxLength": 1200},
		"description_i18n":        localizedTextSchema(1200),
		"effect":                  map[string]interface{}{"type": "string"},
		"risk_level":              map[string]interface{}{"type": "string"},
		"data_egress":             map[string]interface{}{"type": "boolean"},
		"external_destination":    map[string]interface{}{"type": "string", "maxLength": 255},
		"required_scopes":         map[string]interface{}{"type": "array", "maxItems": 128, "items": map[string]interface{}{"type": "string", "maxLength": 255}},
		"required_any_scopes":     map[string]interface{}{"type": "array", "maxItems": 128, "items": map[string]interface{}{"type": "string", "maxLength": 255}},
		"preferred_scopes":        map[string]interface{}{"type": "array", "maxItems": 128, "items": map[string]interface{}{"type": "string", "maxLength": 255}},
		"scope_labels_i18n":       localizedLabelMapSchema(128, 128),
		"schema_hash":             map[string]interface{}{"type": "string", "maxLength": 128},
		"catalog_revision":        map[string]interface{}{"type": "string", "maxLength": 128},
		"connection_name":         map[string]interface{}{"type": "string", "maxLength": 128},
		"connection_display_name": map[string]interface{}{"type": "string", "maxLength": 255},
		"connection_selection":    map[string]interface{}{"type": "string", "enum": []string{"preferred"}},
		"availability":            map[string]interface{}{"type": "string", "enum": []string{"ready", "scope_upgrade_required"}},
		"can_execute":             map[string]interface{}{"type": "boolean"},
		"recovery_action":         map[string]interface{}{"type": "string", "enum": []string{"upgrade_oauth_scope"}},
		"requires_approval":       map[string]interface{}{"type": "boolean"},
		"required_arguments": map[string]interface{}{
			"type": "array", "maxItems": 64, "items": compactArgumentSchema,
		},
		"optional_arguments": map[string]interface{}{
			"type": "array", "maxItems": 64, "items": compactArgumentSchema,
		},
		"guide_recommended": map[string]interface{}{"type": "boolean"},
		"preparation_hints": map[string]interface{}{"type": "array", "maxItems": 8, "items": preparationHintSchema},
	}, "integration_id", "action_id", "name", "description", "effect", "risk_level", "data_egress", "required_scopes", "schema_hash", "catalog_revision", "connection_name", "connection_selection")
}

func localizedTextSchema(maxLength int) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"maxProperties":        16,
		"propertyNames":        map[string]interface{}{"type": "string", "minLength": 2, "maxLength": 35},
		"additionalProperties": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": maxLength},
	}
}

func localizedLabelMapSchema(maxProperties int, maxLabelLength int) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"maxProperties":        maxProperties,
		"propertyNames":        map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 128},
		"additionalProperties": localizedTextSchema(maxLabelLength),
	}
}

func searchActionsOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"actions": map[string]interface{}{"type": "array", "maxItems": 20, "items": actionSummarySchema()},
		"count":   map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 20},
	}, "actions", "count")
}

func actionGuideOutputSchema() map[string]interface{} {
	properties := actionSummarySchema()["properties"].(map[string]interface{})
	copyProperties := make(map[string]interface{}, len(properties)+3)
	for key, value := range properties {
		copyProperties[key] = value
	}
	copyProperties["input_schema"] = map[string]interface{}{"type": "object"}
	copyProperties["output_schema"] = map[string]interface{}{"type": "object"}
	copyProperties["schema_revision"] = map[string]interface{}{"type": "string", "maxLength": 128}
	return strictObject(copyProperties,
		"integration_id", "action_id", "name", "description", "effect", "risk_level", "data_egress",
		"required_scopes", "schema_hash", "schema_revision", "catalog_revision", "connection_name", "connection_selection", "input_schema", "output_schema",
	)
}

func executeActionOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"integration_id":          identifierSchema("Integration ID."),
		"integration_name":        map[string]interface{}{"type": "string", "maxLength": 128},
		"integration_name_i18n":   localizedTextSchema(128),
		"action_id":               identifierSchema("Stable action ID."),
		"action_name":             map[string]interface{}{"type": "string", "maxLength": 128},
		"action_name_i18n":        localizedTextSchema(128),
		"connection_name":         map[string]interface{}{"type": "string", "maxLength": 128},
		"connection_display_name": map[string]interface{}{"type": "string", "maxLength": 255},
		"connection_selection":    map[string]interface{}{"type": "string", "enum": []string{"preferred", "explicit"}},
		"action_schema_hash":      map[string]interface{}{"type": "string", "maxLength": 128},
		"schema_revision":         map[string]interface{}{"type": "string", "maxLength": 128},
		"catalog_revision":        map[string]interface{}{"type": "string", "maxLength": 128},
		"provider_request_id":     map[string]interface{}{"type": "string", "maxLength": 512},
		"cost_usd":                map[string]interface{}{"type": "number", "minimum": 0},
		"result_count":            map[string]interface{}{"type": "integer", "minimum": 0},
		"attempt_count":           map[string]interface{}{"type": "integer", "minimum": 0},
		"result_truncated":        map[string]interface{}{"type": "boolean"},
		"result":                  map[string]interface{}{"type": "object", "additionalProperties": true},
	},
		"integration_id", "integration_name", "integration_name_i18n",
		"action_id", "action_name", "action_name_i18n",
		"connection_name", "connection_selection", "action_schema_hash", "schema_revision", "catalog_revision",
		"result_count", "attempt_count", "result",
	)
}
