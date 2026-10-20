package catalog

import "strings"

// SocialMCPDefinitions describe fixed native operations; a remote tool catalog
// cannot expand these contracts. LinkedIn messaging is declared for discoverability
// but deliberately not verified by the bridge without a supported partner API.
func SocialMCPDefinitions() []Definition {
	var definitions []Definition
	for _, entry := range []struct{ id, name, destination, help string }{
		{"x-social", "X", "api.x.com", "https://docs.x.com/fundamentals/authentication/oauth-2-0/authorization-code"},
		{"linkedin-social", "LinkedIn", "api.linkedin.com", "https://learn.microsoft.com/en-us/linkedin/marketing/community-management/community-management-overview"},
		{"facebook-social", "Facebook Pages", "graph.facebook.com", "https://developers.facebook.com/docs/pages-api/"},
	} {
		description := "Publish content, read interactions, private conversations and platform metrics through an isolated MCP bridge. Requires vendor permissions."
		descriptionZH := "通过独立 MCP 进程发布内容、读取互动、私信及平台指标，需相应平台权限。"
		if entry.id == "linkedin-social" {
			description = "Publish company content, read interactions and organization statistics through an isolated MCP bridge. Company-page messaging is unavailable."
			descriptionZH = "通过独立 MCP 进程发布公司主页内容、读取互动及组织统计。公司主页私信暂不可用。"
		}
		d := Definition{ID: entry.id, PluginID: "zgi." + entry.id, PluginVersion: "1.1.0", SourceDialect: SourceDialectAgentPluginsV1, SourceSchemaVersion: SourceSchemaVersion1,
			ProviderID: entry.id, DriverID: entry.id + "-mcp-v1", AuthMethodID: "access_token", Name: entry.name + " · MCP", NameI18n: LocalizedText{"en-US": entry.name + " · MCP", "zh-Hans": entry.name + " · MCP"},
			Description:     description,
			DescriptionI18n: LocalizedText{"en-US": description, "zh-Hans": descriptionZH},
			Icon:            strings.TrimSuffix(entry.id, "-social"), Classification: ClassificationConnector, CapabilityTypes: []CapabilityType{CapabilityTypeAction, CapabilityTypeData}, SetupMethod: SetupMethodKey, Ownership: OwnershipPersonal,
			ExtensionSchemaVersion: ZGIMetadataSchemaVersion13, ContractVersion: 4, Surfaces: []Surface{SurfaceChat, SurfaceAgent, SurfaceWorkflow, SurfaceDigitalHuman},
			KeyFields: []KeyField{{Name: "token", Label: "Platform access token", LabelI18n: LocalizedText{"en-US": "Platform access token", "zh-Hans": "平台访问令牌"}, Placeholder: "Access token", PlaceholderI18n: LocalizedText{"en-US": "Access token", "zh-Hans": "填写平台访问令牌"}, HelpURL: entry.help, Required: true, Secret: true}},
		}
		for _, action := range []struct {
			id, en, zh string
			write      bool
		}{
			{"account.get", "Read connected account", "读取已连接账号", false}, {"account.list", "List publishing accounts", "列出可运营账号", false},
			{"post.list", "List posts", "读取帖子列表", false}, {"post.get", "Read a post", "读取帖子", false}, {"post.create", "Publish a post", "发布帖子", true},
			{"comment.list", "Read comments", "读取评论", false}, {"comment.get", "Read a comment", "读取评论详情", false}, {"comment.create", "Comment on a post", "评论帖子", true}, {"comment.reply", "Reply to a comment", "回复评论", true},
			{"conversation.list", "List private conversations", "读取私信会话列表", false}, {"message.list", "Read private messages", "读取私信", false}, {"message.send", "Reply to a private conversation", "回复私信", true},
			{"metrics.get", "Read platform metrics", "读取平台指标", false},
			{"post.search", "Search recent posts", "搜索近期帖子", false}, {"mention.list", "Read mentions", "读取提及", false},
			{"notification.list", "Read organization notifications", "读取组织通知", false},
			{"post.repost", "Repost content", "转发帖子", true}, {"post.quote", "Quote a post", "引用帖子", true},
			{"webhook.subscribe", "Subscribe Page events", "订阅主页事件", true},
		} {
			switch action.id {
			case "post.search", "post.quote":
				if entry.id != "x-social" {
					continue
				}
			case "mention.list":
				if entry.id == "linkedin-social" {
					continue
				}
			case "notification.list":
				if entry.id != "linkedin-social" {
					continue
				}
			case "post.repost":
				if entry.id == "facebook-social" {
					continue
				}
			case "webhook.subscribe":
				if entry.id != "facebook-social" {
					continue
				}
			}
			effect, risk := OperationEffectRead, OperationRiskHintLow
			if action.write {
				effect, risk = OperationEffectExternalSend, OperationRiskHintHigh
			}
			if action.id == "post.create" || action.id == "post.repost" || action.id == "post.quote" {
				effect = OperationEffectPublish
			}
			op := Operation{ID: entry.id + "." + action.id, ToolName: strings.ReplaceAll(entry.id, "-", "_") + "_" + strings.ReplaceAll(action.id, ".", "_"), SkillID: "connector-runtime", Name: action.en, NameI18n: LocalizedText{"en-US": action.en, "zh-Hans": action.zh},
				Description: action.en + " using the connected platform account; access depends on the vendor grant.", DescriptionI18n: LocalizedText{"en-US": action.en + " using the connected platform account; access depends on the vendor grant.", "zh-Hans": action.zh + "，使用当前连接的账号，需平台授予对应权限。"},
				InputSchema: socialInput(entry.id, action.id), OutputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []any{"provider", "transport", "data"}, "properties": map[string]any{"provider": map[string]any{"const": entry.id}, "transport": map[string]any{"const": "mcp"}, "data": map[string]any{"type": "object"}}},
				Effect: effect, RiskHint: risk, DataEgress: true, ExternalDestination: entry.destination, Idempotent: !action.write}
			if entry.id == "linkedin-social" && (strings.HasPrefix(action.id, "message.") || action.id == "conversation.list") {
				op.Description = "Unavailable: LinkedIn company-page messaging requires a separately approved partner API; not provided by this integration."
				op.DescriptionI18n = LocalizedText{"en-US": op.Description, "zh-Hans": "不可用：LinkedIn 公司主页私信需单独获批的合作伙伴 API，本集成不提供。"}
			}
			d.Operations = append(d.Operations, op)
		}
		d.Operations = append(d.Operations, socialEventOperations(entry.id)...)
		definitions = append(definitions, d)
	}
	return definitions
}

func IsSocialMCPConnector(id string) bool {
	return id == "x-social" || id == "linkedin-social" || id == "facebook-social"
}

func socialInput(provider, action string) map[string]any {
	text := func(max int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
	}
	properties := map[string]any{}
	required := []any{}
	if action != "account.get" && action != "account.list" {
		properties["account_id"] = text(256)
		if provider == "linkedin-social" && action != "conversation.list" && action != "message.list" && action != "message.send" {
			required = append(required, "account_id")
		}
	}
	if strings.HasSuffix(action, ".list") || action == "post.search" {
		properties["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}
		properties["cursor"] = text(2048)
	}
	if provider == "x-social" && (action == "post.list" || action == "post.search" || action == "mention.list" || action == "comment.list") || provider == "linkedin-social" && action == "notification.list" {
		for _, key := range []string{"since", "until"} {
			properties[key] = map[string]any{"type": "string", "format": "date-time", "maxLength": 64}
		}
		if provider == "x-social" {
			for _, key := range []string{"since_id", "until_id"} {
				properties[key] = map[string]any{"type": "string", "pattern": "^[0-9]{1,19}$"}
			}
		}
	}
	addID := func(name string) { properties[name] = text(256); required = append(required, name) }
	switch action {
	case "post.search":
		properties["query"] = text(512)
		required = append(required, "query")
	case "post.repost", "post.quote":
		addID("post_id")
		if provider == "linkedin-social" || action == "post.quote" {
			properties["text"] = text(3000)
		}
		if action == "post.quote" {
			required = append(required, "text")
		}
	case "post.get", "comment.list":
		addID("post_id")
	case "comment.get":
		if provider == "linkedin-social" {
			addID("post_id")
		}
		addID("comment_id")
	case "post.create":
		properties["text"] = text(3000)
		required = append(required, "text")
		if provider == "facebook-social" {
			properties["image_url"] = text(2048)
		}
		properties["image_data"] = text(196608)
		properties["mime_type"] = map[string]any{"type": "string", "enum": []any{"image/png", "image/jpeg", "image/webp"}}
	case "comment.create":
		addID("post_id")
		properties["text"] = text(3000)
		required = append(required, "text")
	case "comment.reply":
		addID("post_id")
		addID("comment_id")
		properties["text"] = text(3000)
		required = append(required, "text")
	case "message.list", "message.send":
		addID("conversation_id")
		if action == "message.send" {
			properties["text"] = text(3000)
			required = append(required, "text")
		}
	case "metrics.get":
		properties["post_id"] = text(256)
		if provider == "facebook-social" {
			properties["metrics"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 10, "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{0,79}$"}}
		}
		properties["since"] = map[string]any{"type": "string", "format": "date-time", "maxLength": 64}
		properties["until"] = map[string]any{"type": "string", "format": "date-time", "maxLength": 64}
	}
	schema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false, "properties": properties, "required": required}
	if provider == "x-social" && properties["since_id"] != nil {
		schema["allOf"] = []any{
			map[string]any{"not": map[string]any{"required": []any{"since", "since_id"}}},
			map[string]any{"not": map[string]any{"required": []any{"until", "until_id"}}},
		}
	}
	if action == "post.create" {
		schema["dependentRequired"] = map[string]any{"image_data": []any{"mime_type"}, "mime_type": []any{"image_data"}}
		if provider == "facebook-social" {
			schema["not"] = map[string]any{"required": []any{"image_url", "image_data"}}
		}
	}
	return schema
}
