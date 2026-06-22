package guard

func Check(sql string, policy Policy) Result {
	policy = NormalizePolicy(policy)
	a := parseSQL(sql)
	result := Result{
		Verdict: VerdictAllow,
		Action:  ActionAllow,
		Policy:  policy,
	}

	for _, rule := range rules {
		if reason, ok := rule(sql, policy, a); ok {
			result.Reasons = append(result.Reasons, reason)
		}
	}

	if len(result.Reasons) > 0 {
		result.Verdict = VerdictDeny
		if policy.Mode == ModeEnforce {
			result.Action = ActionDeny
		}
	}
	return result
}

type ruleFunc func(sql string, policy Policy, a analysis) (Reason, bool)

var rules = []ruleFunc{
	checkParseError,
	checkMultiStatement,
	checkBlockStatement,
	checkBlockFunction,
	checkRequireWhere,
	checkReadonly,
	checkMaxJoinDepth,
}
