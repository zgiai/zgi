package skills

import (
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// marketCostReasonPattern reads the two amounts of MarketCostApprovalReason back out of it.
var marketCostReasonPattern = regexp.MustCompile(
	`^one call can cost up to (\d+(?:\.\d{1,3})?) points, above the confirmation threshold of (\d+(?:\.\d{1,3})?) points$`)

// MarketCostApprovalReason is the reason of a ZGI Market tool that asks for approval because of
// its cost alone: the most one call can cost and the threshold it is over, in points. The text is
// fixed, so that whoever shows it can read the two amounts back out and say them in the reader's
// language; read as it is, it is a sentence.
func MarketCostApprovalReason(maxPoints, thresholdPoints string) string {
	return "one call can cost up to " + maxPoints + " points, above the confirmation threshold of " + thresholdPoints + " points"
}

// marketToolUserMessageKey is the member of a decision's model feedback that holds what the end
// user is told when a market tool is refused, by language.
const marketToolUserMessageKey = "user_message"

// isMarketToolRequest reports whether a governance request is about a tool of the ZGI Market Skill.
func isMarketToolRequest(req ToolGovernanceRequest) bool {
	return normalizeSkillID(req.SkillID) == SkillZGIMarketTools && req.ProviderType == tools.ToolProviderTypeMarket
}

// explainMarketToolRefusal says in a person's words why a ZGI Market tool does not run where
// nobody can approve it, such as a digital human or a published app: it writes, or one call can
// cost more than the instance lets a tool spend unasked. Such a tool is refused with the code of a
// missing preauthorization, which says nothing to the end user it is shown to; the code stays in
// the decision for diagnostics, and the sentence becomes the reason and what the model is told to
// answer. Any other decision is returned as it is.
func explainMarketToolRefusal(req ToolGovernanceRequest, manifest toolgovernance.Manifest, decision toolgovernance.Decision) toolgovernance.Decision {
	if !isMarketToolRequest(req) || decision.Status != toolgovernance.DecisionStatusDenied ||
		decision.Preauthorization == nil || decision.Preauthorization.Code != agentToolNotPreauthorizedCode {
		return decision
	}
	messages := map[string]string{
		"zh-CN": "该工具会写入数据，需要人工确认，无人值守时不会执行",
		"en-US": "This tool writes data and needs a person's confirmation, so it does not run unattended",
	}
	if amounts := marketCostReasonPattern.FindStringSubmatch(strings.TrimSpace(manifest.ApprovalReason)); amounts != nil {
		messages = map[string]string{
			"zh-CN": "该工具单次最高 " + amounts[1] + " 点，超过确认阈值 " + amounts[2] + " 点，无人值守时不会执行",
			"en-US": "One call of this tool can cost up to " + amounts[1] + " points, above the confirmation threshold of " +
				amounts[2] + " points, so it does not run unattended",
		}
	}
	decision.Reason = messages["en-US"]
	if decision.ModelFeedback == nil {
		decision.ModelFeedback = map[string]interface{}{}
	}
	decision.ModelFeedback["reason"] = decision.Reason
	decision.ModelFeedback[marketToolUserMessageKey] = messages
	return decision
}

// marketToolUserMessages returns what the end user is told about a refused market tool, by
// language, or nil for a decision that carries no such message.
func marketToolUserMessages(decision toolgovernance.Decision) map[string]string {
	messages, _ := decision.ModelFeedback[marketToolUserMessageKey].(map[string]string)
	return messages
}

// modelFacingGovernanceFeedback is the governance feedback as the model reads it. For a refusal
// that carries a sentence for the end user, the internal authorization code and its source are
// left out, so the model has nothing to repeat but the sentence; the decision and the trace keep
// them.
func modelFacingGovernanceFeedback(decision toolgovernance.Decision) map[string]interface{} {
	result := governanceToolFeedback(decision)
	if marketToolUserMessages(decision) == nil {
		return result
	}
	if feedback, ok := result["governance"].(map[string]interface{}); ok {
		for _, key := range []string{"authorization_code", "authorization_source", "preauthorization", "asset_operation_audit"} {
			delete(feedback, key)
		}
	}
	return result
}
