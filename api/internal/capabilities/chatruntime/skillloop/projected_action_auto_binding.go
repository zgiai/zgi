package skillloop

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const projectedActionAutoBindingTraceSource = "server_projected_action_auto_binding"

// IsProjectedActionAutoBindingTrace reports whether trace is the internal,
// server-generated plan mutation used to bind a projected Action call. It is
// exported so the service persistence boundary can commit this trace exactly
// once instead of routing it through best-effort trace persistence.
func IsProjectedActionAutoBindingTrace(trace skills.SkillTrace) bool {
	return strings.EqualFold(strings.TrimSpace(trace.Kind), "plan_update") &&
		strings.EqualFold(strings.TrimSpace(trace.Status), "success") &&
		strings.TrimSpace(evidenceStringFromAny(trace.Arguments["source"])) == projectedActionAutoBindingTraceSource &&
		len(evidenceMapsFromAny(trace.Result["plan"])) > 0
}

func projectedExternalActionAutoBindingTraceSnapshot(trace skills.SkillTrace) skills.SkillTrace {
	cloned := trace
	cloned.Arguments = projectedExternalActionAutoBindingCloneMap(trace.Arguments)
	cloned.Result = projectedExternalActionAutoBindingCloneMap(trace.Result)
	return cloned
}

// projectedExternalActionAutoBindCall lazily creates or reuses the
// server-owned Outcome phase for one direct projected Action call. The model
// chooses only the exposed business function and its business arguments; every
// execution-ledger field returned here is derived from the current server
// projection and runtime state.
func projectedExternalActionAutoBindCall(
	modelCall adapter.ToolCall,
	executionCall adapter.ToolCall,
	toolSet *skills.NativeToolSet,
	runtimeState map[string]interface{},
	round int,
	claimedRootPhaseIDs map[string]struct{},
) (skills.SkillTrace, string) {
	binding, alias, ok := nativeToolBindingForCall(toolSet, modelCall.Function.Name)
	if !ok {
		return skills.SkillTrace{}, ""
	}
	calledIntegrationID, calledActionID, projected := nativeProjectedExternalActionIdentity(binding)
	if !projected || strings.TrimSpace(binding.BindingFingerprint) == "" {
		return skills.SkillTrace{}, ""
	}
	calledCandidate := projectedExternalActionCandidateByFingerprint(runtimeState, binding.BindingFingerprint)
	if !projectedExternalActionAutoBindingCandidateMatches(
		calledCandidate,
		binding,
		calledIntegrationID,
		calledActionID,
	) {
		return skills.SkillTrace{}, ""
	}

	rootBinding, rootAlias, ok := projectedExternalActionAuthoritativeRootBinding(toolSet, alias, binding)
	if !ok {
		return skills.SkillTrace{}, ""
	}
	rootIntegrationID, rootActionID, projected := nativeProjectedExternalActionIdentity(rootBinding)
	if !projected || strings.TrimSpace(rootBinding.BindingFingerprint) == "" {
		return skills.SkillTrace{}, ""
	}
	rootCandidate := projectedExternalActionCandidateByFingerprint(runtimeState, rootBinding.BindingFingerprint)
	if !projectedExternalActionAutoBindingCandidateMatches(rootCandidate, rootBinding, rootIntegrationID, rootActionID) {
		return skills.SkillTrace{}, ""
	}

	wrapper, err := skills.ParseArguments(executionCall.Function.Arguments)
	if err != nil ||
		!strings.EqualFold(strings.TrimSpace(stringArg(wrapper, "skill_id")), skills.SkillExternalApps) ||
		!strings.EqualFold(strings.TrimSpace(stringArg(wrapper, "tool_name")), "execute_action") {
		return skills.SkillTrace{}, ""
	}
	toolArguments := mapArg(wrapper, "arguments")
	if !strings.EqualFold(operationPlanSkillCallTargetValue(toolArguments, "integration_id"), calledIntegrationID) ||
		!strings.EqualFold(operationPlanSkillCallTargetValue(toolArguments, "action_id"), calledActionID) {
		return skills.SkillTrace{}, ""
	}

	directRootCall := strings.EqualFold(calledIntegrationID, rootIntegrationID) &&
		strings.EqualFold(calledActionID, rootActionID)
	requestedPhaseID := projectedExternalActionRequestedPlanPhase(modelCall, binding)
	targetArguments := map[string]interface{}{}
	if directRootCall {
		targetArguments = projectedExternalActionAutoBindingTargetArguments(rootBinding, toolArguments)
		if !projectedExternalActionAutoBindingTargetComplete(rootCandidate, targetArguments) {
			// A mutation without its complete stable target cannot acquire a
			// canonical phase merely by being called. A declared preparation
			// Action may establish the unresolved parent phase first.
			return skills.SkillTrace{}, ""
		}
	}

	plan := projectedExternalActionAutoBindingCloneMap(evidenceMapFromAny(runtimeState["operation_plan"]))
	if plan == nil {
		plan = map[string]interface{}{}
	}
	phases := projectedExternalActionAutoBindingCloneMaps(plan["phases"])
	outcomes := projectedExternalActionAutoBindingCloneMaps(plan["outcomes"])

	phaseIndex := projectedExternalActionAutoBindingExistingPhase(
		phases,
		runtimeState,
		rootBinding.BindingFingerprint,
		targetArguments,
		directRootCall,
		requestedPhaseID,
		claimedRootPhaseIDs,
	)
	if phaseIndex < 0 {
		var ambiguousServerOutcome bool
		phaseIndex, ambiguousServerOutcome = projectedExternalActionAutoBindingServerOutcomePhase(phases, runtimeState, rootCandidate)
		if ambiguousServerOutcome {
			// Multiple server Outcomes in the same intent group represent
			// independently verifiable operations. Reusing an arbitrary pending
			// Outcome would collapse their operation identities, while minting a
			// third Outcome would silently expand the user's request. The service
			// must advance exactly one of them before this call can be bound.
			return skills.SkillTrace{}, ""
		}
	}
	if phaseIndex < 0 && directRootCall &&
		projectedExternalActionEffectRequiresBoundTarget(evidenceStringFromAny(rootCandidate["effect"])) &&
		projectedExternalActionAutoBindingAlreadyConfirmed(phases, runtimeState, rootBinding.BindingFingerprint, targetArguments) {
		// A completed provider-confirmed mutation cannot be turned into a new
		// operation by adding a model advisory phase for the same Action/target.
		// A genuinely distinct repeated write must already have its own open
		// server Outcome phase; otherwise fail closed before Provider execution.
		return skills.SkillTrace{}, ""
	}
	outcomesChanged := false
	if phaseIndex < 0 {
		if directRootCall &&
			projectedExternalActionEffectRequiresBoundTarget(evidenceStringFromAny(rootCandidate["effect"])) &&
			projectedExternalActionAutoBindingHasConfirmedMutation(runtimeState, rootIntegrationID, rootActionID, targetArguments) {
			// A fresh phase would mint a fresh operation identity and could replay a
			// completed non-idempotent mutation. Only an already-open independent
			// server Outcome may authorize another same-target write.
			return skills.SkillTrace{}, ""
		}
		if len(phases) >= maxPlanPhases || len(outcomes) >= maxPlanPhases {
			return skills.SkillTrace{}, ""
		}
		outcomeID := "outcome-external-" + uuid.NewString()
		phaseID := "phase-external-" + uuid.NewString()
		goal := "Complete the requested " + rootIntegrationID + "/" + rootActionID + " Action"
		outcomes = append(outcomes, map[string]interface{}{
			"id":           outcomeID,
			"goal":         goal,
			"title":        goal,
			"status":       "pending",
			"required":     true,
			"capabilities": []interface{}{skills.SkillExternalApps},
		})
		phases = append(phases, map[string]interface{}{
			"id":                        phaseID,
			"outcome_id":                outcomeID,
			"step":                      goal,
			"title":                     goal,
			"status":                    "pending",
			"required":                  true,
			"completion_source":         "runtime_effect_reconciliation",
			operationPlanPhaseSourceKey: operationPlanPhaseSourceServerOutcome,
		})
		phaseIndex = len(phases) - 1
		outcomesChanged = true
	}

	phaseBefore := projectedExternalActionAutoBindingCloneMap(phases[phaseIndex])
	expectedBefore := evidenceMapFromAny(phaseBefore["expected_action"])
	expected := projectedExternalActionAutoBindingExpectedAction(
		rootAlias,
		rootBinding,
		rootIntegrationID,
		rootActionID,
		targetArguments,
	)
	if len(expectedBefore) > 0 {
		if !projectedExternalActionAutoBindingExpectedIdentityMatches(expectedBefore, expected) {
			return skills.SkillTrace{}, ""
		}
		if !directRootCall && len(evidenceMapFromAny(expectedBefore["target_arguments"])) > 0 {
			expected["target_arguments"] = projectedExternalActionAutoBindingCloneMap(
				evidenceMapFromAny(expectedBefore["target_arguments"]),
			)
		}
	}
	phases[phaseIndex]["expected_action"] = expected
	phases[phaseIndex][operationPlanPhaseSourceKey] = operationPlanPhaseSourceServerOutcome
	phases[phaseIndex]["required"] = true
	phases[phaseIndex][operationPlanServerProjectedConnectionBindingKey] =
		projectedExternalActionAutoBindingConnectionBinding(rootBinding)

	targetChanged := !projectedExternalActionAutoBindingTargetsEqual(
		evidenceMapFromAny(expectedBefore["target_arguments"]),
		evidenceMapFromAny(expected["target_arguments"]),
	)
	ledgerEpoch := strings.TrimSpace(evidenceStringFromAny(phases[phaseIndex][operationPlanServerProjectedLedgerEpochKey]))
	if ledgerEpoch == "" || targetChanged {
		phases[phaseIndex][operationPlanServerProjectedLedgerEpochKey] = uuid.NewString()
	}

	if len(expectedBefore) > 0 && targetChanged {
		if _, validationErr := validateProjectedExternalActionObservedTargetBindings(phases, runtimeState); validationErr != nil {
			phases[phaseIndex] = phaseBefore
			return skills.SkillTrace{}, ""
		}
	}
	phaseID := strings.TrimSpace(evidenceStringFromAny(phases[phaseIndex]["id"]))
	if phaseID == "" {
		return skills.SkillTrace{}, ""
	}
	if directRootCall && claimedRootPhaseIDs != nil {
		claimedRootPhaseIDs[strings.ToLower(phaseID)] = struct{}{}
	}

	changed := outcomesChanged || len(expectedBefore) == 0 || targetChanged ||
		!strings.EqualFold(
			strings.TrimSpace(evidenceStringFromAny(expectedBefore[planExpectedActionServerProjectionKey])),
			strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerProjectionKey])),
		) ||
		!strings.EqualFold(
			strings.TrimSpace(evidenceStringFromAny(expectedBefore["projected_tool_name"])),
			strings.TrimSpace(evidenceStringFromAny(expected["projected_tool_name"])),
		) ||
		strings.TrimSpace(evidenceStringFromAny(phaseBefore[operationPlanServerProjectedLedgerEpochKey])) == "" ||
		strings.TrimSpace(evidenceStringFromAny(phaseBefore[operationPlanServerProjectedConnectionBindingKey])) !=
			projectedExternalActionAutoBindingConnectionBinding(rootBinding) ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phaseBefore[operationPlanPhaseSourceKey])), operationPlanPhaseSourceServerOutcome)
	if !changed {
		return skills.SkillTrace{}, phaseID
	}
	result := map[string]interface{}{"plan": phases}
	if outcomesChanged {
		result["outcomes"] = outcomes
	}
	trace := skills.SkillTrace{
		Kind:     "plan_update",
		SkillID:  skills.SkillExternalApps,
		ToolName: skills.MetaToolUpdatePlan,
		Status:   "success",
		Arguments: map[string]interface{}{
			"source":   projectedActionAutoBindingTraceSource,
			"round":    round,
			"call_id":  strings.TrimSpace(modelCall.ID),
			"phase_id": phaseID,
		},
		Result: result,
	}
	return trace, phaseID
}

func projectedExternalActionAutoBindingHasConfirmedMutation(
	runtimeState map[string]interface{},
	integrationID string,
	actionID string,
	targetArguments map[string]interface{},
) bool {
	actionKey := strings.ToLower(strings.TrimSpace(integrationID)) + ":" + strings.ToLower(strings.TrimSpace(actionID))
	targetKey := terminalStateGuardExternalTargetKey(targetArguments)
	if actionKey == ":" || targetKey == "" {
		return false
	}
	for _, invocation := range terminalStateGuardExternalInvocations(runtimeState) {
		if !terminalStateGuardInvocationIsExecutionEvidence(invocation) ||
			!terminalStateGuardExternalActionProviderConfirmed(invocation) ||
			terminalStateGuardExternalActionKey(invocation) != actionKey ||
			terminalStateGuardExternalActionTargetKey(invocation) != targetKey {
			continue
		}
		return true
	}
	return false
}

func projectedExternalActionAuthoritativeRootBinding(
	toolSet *skills.NativeToolSet,
	calledAlias string,
	called skills.NativeToolBinding,
) (skills.NativeToolBinding, string, bool) {
	if !projectedExternalActionToolIsExposed(toolSet, calledAlias) {
		return skills.NativeToolBinding{}, "", false
	}
	if called.IntentMatched || called.Pinned {
		return called, calledAlias, true
	}
	calledIntegrationID, calledActionID, ok := nativeProjectedExternalActionIdentity(called)
	if !ok {
		return skills.NativeToolBinding{}, "", false
	}
	calledConnectionBinding := projectedExternalActionAutoBindingConnectionBinding(called)
	var matched skills.NativeToolBinding
	matchedAlias := ""
	for alias, candidate := range toolSet.ToolBindings {
		if !projectedExternalActionToolIsExposed(toolSet, alias) {
			continue
		}
		if !candidate.IntentMatched && !candidate.Pinned {
			continue
		}
		integrationID, _, projected := nativeProjectedExternalActionIdentity(candidate)
		if !projected || !strings.EqualFold(integrationID, calledIntegrationID) ||
			calledConnectionBinding == "" || projectedExternalActionAutoBindingConnectionBinding(candidate) != calledConnectionBinding {
			continue
		}
		declared := false
		for _, hint := range candidate.PreparationHints {
			if strings.EqualFold(strings.TrimSpace(hint.ActionID), calledActionID) &&
				strings.EqualFold(strings.TrimSpace(hint.Relation), "resolve_target") &&
				len(hint.TargetArguments) > 0 && len(hint.ResultPaths) > 0 {
				declared = true
				break
			}
		}
		if !declared {
			continue
		}
		if matchedAlias != "" {
			return skills.NativeToolBinding{}, "", false
		}
		matched = candidate
		matchedAlias = alias
	}
	if matchedAlias != "" {
		return matched, matchedAlias, true
	}
	// IntentMatched and Pinned describe which requested outcomes the server
	// inferred before execution. They are not an authorization boundary for a
	// business function that the same request-scoped server projection exposed
	// to the model. When no authoritative preparation relationship redirects the
	// call to a parent Action, bind the directly selected Action to its own
	// server Outcome. Exact candidate, fingerprint, connection, durable ledger,
	// policy, approval, schema, and Provider checks remain downstream gates.
	return called, calledAlias, true
}

func projectedExternalActionToolIsExposed(toolSet *skills.NativeToolSet, alias string) bool {
	if toolSet == nil || strings.TrimSpace(alias) == "" {
		return false
	}
	for _, tool := range toolSet.ProviderTools {
		if strings.EqualFold(strings.TrimSpace(tool.Function.Name), strings.TrimSpace(alias)) {
			return true
		}
	}
	return false
}

func projectedExternalActionAutoBindingCandidateMatches(
	candidate map[string]interface{},
	binding skills.NativeToolBinding,
	integrationID string,
	actionID string,
) bool {
	if len(candidate) == 0 ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(candidate["integration_id"])), integrationID) ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(candidate["action_id"])), actionID) ||
		strings.TrimSpace(evidenceStringFromAny(candidate["binding_fingerprint"])) != strings.TrimSpace(binding.BindingFingerprint) {
		return false
	}
	wantConnection := projectedExternalActionAutoBindingConnectionBinding(binding)
	return wantConnection != "" && wantConnection == strings.TrimSpace(evidenceStringFromAny(
		candidate[operationPlanServerProjectedConnectionBindingKey],
	))
}

func projectedExternalActionAutoBindingConnectionBinding(binding skills.NativeToolBinding) string {
	if value := strings.TrimSpace(binding.ConnectionBinding); value != "" {
		return value
	}
	return skills.NativeExternalActionConnectionBindingHash(evidenceStringFromAny(binding.FixedArguments["connection_id"]))
}

func projectedExternalActionAutoBindingTargetArguments(
	binding skills.NativeToolBinding,
	toolArguments map[string]interface{},
) map[string]interface{} {
	businessArguments := evidenceMapFromAny(toolArguments["arguments"])
	out := map[string]interface{}{}
	for _, path := range binding.TargetArgumentPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		value := operationPlanArgumentPathValue(businessArguments, path)
		if value == "" {
			value = strings.TrimSpace(evidenceStringFromAny(businessArguments[path]))
		}
		if value != "" {
			out[path] = value
		}
	}
	return nativeCanonicalizeProjectedActionArguments(out, binding.OptionalTargets)
}

func projectedExternalActionAutoBindingTargetComplete(candidate map[string]interface{}, target map[string]interface{}) bool {
	if !projectedExternalActionEffectRequiresBoundTarget(evidenceStringFromAny(candidate["effect"])) {
		return true
	}
	for _, path := range evidenceStringSliceFromAny(candidate["target_argument_paths"]) {
		path = strings.TrimSpace(path)
		if path == "" || projectedExternalExpectedTargetArgumentValue(target, path) != "" ||
			projectedExternalTargetPathConditionallyOptional(candidate, target, path) {
			continue
		}
		return false
	}
	return true
}

func projectedExternalActionAutoBindingExistingPhase(
	phases []map[string]interface{},
	runtimeState map[string]interface{},
	fingerprint string,
	target map[string]interface{},
	directRootCall bool,
	requestedPhaseID string,
	claimedRootPhaseIDs map[string]struct{},
) int {
	requestedPhaseID = strings.TrimSpace(requestedPhaseID)
	match := -1
	for index, phase := range phases {
		if !operationPlanPhaseOpenForToolCall(phase) || !operationPlanPhaseHasServerAuthority(phase, runtimeState) {
			continue
		}
		if directRootCall {
			if _, claimed := claimedRootPhaseIDs[strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["id"])))]; claimed {
				continue
			}
		}
		if requestedPhaseID != "" && !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), requestedPhaseID) {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		if strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey])) != strings.TrimSpace(fingerprint) {
			continue
		}
		existingTarget := evidenceMapFromAny(expected["target_arguments"])
		if directRootCall && len(existingTarget) > 0 && !projectedExternalActionAutoBindingTargetsEqual(existingTarget, target) {
			continue
		}
		if match >= 0 {
			return -1
		}
		match = index
	}
	return match
}

func projectedExternalActionAutoBindingAlreadyConfirmed(
	phases []map[string]interface{},
	runtimeState map[string]interface{},
	fingerprint string,
	target map[string]interface{},
) bool {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false
	}
	for _, phase := range phases {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["status"])), "completed") ||
			!operationPlanPhaseHasServerAuthority(phase, runtimeState) {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		if strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey])) != fingerprint ||
			!projectedExternalActionAutoBindingTargetsEqual(evidenceMapFromAny(expected["target_arguments"]), target) {
			continue
		}
		if providerConfirmedExternalPhaseSatisfied(phase, runtimeState) {
			return true
		}
	}
	return false
}

func projectedExternalActionRequestedPlanPhase(call adapter.ToolCall, binding skills.NativeToolBinding) string {
	argumentName := strings.TrimSpace(binding.PlanPhaseArgument)
	if argumentName == "" {
		return ""
	}
	arguments, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(evidenceStringFromAny(arguments[argumentName]))
}

func projectedExternalActionAutoBindingServerOutcomePhase(
	phases []map[string]interface{},
	runtimeState map[string]interface{},
	rootCandidate map[string]interface{},
) (int, bool) {
	groupID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(rootCandidate["intent_group"])))
	if groupID == "" {
		return -1, false
	}
	matches := make(map[int]struct{})
	for _, requirement := range projectedExternalActionRequirementsForPhases(phases, runtimeState) {
		if strings.TrimSpace(requirement.phaseID) == "" || requirement.group.id != groupID {
			continue
		}
		for index, phase := range phases {
			if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), requirement.phaseID) ||
				!operationPlanPhaseOpenForToolCall(phase) ||
				len(evidenceMapFromAny(phase["expected_action"])) > 0 ||
				!projectedExternalActionAutoBindingPhaseIsServerOutcome(phase, runtimeState) {
				continue
			}
			matches[index] = struct{}{}
		}
	}
	if len(matches) == 0 {
		return -1, false
	}
	active := -1
	for index := range matches {
		switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phases[index]["status"]))) {
		case "in_progress", "running":
			if active >= 0 && active != index {
				return -1, true
			}
			active = index
		}
	}
	if active >= 0 {
		return active, false
	}
	if len(matches) == 1 {
		for index := range matches {
			return index, false
		}
	}
	return -1, true
}

func projectedExternalActionAutoBindingPhaseIsServerOutcome(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	return operationPlanPhaseHasServerAuthority(phase, runtimeState) &&
		(strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase[operationPlanPhaseSourceKey])), operationPlanPhaseSourceServerOutcome) ||
			len(operationPlanOutcomeForPhase(phase, runtimeState)) > 0)
}

func projectedExternalActionAutoBindingExpectedAction(
	alias string,
	binding skills.NativeToolBinding,
	integrationID string,
	actionID string,
	targetArguments map[string]interface{},
) map[string]interface{} {
	expected := map[string]interface{}{
		"skill_id":                            skills.SkillExternalApps,
		"tool_name":                           "execute_action",
		"projected_tool_name":                 strings.TrimSpace(alias),
		planExpectedActionServerProjectionKey: strings.TrimSpace(alias),
		planExpectedActionServerBindingFingerprintKey: strings.TrimSpace(binding.BindingFingerprint),
		"target": map[string]interface{}{
			"integration_id": strings.ToLower(strings.TrimSpace(integrationID)),
			"action_id":      strings.ToLower(strings.TrimSpace(actionID)),
		},
	}
	if len(targetArguments) > 0 {
		expected["target_arguments"] = projectedExternalActionAutoBindingCloneMap(targetArguments)
	}
	return expected
}

func projectedExternalActionAutoBindingExpectedIdentityMatches(left map[string]interface{}, right map[string]interface{}) bool {
	leftKey, leftExternal := projectedExternalActionKeyFromExpected(left)
	rightKey, rightExternal := projectedExternalActionKeyFromExpected(right)
	return leftExternal && rightExternal && leftKey != "" && leftKey == rightKey &&
		strings.TrimSpace(evidenceStringFromAny(left[planExpectedActionServerBindingFingerprintKey])) ==
			strings.TrimSpace(evidenceStringFromAny(right[planExpectedActionServerBindingFingerprintKey]))
}

func projectedExternalActionAutoBindingTargetsEqual(left map[string]interface{}, right map[string]interface{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if strings.TrimSpace(evidenceStringFromAny(leftValue)) !=
			strings.TrimSpace(evidenceStringFromAny(right[key])) {
			return false
		}
	}
	return true
}

// projectedExternalActionCommittedBindingMatchesCall validates the durable
// snapshot against both the binding generated for this call and the current
// server projection. Merely observing some phase fingerprint or epoch is not
// enough: source, Action identity, connection, target, and the exact generated
// epoch must all survive the commit.
func projectedExternalActionCommittedBindingMatchesCall(
	modelCall adapter.ToolCall,
	executionCall adapter.ToolCall,
	toolSet *skills.NativeToolSet,
	runtimeState map[string]interface{},
	phaseID string,
	bindingTrace skills.SkillTrace,
) bool {
	phaseID = strings.TrimSpace(phaseID)
	if phaseID == "" {
		return false
	}
	called, alias, ok := nativeToolBindingForCall(toolSet, modelCall.Function.Name)
	if !ok {
		return false
	}
	calledIntegrationID, calledActionID, projected := nativeProjectedExternalActionIdentity(called)
	if !projected || strings.TrimSpace(called.BindingFingerprint) == "" {
		return false
	}
	calledCandidate := projectedExternalActionCandidateByFingerprint(runtimeState, called.BindingFingerprint)
	if !projectedExternalActionAutoBindingCandidateMatches(
		calledCandidate,
		called,
		calledIntegrationID,
		calledActionID,
	) {
		return false
	}
	root, rootAlias, ok := projectedExternalActionAuthoritativeRootBinding(toolSet, alias, called)
	if !ok {
		return false
	}
	rootIntegrationID, rootActionID, projected := nativeProjectedExternalActionIdentity(root)
	if !projected || strings.TrimSpace(root.BindingFingerprint) == "" {
		return false
	}
	candidate := projectedExternalActionCandidateByFingerprint(runtimeState, root.BindingFingerprint)
	if !projectedExternalActionAutoBindingCandidateMatches(candidate, root, rootIntegrationID, rootActionID) {
		return false
	}

	wrapper, err := skills.ParseArguments(executionCall.Function.Arguments)
	if err != nil ||
		!strings.EqualFold(strings.TrimSpace(stringArg(wrapper, "skill_id")), skills.SkillExternalApps) ||
		!strings.EqualFold(strings.TrimSpace(stringArg(wrapper, "tool_name")), "execute_action") {
		return false
	}
	toolArguments := mapArg(wrapper, "arguments")
	if !strings.EqualFold(operationPlanSkillCallTargetValue(toolArguments, "integration_id"), calledIntegrationID) ||
		!strings.EqualFold(operationPlanSkillCallTargetValue(toolArguments, "action_id"), calledActionID) {
		return false
	}

	var persisted map[string]interface{}
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), phaseID) {
			persisted = phase
			break
		}
	}
	if len(persisted) == 0 || !operationPlanPhaseOpenForToolCall(persisted) ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(persisted[operationPlanPhaseSourceKey])), operationPlanPhaseSourceServerOutcome) ||
		len(operationPlanOutcomeForPhase(persisted, runtimeState)) == 0 {
		return false
	}
	persistedExpected := evidenceMapFromAny(persisted["expected_action"])
	wantExpected := projectedExternalActionAutoBindingExpectedAction(rootAlias, root, rootIntegrationID, rootActionID, nil)
	if !projectedExternalActionAutoBindingExpectedIdentityMatches(persistedExpected, wantExpected) ||
		strings.TrimSpace(evidenceStringFromAny(persistedExpected[planExpectedActionServerProjectionKey])) != strings.TrimSpace(rootAlias) ||
		strings.TrimSpace(evidenceStringFromAny(persistedExpected["projected_tool_name"])) != strings.TrimSpace(rootAlias) ||
		strings.TrimSpace(evidenceStringFromAny(persisted[operationPlanServerProjectedConnectionBindingKey])) != projectedExternalActionAutoBindingConnectionBinding(root) {
		return false
	}
	persistedEpoch := strings.TrimSpace(evidenceStringFromAny(persisted[operationPlanServerProjectedLedgerEpochKey]))
	if persistedEpoch == "" {
		return false
	}

	directRootCall := strings.EqualFold(calledIntegrationID, rootIntegrationID) &&
		strings.EqualFold(calledActionID, rootActionID)
	if directRootCall {
		target := projectedExternalActionAutoBindingTargetArguments(root, toolArguments)
		if !projectedExternalActionAutoBindingTargetComplete(candidate, target) ||
			!projectedExternalActionAutoBindingTargetsEqual(evidenceMapFromAny(persistedExpected["target_arguments"]), target) {
			return false
		}
	}

	if strings.TrimSpace(bindingTrace.Kind) == "" {
		return true
	}
	if !IsProjectedActionAutoBindingTrace(bindingTrace) {
		return false
	}
	var proposed map[string]interface{}
	for _, phase := range evidenceMapsFromAny(bindingTrace.Result["plan"]) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), phaseID) {
			proposed = phase
			break
		}
	}
	if len(proposed) == 0 ||
		strings.TrimSpace(evidenceStringFromAny(proposed[operationPlanServerProjectedLedgerEpochKey])) != persistedEpoch ||
		strings.TrimSpace(evidenceStringFromAny(proposed[operationPlanServerProjectedConnectionBindingKey])) !=
			strings.TrimSpace(evidenceStringFromAny(persisted[operationPlanServerProjectedConnectionBindingKey])) {
		return false
	}
	proposedExpected := evidenceMapFromAny(proposed["expected_action"])
	return projectedExternalActionAutoBindingExpectedIdentityMatches(proposedExpected, persistedExpected) &&
		projectedExternalActionAutoBindingTargetsEqual(
			evidenceMapFromAny(proposedExpected["target_arguments"]),
			evidenceMapFromAny(persistedExpected["target_arguments"]),
		)
}

func projectedExternalActionInjectPlanPhase(call adapter.ToolCall, phaseID string) adapter.ToolCall {
	phaseID = strings.TrimSpace(phaseID)
	if phaseID == "" {
		return call
	}
	arguments, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		return call
	}
	arguments["plan_phase_id"] = phaseID
	encoded, err := json.Marshal(arguments)
	if err == nil {
		call.Function.Arguments = string(encoded)
	}
	return call
}

func projectedExternalActionAutoBindingCloneMaps(value interface{}) []map[string]interface{} {
	raw := evidenceMapsFromAny(value)
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		out = append(out, projectedExternalActionAutoBindingCloneMap(item))
	}
	return out
}

func projectedExternalActionAutoBindingCloneMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return copyStringAnyMap(value)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return copyStringAnyMap(value)
	}
	return out
}
