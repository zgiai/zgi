package skillloop

import (
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type finalAnswerSubmission struct {
	answer      string
	explanation string
	plan        []map[string]interface{}
	streamed    bool
	planWarning string
}

func finalAnswerCall(calls []adapter.ToolCall) (adapter.ToolCall, bool) {
	for _, call := range calls {
		if strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolFinalAnswer) {
			return call, true
		}
	}
	return adapter.ToolCall{}, false
}

func parseFinalAnswerSubmission(call adapter.ToolCall, evidence map[string]interface{}) (finalAnswerSubmission, error) {
	requirePlan := finalAnswerPlanSnapshotRequired(evidence)
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		answer, complete := partialJSONStringField(call.Function.Arguments, "answer")
		answer = strings.TrimSpace(answer)
		if !complete || answer == "" {
			return finalAnswerSubmission{}, fmt.Errorf("%w: submit_final_answer arguments are invalid: %v", ErrInvalidInput, err)
		}
		warning := "submit_final_answer optional metadata was invalid and ignored: " + err.Error()
		if requirePlan {
			warning = "missing_or_invalid_final_plan_snapshot: " + err.Error()
		}
		return finalAnswerSubmission{answer: answer, planWarning: trimRunes(warning, 500)}, nil
	}
	answer := strings.TrimSpace(stringArg(args, "answer"))
	if answer == "" {
		return finalAnswerSubmission{}, fmt.Errorf("%w: submit_final_answer answer is required", ErrInvalidInput)
	}

	submission := finalAnswerSubmission{
		answer:      answer,
		explanation: trimRunes(stringArg(args, "explanation"), 500),
		streamed:    boolArg(args, streamedFinalAnswerArg),
	}
	if _, exists := args["plan"]; !exists {
		if requirePlan {
			submission.planWarning = "missing_or_invalid_final_plan_snapshot: plan is required when operation_plan.phases is non-empty"
		}
		return submission, nil
	}
	phases, err := normalizePlanSnapshot(args["plan"])
	if err != nil {
		warning := "submit_final_answer optional plan was ignored: " + err.Error()
		if requirePlan {
			warning = "missing_or_invalid_final_plan_snapshot: " + err.Error()
		}
		submission.planWarning = trimRunes(warning, 500)
		return submission, nil
	}
	submission.plan = phases
	return submission, nil
}

func finalAnswerPlanSnapshotRequired(evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	return len(mapSliceFromAny(plan["phases"])) > 0
}

func finalAnswerSkillStep(callID string, submission finalAnswerSubmission) skillStepResult {
	result := map[string]interface{}{
		"status":       "submitted",
		"answer_chars": len([]rune(submission.answer)),
	}
	if len(submission.plan) > 0 {
		result["plan"] = submission.plan
	}
	if submission.explanation != "" {
		result["explanation"] = submission.explanation
	}
	if submission.planWarning != "" {
		result["plan_warning"] = trimRunes(submission.planWarning, 500)
	}
	trace := skills.SkillTrace{
		Kind:     "final_answer",
		ToolName: skills.MetaToolFinalAnswer,
		Status:   "success",
		Arguments: map[string]interface{}{
			"answer_chars": len([]rune(submission.answer)),
			"phase_count":  len(submission.plan),
		},
		Result: result,
	}
	step := terminalSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status": "accepted",
	}), false, false)
	step.answer = submission.answer
	step.answerStreamed = submission.streamed
	return step
}

func failedFinalAnswerSkillStep(callID string, err error, nextAction string) skillStepResult {
	trace := failedSkillTrace("final_answer", skills.MetaToolFinalAnswer, err)
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, nextAction)), false, false)
}
