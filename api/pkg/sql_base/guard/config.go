package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Mode string

const (
	ModeWarn    Mode = "warn"
	ModeEnforce Mode = "enforce"
)

var ErrInvalidPolicy = errors.New("invalid sql guard policy")

type Policy struct {
	Mode              Mode     `json:"mode"`
	Readonly          bool     `json:"readonly"`
	AllowMultiStmt    bool     `json:"allow_multi_stmt"`
	BlockStatements   []string `json:"block_statements"`
	BlockFunctions    []string `json:"block_functions"`
	RequireWhere      bool     `json:"require_where"`
	MaxJoinDepth      int      `json:"max_join_depth"`
	AllowParseFailure bool     `json:"allow_parse_failure"`
}

type policyJSON struct {
	Mode              Mode     `json:"mode"`
	Readonly          bool     `json:"readonly"`
	AllowMultiStmt    bool     `json:"allow_multi_stmt"`
	BlockStatements   []string `json:"block_statements"`
	BlockFunctions    []string `json:"block_functions"`
	RequireWhere      bool     `json:"require_where"`
	MaxJoinDepth      *int     `json:"max_join_depth"`
	AllowParseFailure bool     `json:"allow_parse_failure"`
}

func (p *Policy) UnmarshalJSON(data []byte) error {
	var raw policyJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*p = Policy{
		Mode:              raw.Mode,
		Readonly:          raw.Readonly,
		AllowMultiStmt:    raw.AllowMultiStmt,
		BlockStatements:   raw.BlockStatements,
		BlockFunctions:    raw.BlockFunctions,
		RequireWhere:      raw.RequireWhere,
		AllowParseFailure: raw.AllowParseFailure,
	}
	if raw.MaxJoinDepth == nil {
		p.MaxJoinDepth = DefaultPolicy().MaxJoinDepth
	} else {
		p.MaxJoinDepth = *raw.MaxJoinDepth
	}
	return nil
}

func DefaultPolicy() Policy {
	return Policy{
		Mode:            ModeWarn,
		Readonly:        false,
		AllowMultiStmt:  false,
		BlockStatements: []string{"drop", "truncate", "alter", "create", "grant", "revoke"},
		BlockFunctions:  []string{"pg_read_file", "pg_read_binary_file", "pg_ls_dir", "pg_stat_file"},
		RequireWhere:    true,
		MaxJoinDepth:    8,
	}
}

func DefaultPolicyJSON() []byte {
	b, _ := json.Marshal(DefaultPolicy())
	return b
}

func NormalizePolicy(policy Policy) Policy {
	defaults := DefaultPolicy()
	if policy.Mode == "" {
		policy.Mode = defaults.Mode
	}
	if policy.BlockStatements == nil {
		policy.BlockStatements = defaults.BlockStatements
	}
	if policy.BlockFunctions == nil {
		policy.BlockFunctions = defaults.BlockFunctions
	}
	policy.BlockStatements = normalizeList(policy.BlockStatements)
	policy.BlockFunctions = normalizeList(policy.BlockFunctions)
	return policy
}

func ValidatePolicy(policy Policy) error {
	if policy.Mode != ModeWarn && policy.Mode != ModeEnforce {
		return fmt.Errorf("%w: invalid mode %q", ErrInvalidPolicy, policy.Mode)
	}
	if policy.MaxJoinDepth < 0 {
		return fmt.Errorf("%w: max_join_depth must be >= 0", ErrInvalidPolicy)
	}
	return nil
}

func NormalizeAndValidatePolicy(policy Policy) (Policy, error) {
	policy = NormalizePolicy(policy)
	if err := ValidatePolicy(policy); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func ParsePolicyJSON(data []byte) (Policy, error) {
	if len(data) == 0 {
		return DefaultPolicy(), nil
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	return NormalizeAndValidatePolicy(policy)
}

func normalizeList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
