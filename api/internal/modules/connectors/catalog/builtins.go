package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
)

// Built-in definitions are the accepted public contracts, owned by this
// catalog. Loading them never constructs a provider or reads user state.
//
//go:embed definitions/builtins.json
var builtinsFS embed.FS

const (
	GitHubPluginID = "zgi.github"
	ExaPluginID    = "zgi.exa-search"
)

func BuiltinDefinitions() ([]Definition, error) {
	data, err := builtinsFS.ReadFile("definitions/builtins.json")
	if err != nil {
		return nil, err
	}
	var definitions []Definition
	if err = json.Unmarshal(data, &definitions); err != nil {
		return nil, fmt.Errorf("decode built-in Connector contracts: %w", err)
	}
	for _, definition := range definitions {
		if err = Validate(definition); err != nil {
			return nil, err
		}
	}
	feishu, err := FeishuDefinition()
	if err != nil {
		return nil, err
	}
	messaging, err := MessagingDefinitions()
	if err != nil {
		return nil, err
	}
	mail := []Definition{}
	for _, id := range []string{"gmail", "qqmail", "netease163"} {
		d, _ := MailDefinition(id)
		if err := Validate(d); err != nil {
			return nil, err
		}
		mail = append(mail, d)
	}
	mcp := GitHubMCPDefinition()
	if err := Validate(mcp); err != nil {
		return nil, fmt.Errorf("validate GitHub MCP definition: %w", err)
	}
	return append(append(append(append(definitions, feishu), messaging...), mail...), mcp), nil
}

func ValidateBuiltinPackage(definitions []Definition) error {
	baseline, err := BuiltinDefinitions()
	if err != nil {
		return err
	}
	if len(definitions) != 1 {
		return ErrInvalidDefinition
	}
	candidate := definitions[0]
	for _, accepted := range baseline {
		if accepted.ID != candidate.ID || accepted.PluginID != candidate.PluginID {
			continue
		}
		if err = CheckCompatibleUpdate(accepted, candidate); err != nil {
			return err
		}
		return CheckCompatibleUpdate(candidate, accepted)
	}
	return ErrInvalidDefinition
}
