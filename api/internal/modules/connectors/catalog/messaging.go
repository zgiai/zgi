package catalog

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
)

const (
	ZGIMetadataSchemaVersion12             = "1.2.0"
	SetupMethodCredentials     SetupMethod = "credentials"
)

func (d Definition) IsMessagingCredentials() bool {
	return d.ExtensionSchemaVersion == ZGIMetadataSchemaVersion12 || d.IsDigitalHumanContract() && d.SetupMethod == SetupMethodCredentials
}
func (d Definition) UsesCredentials() bool {
	return d.IsAppCredentials() || d.IsMessagingCredentials() || d.IsMailCredentials()
}

// MessagingDefinitions are static platform contracts. Their presence does not
// enable registration or establish provider admission for this deployment.
func MessagingDefinitions() ([]Definition, error) {
	result := []Definition{messagingDefinition("dingtalk"), WeComReadDefinition(), messagingDefinition("weixin")}
	for _, d := range result {
		if err := Validate(d); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func MessagingDefinition(id string) (Definition, bool) {
	if id != "dingtalk" && id != "wecom" && id != "weixin" {
		return Definition{}, false
	}
	if id == "wecom" {
		return WeComReadDefinition(), true
	}
	return messagingDefinition(id), true
}

func messagingDefinition(id string) Definition {
	d := Definition{ID: id, PluginID: "zgi." + id, PluginVersion: "1.0.1", SourceDialect: SourceDialectAgentPluginsV1,
		SourceSchemaVersion: SourceSchemaVersion1, ProviderID: id, DriverID: id + "-http-v1", AuthMethodID: id + ".credentials",
		Icon: id, Classification: ClassificationConnector, CapabilityTypes: []CapabilityType{CapabilityTypeAction, CapabilityTypeData},
		SetupMethod: SetupMethodCredentials, Ownership: OwnershipPersonal, Surfaces: FirstReleaseSurfaces(), KeyFields: []KeyField{},
		ExtensionSchemaVersion: ZGIMetadataSchemaVersion12, ContractVersion: 3,
		Setup: &SetupDescriptor{Handler: id + "-registration-v1", ProtocolVersion: 1, Methods: []string{"manual", "qr_connect", "qr_reauthorize"},
			Intents: []string{"register_bot", "reauthorize"}, AuthenticationKind: "bot_credentials", VerificationPolicy: "read_operation"},
	}
	field := func(name, en, zh string, secret bool, max int) CredentialField {
		return CredentialField{Name: name, Label: en, LabelI18n: LocalizedText{"en-US": en, "zh-Hans": zh}, Required: true, Secret: secret, Type: "string", MaxLength: max}
	}
	switch id {
	case "dingtalk":
		d.Name, d.Description = "DingTalk", "Send messages using a verified robot application. Recipient eligibility is checked when sending."
		d.NameI18n, d.DescriptionI18n = LocalizedText{"en-US": d.Name, "zh-Hans": "钉钉"}, LocalizedText{"en-US": d.Description, "zh-Hans": "通过机器人应用显式发送消息，发送时检查权限与接收目标。"}
		d.CapabilityTypes = []CapabilityType{CapabilityTypeAction}
		d.Setup.AuthenticationKind, d.Setup.VerificationPolicy = "app_credentials", "credential_and_conditional_operations"
		d.CredentialFields = []CredentialField{field("client_id", "Client ID / AppKey", "Client ID / AppKey", false, 128), field("client_secret", "Client Secret / AppSecret", "Client Secret / AppSecret", true, 16384)}
		d.Operations = []Operation{
			messagingOperation(id, "messages.send_direct", "Send a direct message", "发送单聊消息", "app", true, objectSchema(map[string]any{"user_ids": map[string]any{"type": "array", "items": textSchema(128), "minItems": 1, "maxItems": 20, "uniqueItems": true}, "text": textSchema(4000), "format": map[string]any{"type": "string", "enum": []string{"text", "markdown"}, "default": "text"}}, "user_ids", "text")),
			messagingOperation(id, "messages.send_group", "Send a group message", "发送群消息", "app", true, objectSchema(map[string]any{"conversation_id": textSchema(256), "text": textSchema(4000), "format": map[string]any{"type": "string", "enum": []string{"text", "markdown"}, "default": "text"}}, "conversation_id", "text")),
		}
	case "wecom":
		d.Name, d.Description = "WeCom", "Inspect the authorized bot, list recent sessions, and explicitly send messages."
		d.NameI18n, d.DescriptionI18n = LocalizedText{"en-US": d.Name, "zh-Hans": "企业微信"}, LocalizedText{"en-US": d.Description, "zh-Hans": "查看机器人授权说明和近期会话，并显式发送消息。"}
		d.CredentialFields = []CredentialField{field("bot_id", "Bot ID", "Bot ID", false, 128), field("secret", "Secret", "Secret", true, 16384)}
		d.Operations = []Operation{messagingOperation(id, "identity.get", "Inspect bot authorization", "查看机器人授权", "bot", false, objectSchema(nil)), messagingOperation(id, "sessions.list", "List recent sessions", "列出近期会话", "bot", false, objectSchema(nil)), messagingOperation(id, "messages.send", "Send a message", "发送消息", "bot", true, objectSchema(map[string]any{"context_handle": textSchema(128), "text": textSchema(4000)}, "context_handle", "text"))}
	case "weixin":
		d.Name, d.Description = "WeChat", "Explicitly retrieve direct messages and reply to a verified conversation context."
		d.NameI18n, d.DescriptionI18n = LocalizedText{"en-US": d.Name, "zh-Hans": "微信"}, LocalizedText{"en-US": d.Description, "zh-Hans": "显式读取个人会话消息，并通过已验证的会话上下文回复。"}
		d.Setup.AuthenticationKind, d.Setup.Methods, d.Setup.Intents = "session_token", []string{"qr_connect", "qr_reauthorize"}, []string{"login_session", "reauthorize"}
		d.CredentialFields = []CredentialField{field("account_id", "Account ID", "账号 ID", false, 128), field("token", "Token", "Token", true, 16384), field("base_url", "Service origin", "服务地址", false, 256)}
		d.Operations = []Operation{messagingOperation(id, "messages.pull", "Retrieve direct messages", "读取会话消息", "account", false, objectSchema(nil)), messagingOperation(id, "messages.reply", "Reply to a message", "回复消息", "account", true, objectSchema(map[string]any{"context_handle": textSchema(128), "text": textSchema(4000)}, "context_handle", "text"))}
	}
	// JSON Schema expects JSON-number/array types, including for static definitions.
	for i := range d.Operations {
		d.Operations[i].OutputSchema = messagingOutput(d.Operations[i].ID)
		encoded, _ := json.Marshal(d.Operations[i].InputSchema)
		_ = json.Unmarshal(encoded, &d.Operations[i].InputSchema)
		encoded, _ = json.Marshal(d.Operations[i].OutputSchema)
		_ = json.Unmarshal(encoded, &d.Operations[i].OutputSchema)
	}
	return d
}

func messagingOutput(id string) map[string]any {
	provider := strings.SplitN(id, ".", 2)[0]
	props := map[string]any{"provider": map[string]any{"const": provider}, "operation": map[string]any{"const": id}}
	required := []string{"provider", "operation"}
	accepted := map[string]any{"type": "string", "enum": []string{"accepted"}}
	switch id {
	case "dingtalk.messages.send_direct", "dingtalk.messages.send_group":
		props["status"] = map[string]any{"type": "string", "enum": []string{"accepted", "partial"}}
		props["receipt"] = textSchema(128)
		for _, name := range []string{"invalid_user_ids", "rate_limited_user_ids", "filtered_user_ids"} {
			props[name] = map[string]any{"type": "array", "maxItems": 20, "items": textSchema(128)}
			required = append(required, name)
		}
		required = append(required, "status", "receipt")
	case "wecom.identity.get":
		props["data"] = objectSchema(map[string]any{"authorization_context": textSchema(4096)}, "authorization_context")
		required = append(required, "data")
	case "wecom.sessions.list":
		item := objectSchema(map[string]any{"context_handle": textSchema(128), "name": map[string]any{"type": "string", "maxLength": 128}, "type": map[string]any{"enum": []string{"single", "group"}}, "last_message_time": map[string]any{"type": "string", "maxLength": 48}}, "context_handle", "name", "type", "last_message_time")
		props["data"] = objectSchema(map[string]any{"sessions": map[string]any{"type": "array", "maxItems": 20, "items": item}}, "sessions")
		required = append(required, "data")
	case "wecom.messages.send":
		props["data"] = objectSchema(map[string]any{"status": accepted}, "status")
		required = append(required, "data")
	case "weixin.messages.pull":
		item := objectSchema(map[string]any{"message_id": textSchema(256), "text": map[string]any{"type": "string", "maxLength": 4000}, "reply_available": map[string]any{"type": "boolean"}, "context_handle": textSchema(128)}, "message_id", "text", "reply_available")
		props["messages"] = map[string]any{"type": "array", "maxItems": 100, "items": item}
		required = append(required, "messages")
	case "weixin.messages.reply":
		props["status"] = accepted
		props["receipt"] = textSchema(128)
		required = append(required, "status", "receipt")
	}
	return objectSchema(props, required...)
}

func messagingOperation(provider, id, en, zh, subject string, send bool, input map[string]any) Operation {
	op := Operation{ID: provider + "." + id, ToolName: strings.ReplaceAll(provider+"_"+id, ".", "_"), SkillID: "connector-runtime", Name: en, NameI18n: LocalizedText{"en-US": en, "zh-Hans": zh}, Description: en, DescriptionI18n: LocalizedText{"en-US": en, "zh-Hans": zh}, InputSchema: input, OutputSchema: map[string]any{"type": "object"}, Effect: OperationEffectRead, RiskHint: OperationRiskHintLow, Idempotent: true, CredentialSubject: subject}
	if send {
		op.Effect, op.RiskHint, op.Idempotent, op.DataEgress, op.ExternalDestination = OperationEffectExternalSend, OperationRiskHintHigh, false, true, provider
	}
	// Pull advances a provider cursor; its local replay is fenced by logical invocation.
	if provider == "weixin" && id == "messages.pull" {
		op.Idempotent = false
	}
	return op
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	result := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}
func textSchema(max int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
}

func validateMessagingContract(d Definition) error {
	canonical, ok := MessagingDefinition(d.ID)
	if d.ID == "wecom" && d.PluginVersion != "1.1.0" && d.PluginVersion != "1.1.0-digitalhuman.1" {
		canonical = messagingDefinition("wecom")
	}
	if !ok || d.ContractVersion != 3 || d.ProviderID != canonical.ProviderID || d.DriverID != canonical.DriverID || d.AuthMethodID != canonical.AuthMethodID || d.SetupMethod != SetupMethodCredentials || len(d.KeyFields) != 0 || d.Setup == nil {
		return invalid("messaging_contract")
	}
	if !reflect.DeepEqual(d.Setup, canonical.Setup) || len(d.CredentialFields) != len(canonical.CredentialFields) {
		return invalid("messaging_setup")
	}
	for _, expected := range canonical.CredentialFields {
		found := false
		for _, field := range d.CredentialFields {
			if field.Name == expected.Name {
				if found || field.Required != expected.Required || field.Secret != expected.Secret || field.Type != expected.Type || field.MaxLength != expected.MaxLength || len(field.Enum) != 0 || strings.TrimSpace(field.Label) == "" || !validLocalizedText(field.LabelI18n) {
					return invalid("messaging_credential_field")
				}
				found = true
			}
		}
		if !found {
			return invalid("messaging_credential_field")
		}
	}
	if len(d.Operations) != len(canonical.Operations) {
		return invalid("messaging_operation")
	}
	for _, op := range d.Operations {
		matched := false
		for _, expected := range canonical.Operations {
			// Skill ownership is frozen by the accepted version digest and the
			// ordinary update check. Provider shape validation must also work
			// for historical accepted versions after a runtime capability rename.
			expected.SkillID = op.SkillID
			a, _ := digestJSON(canonicalOperationContract(op))
			b, _ := digestJSON(canonicalOperationContract(expected))
			if a == b && op.CredentialSubject == expected.CredentialSubject {
				matched = true
			}
		}
		if !matched || len(op.RequiredScopes) != 0 {
			return invalid("messaging_operation")
		}
	}
	return nil
}

// CredentialIdentity is an internal canonical subject, never an owner claim.
func CredentialIdentity(d Definition, values map[string]string) string {
	if d.IsMailCredentials() {
		return values["email"]
	}
	switch d.ID {
	case "dingtalk":
		return "dingtalk:app:" + values["client_id"]
	case "wecom":
		return "wecom:bot:" + values["bot_id"]
	case "weixin":
		return "weixin:account:" + values["account_id"]
	}
	if d.IsAppCredentials() {
		return values["region"] + ":" + values["app_id"]
	}
	return ""
}

func validMessagingOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/") && u.Host == "ilinkai.weixin.qq.com"
}
