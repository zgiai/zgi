package guard

type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

type Verdict string

const (
	VerdictAllow Verdict = "allow"
	VerdictDeny  Verdict = "deny"
)

type ReasonCode string

const (
	ReasonParseError     ReasonCode = "parse_error"
	ReasonMultiStatement ReasonCode = "multi_stmt"
	ReasonBlockStatement ReasonCode = "block_statement"
	ReasonBlockFunction  ReasonCode = "block_function"
	ReasonRequireWhere   ReasonCode = "require_where"
	ReasonReadonly       ReasonCode = "readonly"
	ReasonMaxJoinDepth   ReasonCode = "max_join_depth"
)

type Reason struct {
	Code    ReasonCode `json:"code"`
	Message string     `json:"message"`
	Detail  string     `json:"detail,omitempty"`
}

type Result struct {
	Verdict Verdict  `json:"verdict"`
	Action  Action   `json:"action"`
	Reasons []Reason `json:"reasons,omitempty"`
	Policy  Policy   `json:"policy"`
}

func (r Result) Denied() bool {
	return r.Verdict == VerdictDeny
}

type DeniedError struct {
	Result Result
}

func (e *DeniedError) Error() string {
	if len(e.Result.Reasons) == 0 {
		return "sql guard denied query"
	}
	return "sql guard denied query: " + e.Result.Reasons[0].Message
}
