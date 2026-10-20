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
	// The SSH Server Operations contract is the last built-in. Catalog
	// admission here is contract-only and never dials a host: this function
	// builds no provider or driver and wires no approval. The serveropsssh
	// driver and the Digital-Human surface gate are wired elsewhere, and a
	// runtime operation still requires a user-set-up credential, an exact bound
	// target, and fresh per-invocation approval.
	serveropsSSH := ServerOpsSSHDefinition()
	if err := Validate(serveropsSSH); err != nil {
		return nil, err
	}
	mcp := GitHubMCPDefinition()
	if err := Validate(mcp); err != nil {
		return nil, fmt.Errorf("validate GitHub MCP definition: %w", err)
	}
	// Keep the SSH contract as the final catalog entry; callers use that stable
	// position when validating the isolated Server Operations surface.
	return append(append(append(append(append(append(definitions, feishu), messaging...), mail...), mcp), SocialMCPDefinitions()...), serveropsSSH), nil
}

// ValidateBuiltinPackage admits exactly one package for a built-in plugin as
// either the exact current built-in contract or the reviewed Digital-Human
// projection of a pre-V4 built-in. A forged plugin version, operation,
// credential, or other immutable contract field is rejected.
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
		return validateBuiltinPackageCandidate(accepted, candidate)
	}
	return ErrInvalidDefinition
}

// validateBuiltinPackageCandidate accepts the candidate as an exact update of
// the accepted built-in or, for a pre-V4 built-in, as the additive Digital-Human
// projection at the single derived version. An already-admitted V4
// Digital-Human-only built-in is its own current package: it is compared
// exactly and is never upgraded a second time through WithDigitalHumanSurface
// with a derived version suffix.
func validateBuiltinPackageCandidate(accepted, candidate Definition) error {
	// A built-in may already publish the additive V4 contract itself. Admit its
	// exact current version before considering the older V1 → V4 projection.
	if candidate.PluginVersion == accepted.PluginVersion {
		if err := CheckCompatibleUpdate(accepted, candidate); err != nil {
			return err
		}
		return CheckCompatibleUpdate(candidate, accepted)
	}
	if alreadyV4DigitalHumanOnly(accepted) {
		if candidate.PluginVersion != accepted.PluginVersion {
			return ErrInvalidDefinition
		}
	} else if candidate.Supports(SurfaceDigitalHuman) {
		projected, err := WithDigitalHumanSurface(accepted, accepted.PluginVersion+"-digitalhuman.1")
		if err != nil || candidate.PluginVersion != projected.PluginVersion {
			return ErrInvalidDefinition
		}
		accepted = projected
	}
	if err := CheckCompatibleUpdate(accepted, candidate); err != nil {
		return err
	}
	return CheckCompatibleUpdate(candidate, accepted)
}

// alreadyV4DigitalHumanOnly reports whether the accepted built-in is itself the
// current V4 Digital-Human-only contract. Such a built-in has no earlier
// published base to project onto, so its version identity must be preserved
// exactly instead of being replaced by a derived Digital-Human suffix.
func alreadyV4DigitalHumanOnly(definition Definition) bool {
	return definition.IsDigitalHumanContract() &&
		len(definition.Surfaces) == 1 && definition.Surfaces[0] == SurfaceDigitalHuman
}
