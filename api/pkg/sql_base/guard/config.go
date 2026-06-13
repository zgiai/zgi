package guard

import (
	"encoding/json"
	"strings"
)

type Mode string

const (
	ModeWarn    Mode = "warn"
	ModeEnforce Mode = "enforce"
)

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
	if policy.Mode != ModeEnforce {
		policy.Mode = ModeWarn
	}
	if policy.BlockStatements == nil {
		policy.BlockStatements = defaults.BlockStatements
	}
	if policy.BlockFunctions == nil {
		policy.BlockFunctions = defaults.BlockFunctions
	}
	policy.BlockStatements = normalizeList(policy.BlockStatements)
	policy.BlockFunctions = normalizeList(policy.BlockFunctions)
	if policy.MaxJoinDepth <= 0 {
		policy.MaxJoinDepth = defaults.MaxJoinDepth
	}
	return policy
}

func ParsePolicyJSON(data []byte) (Policy, error) {
	if len(data) == 0 {
		return DefaultPolicy(), nil
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	return NormalizePolicy(policy), nil
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
