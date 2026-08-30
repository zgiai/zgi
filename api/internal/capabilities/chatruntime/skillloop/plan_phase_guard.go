package skillloop

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const operationPlanPhaseMismatchCode = "operation_plan_phase_mismatch"
const projectedExternalActionPlanIncompleteCode = "projected_external_action_plan_incomplete"
const projectedExternalMutationTargetMissingIssue = "external mutation expected_action is missing a required server-bound target argument"
const projectedExternalObservedPreparationTargetsKey = "_server_observed_preparation_targets"
const operationPlanServerTargetKey = "_server_operation_plan_target"
const operationPlanServerProjectedLedgerEpochKey = "_server_projected_ledger_epoch"
const operationPlanServerProjectedConnectionBindingKey = "_server_projected_connection_binding"

func operationPlanProjectedLedgerEpoch(runtimeState map[string]interface{}, phaseID string) string {
	phaseID = strings.TrimSpace(phaseID)
	if phaseID == "" {
		return ""
	}
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), phaseID) {
			return strings.TrimSpace(evidenceStringFromAny(phase[operationPlanServerProjectedLedgerEpochKey]))
		}
	}
	return ""
}

func operationPlanProjectedBindingFingerprint(runtimeState map[string]interface{}, phaseID string) string {
	phaseID = strings.TrimSpace(phaseID)
	if phaseID == "" {
		return ""
	}
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), phaseID) {
			return strings.TrimSpace(evidenceStringFromAny(evidenceMapFromAny(phase["expected_action"])[planExpectedActionServerBindingFingerprintKey]))
		}
	}
	return ""
}

func projectedExternalActionPhaseOperationItemID(
	runtimeState map[string]interface{},
	phaseID string,
	skillID string,
	toolName string,
	arguments map[string]interface{},
) string {
	actionKey := projectedExternalActionKeyFromCall(skillID, toolName, arguments)
	if actionKey == "" || strings.TrimSpace(phaseID) == "" {
		return ""
	}
	if _, projected := projectedExternalActionKeys(runtimeState)[actionKey]; !projected {
		return ""
	}
	canonicalPhaseID := ""
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), strings.TrimSpace(phaseID)) {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		expectedKey, external := projectedExternalActionKeyFromExpected(expected)
		if !external || expectedKey != actionKey || !operationPlanExpectedActionMatchesSkillCall(expected, skillID, toolName, arguments) {
			return ""
		}
		canonicalPhaseID = strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		break
	}
	if canonicalPhaseID == "" {
		return ""
	}
	parts := strings.SplitN(actionKey, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	bindingFingerprint := operationPlanProjectedBindingFingerprint(runtimeState, canonicalPhaseID)
	candidate := projectedExternalActionCandidateByFingerprint(runtimeState, bindingFingerprint)
	connectionID := operationPlanSkillCallTargetValue(arguments, "connection_id")
	if len(candidate) == 0 ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(candidate["integration_id"])), parts[0]) ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(candidate["action_id"])), parts[1]) ||
		strings.TrimSpace(evidenceStringFromAny(candidate[operationPlanServerProjectedConnectionBindingKey])) !=
			skills.NativeExternalActionConnectionBindingHash(connectionID) {
		return ""
	}
	return skills.ProjectedExternalActionOperationItemID(
		canonicalPhaseID,
		operationPlanProjectedLedgerEpoch(runtimeState, canonicalPhaseID),
		bindingFingerprint,
		parts[0],
		parts[1],
		connectionID,
	)
}

func resolveOperationPlanPhaseForSkillCall(
	runtimeState map[string]interface{},
	requestedPhaseID string,
	skillID string,
	toolName string,
	arguments map[string]interface{},
) (string, bool, error) {
	plan := evidenceMapFromAny(runtimeState["operation_plan"])
	phases := evidenceMapsFromAny(plan["phases"])
	if len(phases) == 0 {
		return strings.TrimSpace(requestedPhaseID), false, nil
	}

	requestedPhaseID = strings.TrimSpace(requestedPhaseID)
	if requestedPhaseID != "" {
		for _, phase := range phases {
			if !operationPlanPhaseOpenForToolCall(phase) {
				continue
			}
			phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
			if strings.EqualFold(phaseID, requestedPhaseID) {
				// A phase ID is an evidence-association hint for ordinary tools, not
				// an authorization boundary. The runtime records the exact call and
				// only completes the phase after a successful matching outcome. Tool
				// Governance separately freezes side-effecting calls that need exact
				// approval binding.
				return phaseID, true, nil
			}
		}
		// Stale or unknown phase IDs must not block a safe prerequisite call.
		// Drop the association and keep the concrete execution in the ledger.
		return "", false, nil
	}

	matches := make([]string, 0, 1)
	for _, phase := range phases {
		if !operationPlanPhaseOpenForToolCall(phase) {
			continue
		}
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		expected := evidenceMapFromAny(phase["expected_action"])
		if len(expected) == 0 {
			continue
		}
		if operationPlanExpectedActionMatchesSkillCall(expected, skillID, toolName, arguments) {
			matches = append(matches, phaseID)
		}
	}
	if len(matches) == 1 && strings.TrimSpace(matches[0]) != "" {
		return matches[0], true, nil
	}
	actionKey := projectedExternalActionKeyFromCall(skillID, toolName, arguments)
	if _, projected := projectedExternalActionKeys(runtimeState)[actionKey]; projected &&
		strings.EqualFold(projectedExternalActionEffect(runtimeState, actionKey), "read") {
		preparationMatches := projectedExternalActionPreparationPhaseMatches(runtimeState, actionKey)
		if len(preparationMatches) == 1 && strings.TrimSpace(preparationMatches[0]) != "" {
			return preparationMatches[0], true, nil
		}
	}
	return "", false, nil
}

type projectedExternalActionCandidateGroup struct {
	id         string
	tokens     []string
	candidates []map[string]interface{}
}

type projectedExternalActionRequirement struct {
	phaseID string
	group   projectedExternalActionCandidateGroup
}

func projectedExternalActionIntentGroups(runtimeState map[string]interface{}) []projectedExternalActionCandidateGroup {
	byID := map[string]*projectedExternalActionCandidateGroup{}
	for _, candidate := range evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionCandidatesKey]) {
		matched, _ := candidate["intent_matched"].(bool)
		groupID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(candidate["intent_group"])))
		if !matched || groupID == "" {
			continue
		}
		group := byID[groupID]
		if group == nil {
			group = &projectedExternalActionCandidateGroup{id: groupID}
			byID[groupID] = group
		}
		group.candidates = append(group.candidates, candidate)
		group.tokens = appendUniqueStringsFold(group.tokens, evidenceStringSliceFromAny(candidate["intent_tokens"])...)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]projectedExternalActionCandidateGroup, 0, len(ids))
	for _, id := range ids {
		group := *byID[id]
		sort.Strings(group.tokens)
		out = append(out, group)
	}
	return out
}

func appendUniqueStringsFold(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range additions {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func projectedExternalActionRequirementsForPhases(
	phases []map[string]interface{},
	runtimeState map[string]interface{},
) []projectedExternalActionRequirement {
	groups := projectedExternalActionIntentGroups(runtimeState)
	eligible := make([]map[string]interface{}, 0, len(phases))
	for _, phase := range phases {
		if !operationPlanPhaseOpenForToolCall(phase) || !operationPlanPhaseRequiredByServer(phase, runtimeState) ||
			operationPlanPhaseServerClassifiedNonTool(phase, runtimeState) {
			continue
		}
		eligible = append(eligible, phase)
	}
	requirements := make([]projectedExternalActionRequirement, 0)
	seen := map[string]struct{}{}
	appendRequirement := func(phaseID string, group projectedExternalActionCandidateGroup) {
		key := strings.ToLower(strings.TrimSpace(phaseID)) + "\x00" + group.id
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		requirements = append(requirements, projectedExternalActionRequirement{phaseID: strings.TrimSpace(phaseID), group: group})
	}
	for _, phase := range eligible {
		expected := evidenceMapFromAny(phase["expected_action"])
		candidate := projectedExternalActionCandidateForExpected(expected, runtimeState)
		if len(candidate) == 0 {
			continue
		}
		if operationPlanPhaseServerRequiresExternalAction(phase, runtimeState) {
			matched, _ := candidate["intent_matched"].(bool)
			if !matched && !projectedExternalActionPhaseHasPersistedFingerprint(phase, runtimeState) {
				// A model-selected alias is not semantic proof that this Action
				// satisfies a server external-apps outcome. Only a narrow intent
				// candidate or an already persisted server binding may seed it.
				continue
			}
		}
		groupID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(candidate["intent_group"])))
		group := projectedExternalActionCandidateGroup{id: groupID, candidates: []map[string]interface{}{candidate}}
		for _, candidateGroup := range groups {
			if candidateGroup.id == groupID {
				group = candidateGroup
				break
			}
		}
		if group.id == "" {
			group.id = "binding:" + strings.TrimSpace(evidenceStringFromAny(candidate["binding_fingerprint"]))
		}
		appendRequirement(evidenceStringFromAny(phase["id"]), group)
	}
	for _, group := range groups {
		maxScore := 0
		phaseScores := make(map[string]int, len(eligible))
		for _, phase := range eligible {
			phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
			text := strings.ToLower(strings.Join([]string{
				evidenceStringFromAny(phase["step"]), evidenceStringFromAny(phase["title"]),
				evidenceStringFromAny(operationPlanOutcomeForPhase(phase, runtimeState)["goal"]),
				evidenceStringFromAny(operationPlanOutcomeForPhase(phase, runtimeState)["title"]),
			}, " "))
			score := 0
			for _, token := range group.tokens {
				if token != "" && strings.Contains(text, token) {
					score++
				}
			}
			phaseScores[phaseID] = score
			if score > maxScore {
				maxScore = score
			}
		}
		if maxScore > 0 {
			for _, phase := range eligible {
				phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
				if phaseScores[phaseID] == maxScore {
					appendRequirement(phaseID, group)
				}
			}
			continue
		}
		if len(eligible) == 1 {
			appendRequirement(evidenceStringFromAny(eligible[0]["id"]), group)
		} else {
			appendRequirement("", group)
		}
	}
	// Structured server outcomes are authoritative even when the current query
	// has no usable lexical Action match (for example a continuation request of
	// just "continue"). Require an external canonical binding for every phase
	// explicitly classified with the external-apps capability. If no intent
	// group already maps that phase, any currently authorized candidate is an
	// any-of choice; an empty candidate set remains fail-closed.
	for _, phase := range eligible {
		if !operationPlanPhaseServerRequiresExternalAction(phase, runtimeState) {
			continue
		}
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		hasRequirement := false
		for _, requirement := range requirements {
			if strings.EqualFold(strings.TrimSpace(requirement.phaseID), phaseID) {
				hasRequirement = true
				break
			}
		}
		if !hasRequirement {
			appendRequirement(phaseID, projectedExternalActionCandidateGroup{
				id: "server-external-unresolved:" + strings.ToLower(phaseID),
			})
		}
	}
	return requirements
}

func projectedExternalActionPhaseHasPersistedFingerprint(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
	fingerprint := strings.TrimSpace(evidenceStringFromAny(evidenceMapFromAny(phase["expected_action"])[planExpectedActionServerBindingFingerprintKey]))
	if phaseID == "" || fingerprint == "" {
		return false
	}
	for _, baseline := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(baseline["id"])), phaseID) {
			continue
		}
		return fingerprint == strings.TrimSpace(evidenceStringFromAny(
			evidenceMapFromAny(baseline["expected_action"])[planExpectedActionServerBindingFingerprintKey],
		))
	}
	return false
}

func projectedExternalActionExpectedMatchesGroup(expected map[string]interface{}, group projectedExternalActionCandidateGroup) bool {
	fingerprint := strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey]))
	if fingerprint == "" {
		return false
	}
	for _, candidate := range group.candidates {
		if strings.TrimSpace(evidenceStringFromAny(candidate["binding_fingerprint"])) == fingerprint {
			return true
		}
	}
	return false
}

func validateProjectedExternalActionCandidateContracts(phases []map[string]interface{}, runtimeState map[string]interface{}) error {
	requirements := projectedExternalActionRequirementsForPhases(phases, runtimeState)
	if len(requirements) == 0 {
		return nil
	}
	byPhase := make(map[string][]projectedExternalActionRequirement)
	for _, requirement := range requirements {
		byPhase[requirement.phaseID] = append(byPhase[requirement.phaseID], requirement)
	}
	for phaseID, phaseRequirements := range byPhase {
		if phaseID == "" {
			for _, requirement := range phaseRequirements {
				matched := false
				for _, phase := range phases {
					if projectedExternalActionExpectedMatchesGroup(evidenceMapFromAny(phase["expected_action"]), requirement.group) {
						matched = true
						break
					}
				}
				if !matched {
					return fmt.Errorf("%w: external Action intent group %q is not mapped to a canonical plan phase", ErrInvalidInput, requirement.group.id)
				}
			}
			continue
		}
		if len(phaseRequirements) > 1 {
			return fmt.Errorf("%w: phase %q contains multiple requested external Action groups; split them into independently verifiable phases", ErrInvalidInput, phaseID)
		}
		var phase map[string]interface{}
		for _, candidate := range phases {
			if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(candidate["id"])), phaseID) {
				phase = candidate
				break
			}
		}
		if !projectedExternalActionExpectedMatchesGroup(evidenceMapFromAny(phase["expected_action"]), phaseRequirements[0].group) {
			return fmt.Errorf("%w: external phase %q requires one server-canonical Action from candidate group %q", ErrInvalidInput, phaseID, phaseRequirements[0].group.id)
		}
	}
	return nil
}

func projectedExternalActionSuccessfulExecutionKeys(runtimeState map[string]interface{}) map[string]struct{} {
	latest := map[string]bool{}
	for _, invocation := range terminalStateGuardExternalInvocations(runtimeState) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillExternalApps) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])), "execute_action") {
			continue
		}
		if actionKey := terminalStateGuardExternalActionKey(invocation); actionKey != "" {
			latest[actionKey] = runtimeInvocationSucceeded(invocation)
		}
	}
	out := map[string]struct{}{}
	for key, succeeded := range latest {
		if succeeded {
			out[key] = struct{}{}
		}
	}
	return out
}

func operationPlanCurrentOpenPhaseID(phases []map[string]interface{}) string {
	firstOpen := ""
	inProgress := ""
	for _, phase := range phases {
		if !operationPlanPhaseOpenForToolCall(phase) {
			continue
		}
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		if phaseID == "" {
			continue
		}
		if firstOpen == "" {
			firstOpen = phaseID
		}
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["status"])), "in_progress") ||
			strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["status"])), "running") {
			if inProgress != "" {
				return ""
			}
			inProgress = phaseID
		}
	}
	if inProgress != "" {
		return inProgress
	}
	return firstOpen
}

func operationPlanPhaseOpenForToolCall(phase map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["status"]))) {
	case "pending", "in_progress", "running":
		return true
	default:
		return false
	}
}

func operationPlanExpectedActionMatchesSkillCall(expected map[string]interface{}, skillID string, toolName string, arguments map[string]interface{}) bool {
	if len(expected) == 0 ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(expected["skill_id"])), strings.TrimSpace(skillID)) ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(expected["tool_name"])), strings.TrimSpace(toolName)) {
		return false
	}
	for key, expectedValue := range evidenceMapFromAny(expected["target"]) {
		expectedText := normalizeOperationPlanTargetValue(key, evidenceStringFromAny(expectedValue))
		actualText := normalizeOperationPlanTargetValue(key, operationPlanSkillCallTargetValue(arguments, key))
		if expectedText == "" || !strings.EqualFold(expectedText, actualText) {
			return false
		}
	}
	for path, expectedValue := range evidenceMapFromAny(expected["target_arguments"]) {
		expectedText := strings.TrimSpace(evidenceStringFromAny(expectedValue))
		actualText := operationPlanArgumentPathValue(evidenceMapFromAny(arguments["arguments"]), path)
		if actualText == "" {
			actualText = operationPlanArgumentPathValue(arguments, path)
		}
		if expectedText == "" || expectedText != strings.TrimSpace(actualText) {
			return false
		}
	}
	return true
}

func operationPlanArgumentPathValue(arguments map[string]interface{}, path string) string {
	path = strings.TrimSpace(path)
	if len(arguments) == 0 || path == "" {
		return ""
	}
	current := interface{}(arguments)
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return ""
		}
		mapped := evidenceMapFromAny(current)
		if len(mapped) == 0 {
			return ""
		}
		current = mapped[segment]
	}
	return strings.TrimSpace(evidenceStringFromAny(current))
}

func operationPlanSkillCallTargetValue(arguments map[string]interface{}, key string) string {
	if value := strings.TrimSpace(evidenceStringFromAny(arguments[key])); value != "" {
		return value
	}
	for _, containerKey := range []string{"arguments", "target", "system_prompt_source", "agent", "file", "resource"} {
		if value := strings.TrimSpace(evidenceStringFromAny(evidenceMapFromAny(arguments[containerKey])[key])); value != "" {
			return value
		}
	}
	if patch := evidenceMapFromAny(arguments["system_prompt_patch"]); len(patch) > 0 {
		if value := strings.TrimSpace(evidenceStringFromAny(evidenceMapFromAny(patch["source"])[key])); value != "" {
			return value
		}
	}
	return ""
}

func projectedExternalActionPlanIssue(
	runtimeState map[string]interface{},
	requestedPhaseID string,
	skillID string,
	toolName string,
	arguments map[string]interface{},
) string {
	actionKey := projectedExternalActionKeyFromCall(skillID, toolName, arguments)
	trusted := projectedExternalActionKeys(runtimeState)
	if actionKey == "" {
		return ""
	}
	if _, ok := trusted[actionKey]; !ok {
		return ""
	}
	phases := evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"])
	openPhases := make([]map[string]interface{}, 0, len(phases))
	for _, phase := range phases {
		if !operationPlanPhaseOpenForToolCall(phase) {
			continue
		}
		openPhases = append(openPhases, phase)
	}
	if len(openPhases) == 0 {
		return "a projected external Action requires a server-canonical expected_action ledger before its first execution"
	}
	if err := validateProjectedExternalActionCandidateContracts(phases, runtimeState); err != nil {
		return strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
	}
	matches := 0
	requestedMatch := false
	projectedExpectedActions := 0
	preparationMatches := projectedExternalActionPreparationPhaseMatches(runtimeState, actionKey)
	requestedPreparationMatch := false
	for _, phaseID := range preparationMatches {
		if strings.EqualFold(strings.TrimSpace(phaseID), strings.TrimSpace(requestedPhaseID)) {
			requestedPreparationMatch = true
			break
		}
	}
	for _, phase := range openPhases {
		expected := evidenceMapFromAny(phase["expected_action"])
		if expectedKey, external := projectedExternalActionKeyFromExpected(expected); external {
			if _, ok := trusted[expectedKey]; !ok {
				return "an external expected_action is not bound to a currently projected Action"
			}
			if issue := projectedExpectedActionServerBindingIssue(expected, runtimeState); issue != "" {
				return issue
			}
			if expectedKey == actionKey {
				if issue := projectedExpectedActionMutationTargetIssue(expected, runtimeState); issue != "" {
					return issue
				}
			}
			projectedExpectedActions++
		}
		if len(expected) > 0 && operationPlanExpectedActionMatchesSkillCall(expected, skillID, toolName, arguments) {
			matches++
			if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["id"])), strings.TrimSpace(requestedPhaseID)) {
				requestedMatch = true
			}
		}
	}
	if projectedExpectedActions == 0 {
		return "the current outcome ledger must include at least one server-canonical projected expected_action"
	}
	if strings.TrimSpace(requestedPhaseID) != "" {
		if !requestedMatch && !(strings.EqualFold(projectedExternalActionEffect(runtimeState, actionKey), "read") && requestedPreparationMatch) {
			return "plan_phase_id must name an open phase whose expected_action matches this projected Action and target"
		}
		return ""
	}
	if matches == 1 {
		return ""
	}
	if matches == 0 && strings.EqualFold(projectedExternalActionEffect(runtimeState, actionKey), "read") && len(preparationMatches) == 1 {
		// A projected read may be a prerequisite for a different server-bound
		// outcome (for example resolving a recipient before a send). It may run
		// only after that final expected Action is in the ledger, and it never
		// completes or substitutes for the outcome phase.
		return ""
	}
	if matches == 0 && strings.EqualFold(projectedExternalActionEffect(runtimeState, actionKey), "read") {
		if len(preparationMatches) == 0 {
			return "the projected read Action is not a server-attested prerequisite of any open expected Action"
		}
		return "plan_phase_id is required because this projected read Action prepares more than one open expected Action"
	}
	if matches != 1 {
		return "the projected external Action must match exactly one open required phase, including its stable target"
	}
	return ""
}

func projectedExternalActionPreparationPhaseMatches(runtimeState map[string]interface{}, prerequisiteActionKey string) []string {
	prerequisiteActionKey = strings.ToLower(strings.TrimSpace(prerequisiteActionKey))
	if prerequisiteActionKey == "" {
		return nil
	}
	out := make([]string, 0)
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if !operationPlanPhaseOpenForToolCall(phase) {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		if projectedExpectedActionServerBindingIssue(expected, runtimeState) != "" {
			continue
		}
		candidate := projectedExternalActionCandidateForExpected(expected, runtimeState)
		if len(candidate) > 0 {
			for _, preparationActionKey := range evidenceStringSliceFromAny(candidate["preparation_action_keys"]) {
				if strings.EqualFold(strings.TrimSpace(preparationActionKey), prerequisiteActionKey) {
					if phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"])); phaseID != "" {
						out = append(out, phaseID)
					}
					break
				}
			}
		}
	}
	return out
}

func validateProjectedExternalActionPlanSnapshot(phases []map[string]interface{}, runtimeState map[string]interface{}) error {
	trusted := projectedExternalActionKeys(runtimeState)
	if len(phases) == 0 {
		return nil
	}
	if err := validatePersistedProjectedPhaseImmutability(phases, runtimeState); err != nil {
		return err
	}
	if len(trusted) == 0 {
		return nil
	}
	hasProjectedAction := false
	for _, phase := range phases {
		expected := evidenceMapFromAny(phase["expected_action"])
		if expectedKey, external := projectedExternalActionKeyFromExpected(expected); external {
			if _, ok := trusted[expectedKey]; !ok {
				return fmt.Errorf("%w: external expected_action is not a currently projected Action", ErrInvalidInput)
			}
			if issue := projectedExpectedActionServerBindingIssue(expected, runtimeState); issue != "" {
				return fmt.Errorf("%w: %s", ErrInvalidInput, issue)
			}
			hasProjectedAction = true
		}
	}
	if err := validateProjectedExternalActionCandidateContracts(phases, runtimeState); err != nil {
		return err
	}
	baselineByID := map[string]map[string]interface{}{}
	baselineAllByID := map[string]map[string]interface{}{}
	baselineProjectedByID := map[string]map[string]interface{}{}
	baselineHasProjectedAction := false
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		if _, external := projectedExternalActionKeyFromExpected(evidenceMapFromAny(phase["expected_action"])); external && phaseID != "" {
			baselineProjectedByID[phaseID] = phase
			baselineHasProjectedAction = true
		}
		if operationPlanPhaseOpenForToolCall(phase) {
			if phaseID != "" {
				baselineAllByID[phaseID] = phase
			}
		}
		if !operationPlanPhaseRequiredByServer(phase, runtimeState) {
			continue
		}
		if operationPlanPhaseOpenForToolCall(phase) {
			if phaseID != "" {
				baselineByID[phaseID] = phase
			}
		}
	}
	for _, phase := range phases {
		expected := evidenceMapFromAny(phase["expected_action"])
		if _, external := projectedExternalActionKeyFromExpected(expected); !external {
			continue
		}
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		if _, persisted := baselineProjectedByID[phaseID]; persisted {
			continue
		}
		if !operationPlanPhaseOpenForToolCall(phase) {
			return fmt.Errorf(
				"%w: new projected external phase %q cannot be completed or skipped before matching runtime evidence",
				ErrInvalidInput,
				phaseID,
			)
		}
	}
	externalRequiredPhaseIDs := projectedExternalActionRequiredPhaseIDs(runtimeState)
	enforceProjectedLedger := hasProjectedAction || baselineHasProjectedAction || len(externalRequiredPhaseIDs) > 0
	legacyProjectedLedger := (hasProjectedAction || baselineHasProjectedAction) && len(evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionCandidatesKey])) == 0
	if len(baselineByID) > 1 && len(phases) < len(baselineByID) {
		return fmt.Errorf("%w: plan kept %d phases but the current request has %d open required phases", ErrInvalidInput, len(phases), len(baselineByID))
	}
	nextByID := make(map[string]map[string]interface{}, len(phases))
	for _, phase := range phases {
		if id := strings.TrimSpace(evidenceStringFromAny(phase["id"])); id != "" {
			nextByID[id] = phase
		}
	}
	for id, next := range nextByID {
		if outcomeID := strings.TrimSpace(evidenceStringFromAny(baselineAllByID[id]["outcome_id"])); outcomeID != "" {
			// outcome_id is server-owned linkage to required/verification policy.
			// normalizePlanSnapshot intentionally ignores model-provided values.
			next["outcome_id"] = outcomeID
		}
	}
	for id, baseline := range baselineProjectedByID {
		next := nextByID[id]
		if len(next) == 0 {
			return fmt.Errorf("%w: plan omitted server-bound projected phase %q", ErrInvalidInput, id)
		}
		if issue := projectedExpectedActionReplacementIssue(
			evidenceMapFromAny(baseline["expected_action"]),
			evidenceMapFromAny(next["expected_action"]),
		); issue != "" {
			return fmt.Errorf("%w: server-bound projected phase %q %s", ErrInvalidInput, id, issue)
		}
		baselineStatus := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(baseline["status"])))
		if operationPlanPhaseOpenForToolCall(baseline) && !operationPlanPhaseOpenForToolCall(next) {
			return fmt.Errorf("%w: open projected phase %q cannot be completed or skipped before matching runtime evidence", ErrInvalidInput, id)
		}
		if baselineStatus == "completed" && !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(next["status"])), "completed") {
			return fmt.Errorf("%w: completed projected phase %q cannot be reopened", ErrInvalidInput, id)
		}
	}
	for id, baseline := range baselineByID {
		next := nextByID[id]
		if len(next) == 0 {
			return fmt.Errorf("%w: plan omitted open required phase %q", ErrInvalidInput, id)
		}
		_, projectedBaseline := baselineProjectedByID[id]
		_, externalRequired := externalRequiredPhaseIDs[id]
		if legacyProjectedLedger || projectedBaseline || externalRequired {
			switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(next["status"]))) {
			case "pending", "in_progress", "running":
			default:
				return fmt.Errorf("%w: open required external phase %q cannot be completed or skipped before matching runtime evidence", ErrInvalidInput, id)
			}
		}
		if enforceProjectedLedger && (legacyProjectedLedger || externalRequired || projectedBaseline) {
			if externalRequired {
				expected := evidenceMapFromAny(next["expected_action"])
				if _, external := projectedExternalActionKeyFromExpected(expected); !external ||
					projectedExpectedActionServerBindingIssue(expected, runtimeState) != "" {
					return fmt.Errorf("%w: external runtime phase %q requires a server-canonical projected expected_action", ErrInvalidInput, id)
				}
			}
			if len(evidenceMapFromAny(next["expected_action"])) == 0 &&
				!(operationPlanPhaseServerClassifiedNonTool(baseline, runtimeState) && operationPlanPhaseHasSafeNonToolClassification(next)) {
				return fmt.Errorf("%w: open required phase %q requires expected_action", ErrInvalidInput, id)
			}
			if issue := projectedExpectedActionReplacementIssue(
				evidenceMapFromAny(baseline["expected_action"]),
				evidenceMapFromAny(next["expected_action"]),
			); issue != "" {
				return fmt.Errorf("%w: open required phase %q %s", ErrInvalidInput, id, issue)
			}
		}
	}
	if !enforceProjectedLedger {
		return nil
	}
	if max(len(baselineByID), len(phases)) <= 1 {
		return nil
	}
	for _, phase := range phases {
		if !operationPlanPhaseRequiredByServer(phase, runtimeState) {
			continue
		}
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		if _, externalRequired := externalRequiredPhaseIDs[phaseID]; !externalRequired && !legacyProjectedLedger {
			continue
		}
		if len(evidenceMapFromAny(phase["expected_action"])) == 0 && !operationPlanPhaseHasSafeNonToolClassification(phase) {
			return fmt.Errorf("%w: every phase in a compound projected Action plan requires expected_action", ErrInvalidInput)
		}
	}
	return nil
}

func validatePersistedProjectedPhaseImmutability(phases []map[string]interface{}, runtimeState map[string]interface{}) error {
	nextByID := make(map[string]map[string]interface{}, len(phases))
	for _, phase := range phases {
		if id := strings.TrimSpace(evidenceStringFromAny(phase["id"])); id != "" {
			nextByID[id] = phase
		}
	}
	for _, baseline := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		baselineExpected := evidenceMapFromAny(baseline["expected_action"])
		if _, external := projectedExternalActionKeyFromExpected(baselineExpected); !external ||
			(strings.TrimSpace(evidenceStringFromAny(baselineExpected[planExpectedActionServerBindingFingerprintKey])) == "" &&
				strings.TrimSpace(evidenceStringFromAny(baselineExpected[planExpectedActionServerProjectionKey])) == "") {
			continue
		}
		id := strings.TrimSpace(evidenceStringFromAny(baseline["id"]))
		next := nextByID[id]
		if id == "" || len(next) == 0 {
			return fmt.Errorf("%w: plan omitted server-bound projected phase %q", ErrInvalidInput, id)
		}
		if issue := projectedExpectedActionReplacementIssue(baselineExpected, evidenceMapFromAny(next["expected_action"])); issue != "" {
			return fmt.Errorf("%w: server-bound projected phase %q %s", ErrInvalidInput, id, issue)
		}
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(baseline["status"])), "completed") &&
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(next["status"])), "completed") {
			return fmt.Errorf("%w: completed projected phase %q cannot be reopened", ErrInvalidInput, id)
		}
	}
	return nil
}

func projectedExpectedActionServerBindingIssue(expected map[string]interface{}, runtimeState map[string]interface{}) string {
	actionKey, external := projectedExternalActionKeyFromExpected(expected)
	if !external || actionKey == "" {
		return ""
	}
	fingerprint := strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey]))
	if fingerprint == "" {
		serverAlias := strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerProjectionKey]))
		if serverAlias == "" {
			return "external expected_action was not canonicalized from a currently exposed projected Action"
		}
		for _, projection := range evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionProjectionsKey]) {
			if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(projection["tool_name"])), serverAlias) {
				continue
			}
			projectionKey := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["integration_id"]))) + ":" +
				strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["action_id"])))
			if projectionKey == actionKey {
				return ""
			}
		}
		return "external expected_action projection binding is no longer exposed"
	}
	projection := projectedExternalActionCandidateByFingerprint(runtimeState, fingerprint)
	if len(projection) == 0 {
		return "external expected_action projection binding is no longer exposed"
	}
	projectionKey := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["integration_id"]))) + ":" +
		strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["action_id"])))
	if projectionKey != actionKey {
		return "external expected_action identity does not match its server projection binding"
	}
	allowedTargetPaths := map[string]struct{}{}
	for _, path := range evidenceStringSliceFromAny(projection["target_argument_paths"]) {
		if path = strings.TrimSpace(path); path != "" {
			allowedTargetPaths[path] = struct{}{}
		}
	}
	targetArguments := evidenceMapFromAny(expected["target_arguments"])
	for path := range targetArguments {
		if _, ok := allowedTargetPaths[strings.TrimSpace(path)]; !ok {
			return "external expected_action contains a target path outside its server projection binding"
		}
	}
	return ""
}

func projectedExpectedActionMutationTargetIssue(expected map[string]interface{}, runtimeState map[string]interface{}) string {
	projection := projectedExternalActionCandidateForExpected(expected, runtimeState)
	if len(projection) == 0 || !projectedExternalActionEffectRequiresBoundTarget(evidenceStringFromAny(projection["effect"])) {
		return ""
	}
	targetArguments := evidenceMapFromAny(expected["target_arguments"])
	for _, path := range evidenceStringSliceFromAny(projection["target_argument_paths"]) {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		value := strings.TrimSpace(evidenceStringFromAny(targetArguments[path]))
		if value == "" {
			value = operationPlanArgumentPathValue(targetArguments, path)
		}
		if value == "" && !projectedExternalTargetPathConditionallyOptional(projection, targetArguments, path) {
			return projectedExternalMutationTargetMissingIssue
		}
	}
	return ""
}

func projectedExternalTargetPathConditionallyOptional(
	candidate map[string]interface{},
	targetArguments map[string]interface{},
	path string,
) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	defaults := evidenceMapFromAny(candidate["default_arguments"])
	for _, condition := range evidenceMapsFromAny(candidate["optional_targets"]) {
		if strings.TrimSpace(evidenceStringFromAny(condition["path"])) != path {
			continue
		}
		whenArgument := strings.TrimSpace(evidenceStringFromAny(condition["when_argument"]))
		if whenArgument == "" {
			continue
		}
		actual := projectedExternalExpectedTargetArgumentValue(targetArguments, whenArgument)
		if actual == "" {
			actual = operationPlanArgumentPathValue(defaults, whenArgument)
		}
		if actual == "" {
			actual = strings.TrimSpace(evidenceStringFromAny(defaults[whenArgument]))
		}
		expected := strings.TrimSpace(evidenceStringFromAny(condition["when_equals"]))
		if actual != "" && expected != "" && actual == expected {
			return true
		}
	}
	return false
}

func projectedExternalActionEffectRequiresBoundTarget(effect string) bool {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "", "none", "read":
		return false
	default:
		return true
	}
}

func projectedExternalActionCandidateByFingerprint(runtimeState map[string]interface{}, fingerprint string) map[string]interface{} {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil
	}
	for _, candidate := range evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionCandidatesKey]) {
		if strings.TrimSpace(evidenceStringFromAny(candidate["binding_fingerprint"])) == fingerprint {
			return candidate
		}
	}
	return nil
}

func projectedExternalActionCandidateForExpected(expected map[string]interface{}, runtimeState map[string]interface{}) map[string]interface{} {
	return projectedExternalActionCandidateByFingerprint(
		runtimeState,
		strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey])),
	)
}

func projectedExternalActionObservedPreparationTargets(
	runtimeState map[string]interface{},
	callArguments map[string]interface{},
	phaseID string,
	ledgerEpoch string,
	bindingFingerprint string,
	providerPayload map[string]interface{},
) map[string]interface{} {
	phaseID = strings.TrimSpace(phaseID)
	ledgerEpoch = strings.TrimSpace(ledgerEpoch)
	bindingFingerprint = strings.TrimSpace(bindingFingerprint)
	if phaseID == "" || ledgerEpoch == "" || bindingFingerprint == "" {
		return nil
	}
	actionKey := projectedExternalActionKeyFromCall(skills.SkillExternalApps, "execute_action", callArguments)
	if actionKey == "" || !strings.EqualFold(projectedExternalActionEffect(runtimeState, actionKey), "read") {
		return nil
	}
	payloadActionKey := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(providerPayload["integration_id"]))) + ":" +
		strings.ToLower(strings.TrimSpace(evidenceStringFromAny(providerPayload["action_id"])))
	if payloadActionKey != actionKey {
		return nil
	}
	candidate := projectedExternalActionCandidateByFingerprint(runtimeState, bindingFingerprint)
	if len(candidate) == 0 {
		return nil
	}
	expectedConnectionBinding := strings.TrimSpace(evidenceStringFromAny(candidate[operationPlanServerProjectedConnectionBindingKey]))
	actualConnectionBinding := skills.NativeExternalActionConnectionBindingHash(
		operationPlanSkillCallTargetValue(callArguments, "connection_id"),
	)
	if expectedConnectionBinding == "" || actualConnectionBinding == "" || expectedConnectionBinding != actualConnectionBinding {
		return nil
	}
	phaseMatched := false
	var phaseExpected map[string]interface{}
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if strings.TrimSpace(evidenceStringFromAny(phase["id"])) != phaseID ||
			strings.TrimSpace(evidenceStringFromAny(phase[operationPlanServerProjectedLedgerEpochKey])) != ledgerEpoch {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		if strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey])) == bindingFingerprint {
			phaseMatched = true
			phaseExpected = expected
			break
		}
	}
	if !phaseMatched {
		return nil
	}
	providerResult := evidenceMapFromAny(providerPayload["result"])
	if len(providerResult) == 0 || !projectedExternalPreparationProviderStatusSucceeded(
		strings.ToLower(strings.TrimSpace(evidenceStringFromAny(providerPayload["operation_status"]))),
	) {
		return nil
	}
	out := map[string]interface{}{}
	for _, hint := range evidenceMapsFromAny(candidate["preparation_hints"]) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(hint["action_key"])), actionKey) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(hint["relation"])), "resolve_target") {
			continue
		}
		targetArguments := evidenceStringSliceFromAny(hint["target_arguments"])
		resultPaths := evidenceStringSliceFromAny(hint["result_paths"])
		resultTransform := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(hint["result_transform"])))
		if resultTransform == "split_slash_pair" {
			if len(targetArguments) != 2 || len(resultPaths) != 1 {
				continue
			}
			rawValues := projectedExternalPreparationResultPathValues(providerResult, resultPaths[0])
			if len(rawValues) == 0 {
				continue
			}
			values := projectedExternalPreparationDistinctValues(rawValues)
			if len(values) != 1 || !projectedExternalPreparationResultIsComplete(
				callArguments, providerPayload, len(rawValues),
				projectedExternalPreparationResultLimitDefault(runtimeState, actionKey), resultPaths[0],
			) {
				return nil
			}
			left, right, ok := projectedExternalPreparationSplitSlashPair(values[0])
			if !ok {
				return nil
			}
			transformed := []string{left, right}
			baselineTargets := evidenceMapFromAny(phaseExpected["target_arguments"])
			for index, targetArgument := range targetArguments {
				targetArgument = strings.TrimSpace(targetArgument)
				if targetArgument == "" {
					return nil
				}
				if baseline := projectedExternalExpectedTargetArgumentValue(baselineTargets, targetArgument); baseline != "" && baseline != transformed[index] {
					return nil
				}
				out[targetArgument] = []interface{}{transformed[index]}
			}
			continue
		}
		if resultTransform != "" || len(targetArguments) != 1 {
			// Without a server-declared row/field mapping, independently
			// flattening multiple target arguments could combine values from
			// different provider rows. Fail closed until the catalog contract can
			// express tuple correlation explicitly.
			continue
		}
		for _, targetArgument := range targetArguments {
			targetArgument = strings.TrimSpace(targetArgument)
			if targetArgument == "" {
				continue
			}
			for _, selectedResultPath := range projectedExternalPreparationResultPathsForTarget(targetArgument, targetArguments, resultPaths) {
				rawValues := projectedExternalPreparationResultPathValues(providerResult, selectedResultPath)
				values := projectedExternalPreparationDistinctValues(rawValues)
				if len(values) == 0 {
					continue
				}
				// Equivalent result paths (notably Feishu open_id/user_id)
				// are alternatives, not a merged collection. Select the first
				// non-empty path that satisfies the server-bound discriminator.
				if !projectedExternalPreparationContextMatches(
					candidate, phaseExpected, callArguments, targetArgument, selectedResultPath,
				) {
					continue
				}
				if !projectedExternalPreparationResultIsComplete(
					callArguments, providerPayload, len(rawValues),
					projectedExternalPreparationResultLimitDefault(runtimeState, actionKey), selectedResultPath,
				) {
					return nil
				}
				out[targetArgument] = stringSliceInterfaces(values)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func projectedExternalPreparationProviderStatusSucceeded(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "completed", "already_completed":
		return true
	default:
		return false
	}
}

func projectedExternalPreparationObservedValuesForTarget(
	providerResult map[string]interface{},
	targetArgument string,
	targetArguments []string,
	resultPaths []string,
) []string {
	values, _, _ := projectedExternalPreparationObservedValuesPathAndCount(providerResult, targetArgument, targetArguments, resultPaths)
	return values
}

func projectedExternalPreparationObservedValuesAndPath(
	providerResult map[string]interface{},
	targetArgument string,
	targetArguments []string,
	resultPaths []string,
) ([]string, string) {
	values, path, _ := projectedExternalPreparationObservedValuesPathAndCount(
		providerResult, targetArgument, targetArguments, resultPaths,
	)
	return values, path
}

func projectedExternalPreparationObservedValuesPathAndCount(
	providerResult map[string]interface{},
	targetArgument string,
	targetArguments []string,
	resultPaths []string,
) ([]string, string, int) {
	for _, resultPath := range projectedExternalPreparationResultPathsForTarget(targetArgument, targetArguments, resultPaths) {
		rawValues := projectedExternalPreparationResultPathValues(providerResult, resultPath)
		values := projectedExternalPreparationDistinctValues(rawValues)
		if len(values) > 0 {
			// ResultPaths are an ordered server contract. One preparation
			// result may expose equivalent identifier forms (for example
			// open_id and user_id); never merge those forms into fake
			// duplicate entities.
			return values, resultPath, len(rawValues)
		}
	}
	return nil, "", 0
}

type projectedExternalPreparationCompleteness struct {
	invalid        bool
	incomplete     bool
	explicitNoMore bool
	total          int
	hasTotal       bool
	maxEstimate    int
	hasEstimate    bool
}

// projectedExternalPreparationResultIsComplete accepts target evidence only
// when the server facade proves that the one observed row is the complete
// result, rather than merely the first (or an arbitrary later) page. Provider
// prose and model claims are deliberately ignored.
func projectedExternalPreparationResultIsComplete(
	callArguments map[string]interface{},
	providerPayload map[string]interface{},
	observedRows int,
	serverDefaultLimit int,
	selectedResultPath string,
) bool {
	if observedRows != 1 {
		return false
	}
	resultCount, ok := projectedExternalPreparationExactNonnegativeInt(providerPayload["result_count"])
	if !ok || resultCount != observedRows {
		return false
	}
	input := evidenceMapFromAny(callArguments["arguments"])
	inputState := projectedExternalPreparationInputPageState(input)
	if inputState.invalid || inputState.nonFirstPage {
		return false
	}
	if inputState.limit == 0 && serverDefaultLimit > 0 {
		inputState.limit = serverDefaultLimit
	}
	state := projectedExternalPreparationCompleteness{}
	budget := 8192
	projectedExternalPreparationScanCompleteness(providerPayload, 0, &budget, &state)
	if state.invalid || state.incomplete || budget < 0 {
		return false
	}
	if state.hasTotal && state.total != observedRows {
		return false
	}
	if state.hasEstimate && state.maxEstimate > observedRows {
		return false
	}
	if !strings.Contains(strings.TrimSpace(selectedResultPath), "[]") {
		// A server-declared scalar path denotes one exact object lookup, not a
		// page. The facade's exact result_count plus the structural path is
		// sufficient when no explicit truncation/incompleteness signal exists.
		return true
	}
	if inputState.limit == 0 && !state.explicitNoMore && !state.hasTotal {
		// A single row without an authoritative total/no-more marker or an
		// explicit under-filled server request is only "one row on this
		// response", not proof of global uniqueness.
		return false
	}
	// Hitting an explicit result cap proves only that the current page is
	// full. It is safe solely when the provider also emitted an authoritative
	// no-more marker such as has_more=false or an empty next-page token.
	if inputState.limit > 0 && inputState.limit <= observedRows && !state.explicitNoMore && !state.hasTotal {
		return false
	}
	return true
}

func projectedExternalPreparationResultLimitDefault(runtimeState map[string]interface{}, actionKey string) int {
	actionKey = strings.ToLower(strings.TrimSpace(actionKey))
	for _, candidate := range evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionCandidatesKey]) {
		candidateKey := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(candidate["integration_id"]))) + ":" +
			strings.ToLower(strings.TrimSpace(evidenceStringFromAny(candidate["action_id"])))
		if candidateKey != actionKey {
			continue
		}
		limit, ok := projectedExternalPreparationExactNonnegativeInt(candidate["result_limit_default"])
		if ok && limit > 0 {
			return limit
		}
	}
	return 0
}

type projectedExternalPreparationInputState struct {
	invalid      bool
	nonFirstPage bool
	limit        int
}

func projectedExternalPreparationInputPageState(input map[string]interface{}) projectedExternalPreparationInputState {
	state := projectedExternalPreparationInputState{}
	budget := 1024
	var visit func(interface{}, int)
	visit = func(value interface{}, depth int) {
		if state.invalid || depth > 12 {
			state.invalid = true
			return
		}
		budget--
		if budget < 0 {
			state.invalid = true
			return
		}
		switch typed := value.(type) {
		case map[string]interface{}:
			for key, nested := range typed {
				switch projectedExternalPreparationMetadataKey(key) {
				case "cursor", "pagetoken", "nextpagetoken", "nexttoken", "nextcursor", "paginationtoken", "continuationtoken", "synctoken":
					if strings.TrimSpace(evidenceStringFromAny(nested)) != "" {
						state.nonFirstPage = true
					}
				case "page", "pagenumber":
					page, ok := projectedExternalPreparationExactNonnegativeInt(nested)
					if !ok {
						state.invalid = true
					} else if page > 1 {
						state.nonFirstPage = true
					}
				case "pageindex", "offset":
					page, ok := projectedExternalPreparationExactNonnegativeInt(nested)
					if !ok {
						state.invalid = true
					} else if page > 0 {
						state.nonFirstPage = true
					}
				case "limit", "maxresults", "pagesize", "perpage":
					limit, ok := projectedExternalPreparationExactNonnegativeInt(nested)
					if !ok || limit <= 0 {
						state.invalid = true
					} else if state.limit == 0 || limit < state.limit {
						state.limit = limit
					}
				}
				visit(nested, depth+1)
			}
		case []interface{}:
			for _, nested := range typed {
				visit(nested, depth+1)
			}
		}
	}
	visit(input, 0)
	return state
}

func projectedExternalPreparationScanCompleteness(
	value interface{},
	depth int,
	budget *int,
	state *projectedExternalPreparationCompleteness,
) {
	if state.invalid || depth > 16 {
		state.invalid = true
		return
	}
	*budget--
	if *budget < 0 {
		state.invalid = true
		return
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			switch projectedExternalPreparationMetadataKey(key) {
			case "truncated", "resulttruncated", "contenttruncated", "istruncated":
				flag, ok := projectedExternalPreparationExactBool(nested)
				if !ok {
					state.invalid = true
				} else if flag {
					state.incomplete = true
				}
			case "hasmore", "hasnext":
				flag, ok := projectedExternalPreparationExactBool(nested)
				if !ok {
					state.invalid = true
				} else if flag {
					state.incomplete = true
				} else {
					state.explicitNoMore = true
				}
			case "incomplete", "incompleteresults", "isincomplete":
				flag, ok := projectedExternalPreparationExactBool(nested)
				if !ok {
					state.invalid = true
				} else if flag {
					state.incomplete = true
				}
			case "nextpagetoken", "nexttoken", "nextcursor", "continuationtoken", "paginationtoken", "pagetoken", "cursor":
				if strings.TrimSpace(evidenceStringFromAny(nested)) != "" {
					state.incomplete = true
				} else if projectedExternalPreparationMetadataKey(key) != "cursor" &&
					projectedExternalPreparationMetadataKey(key) != "pagetoken" {
					state.explicitNoMore = true
				}
			case "total", "totalcount":
				total, ok := projectedExternalPreparationExactNonnegativeInt(nested)
				if !ok {
					state.invalid = true
				} else {
					if state.hasTotal && state.total != total {
						state.invalid = true
					} else {
						state.hasTotal = true
						state.total = total
					}
				}
			case "resultsizeestimate":
				estimate, ok := projectedExternalPreparationExactNonnegativeInt(nested)
				if !ok {
					state.invalid = true
				} else {
					state.hasEstimate = true
					if estimate > state.maxEstimate {
						state.maxEstimate = estimate
					}
				}
			}
			projectedExternalPreparationScanCompleteness(nested, depth+1, budget, state)
		}
	case []interface{}:
		// Pagination metadata is authoritative only on response envelopes.
		// Never let a provider-controlled business record inside a result array
		// contribute has_more=false or an empty next token as completeness proof.
		return
	}
}

func projectedExternalPreparationMetadataKey(key string) string {
	var out strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(key)) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			out.WriteRune(char)
		}
	}
	return out.String()
}

func projectedExternalPreparationExactBool(value interface{}) (bool, bool) {
	typed, ok := value.(bool)
	return typed, ok
}

func projectedExternalPreparationExactNonnegativeInt(value interface{}) (int, bool) {
	var parsed int64
	switch typed := value.(type) {
	case int:
		parsed = int64(typed)
	case int8:
		parsed = int64(typed)
	case int16:
		parsed = int64(typed)
	case int32:
		parsed = int64(typed)
	case int64:
		parsed = typed
	case uint:
		if uint64(typed) > uint64(math.MaxInt64) {
			return 0, false
		}
		parsed = int64(typed)
	case uint8:
		parsed = int64(typed)
	case uint16:
		parsed = int64(typed)
	case uint32:
		parsed = int64(typed)
	case uint64:
		if typed > uint64(math.MaxInt64) {
			return 0, false
		}
		parsed = int64(typed)
	case float32:
		value64 := float64(typed)
		if math.IsNaN(value64) || math.IsInf(value64, 0) || math.Trunc(value64) != value64 || value64 > float64(math.MaxInt64) {
			return 0, false
		}
		parsed = int64(value64)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed > float64(math.MaxInt64) {
			return 0, false
		}
		parsed = int64(typed)
	case json.Number:
		value64, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		parsed = value64
	case string:
		value64, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		parsed = value64
	default:
		return 0, false
	}
	if parsed < 0 || parsed > int64(math.MaxInt) {
		return 0, false
	}
	return int(parsed), true
}

func projectedExternalPreparationContextMatches(
	candidate map[string]interface{},
	phaseExpected map[string]interface{},
	callArguments map[string]interface{},
	resolvedTargetPath string,
	selectedResultPath string,
) bool {
	baselineTargets := evidenceMapFromAny(phaseExpected["target_arguments"])
	preparationArguments := evidenceMapFromAny(callArguments["arguments"])
	selectedSegments := strings.Split(strings.TrimSpace(selectedResultPath), ".")
	selectedLeaf := ""
	if len(selectedSegments) > 0 {
		selectedLeaf = strings.TrimSuffix(strings.TrimSpace(selectedSegments[len(selectedSegments)-1]), "[]")
	}
	for _, contextPath := range evidenceStringSliceFromAny(candidate["target_argument_paths"]) {
		contextPath = strings.TrimSpace(contextPath)
		if contextPath == "" || contextPath == strings.TrimSpace(resolvedTargetPath) {
			continue
		}
		baselineValue := projectedExternalExpectedTargetArgumentValue(baselineTargets, contextPath)
		if contextPath == "recipient_type" && strings.TrimSpace(resolvedTargetPath) == "recipient_id" {
			// recipient_id has no meaning without its identifier namespace. The
			// selected server result path is the namespace proof; missing or
			// mismatched model-authored context must fail closed.
			if baselineValue == "" || selectedLeaf != baselineValue {
				return false
			}
			continue
		}
		if baselineValue == "" {
			continue
		}
		if preparationValue := operationPlanArgumentPathValue(preparationArguments, contextPath); preparationValue == baselineValue {
			continue
		}
		// Feishu-style identifier discriminators are safely derived from the
		// server-declared selected result path, never from model-authored target
		// updates. Other companion context requires an exact preparation input.
		if contextPath == "recipient_type" && selectedLeaf == baselineValue {
			continue
		}
		return false
	}
	return true
}

func projectedExternalPreparationSplitSlashPair(value string) (string, string, bool) {
	if value != strings.TrimSpace(value) || strings.Count(value, "/") != 1 {
		return "", "", false
	}
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || !projectedExternalPreparationSafeIdentifier(parts[0]) || !projectedExternalPreparationSafeIdentifier(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func projectedExternalPreparationSafeIdentifier(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 100 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '.' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func projectedExternalPreparationResultPathsForTarget(target string, targets []string, resultPaths []string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	matched := make([]string, 0, len(resultPaths))
	for _, resultPath := range resultPaths {
		resultPath = strings.TrimSpace(resultPath)
		segments := strings.Split(resultPath, ".")
		leaf := strings.TrimSuffix(strings.TrimSpace(segments[len(segments)-1]), "[]")
		if leaf == target {
			matched = append(matched, resultPath)
		}
	}
	if len(matched) > 0 || len(targets) != 1 {
		return matched
	}
	return append([]string(nil), resultPaths...)
}

func projectedExternalPreparationResultPathValues(root interface{}, path string) []string {
	current := []interface{}{root}
	for _, rawSegment := range strings.Split(strings.TrimSpace(path), ".") {
		segment := strings.TrimSpace(rawSegment)
		if segment == "" {
			return nil
		}
		array := strings.HasSuffix(segment, "[]")
		segment = strings.TrimSuffix(segment, "[]")
		next := make([]interface{}, 0)
		for _, item := range current {
			value := evidenceMapFromAny(item)[segment]
			if array {
				next = append(next, evidenceSliceFromAny(value)...)
			} else if value != nil {
				next = append(next, value)
			}
		}
		current = next
		if len(current) == 0 {
			return nil
		}
	}
	out := make([]string, 0, len(current))
	for _, value := range current {
		switch value.(type) {
		case map[string]interface{}, []interface{}, []map[string]interface{}:
			continue
		}
		if normalized := strings.TrimSpace(evidenceStringFromAny(value)); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

func projectedExternalPreparationDistinctValues(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func validateProjectedExternalActionObservedTargetBindings(
	phases []map[string]interface{},
	runtimeState map[string]interface{},
) (map[string]struct{}, error) {
	baselineByID := map[string]map[string]interface{}{}
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if id := strings.TrimSpace(evidenceStringFromAny(phase["id"])); id != "" {
			baselineByID[id] = phase
		}
	}
	resolved := map[string]struct{}{}
	for _, phase := range phases {
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		baseline := baselineByID[phaseID]
		if phaseID == "" || len(baseline) == 0 {
			continue
		}
		baselineExpected := evidenceMapFromAny(baseline["expected_action"])
		fingerprint := strings.TrimSpace(evidenceStringFromAny(baselineExpected[planExpectedActionServerBindingFingerprintKey]))
		candidate := projectedExternalActionCandidateByFingerprint(runtimeState, fingerprint)
		if fingerprint == "" || len(candidate) == 0 ||
			!projectedExternalActionEffectRequiresBoundTarget(evidenceStringFromAny(candidate["effect"])) {
			continue
		}
		nextExpected := evidenceMapFromAny(phase["expected_action"])
		baselineTargets := evidenceMapFromAny(baselineExpected["target_arguments"])
		nextTargets := evidenceMapFromAny(nextExpected["target_arguments"])
		wasIncomplete := false
		isComplete := true
		changed := false
		for _, path := range evidenceStringSliceFromAny(candidate["target_argument_paths"]) {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			if projectedExternalTargetPathConditionallyOptional(candidate, baselineTargets, path) ||
				projectedExternalTargetPathConditionallyOptional(candidate, nextTargets, path) {
				continue
			}
			baselineValue := projectedExternalExpectedTargetArgumentValue(baselineTargets, path)
			nextValue := projectedExternalExpectedTargetArgumentValue(nextTargets, path)
			if baselineValue == "" {
				wasIncomplete = true
				if nextValue != "" {
					changed = true
					observed := projectedExternalActionObservedTargetValues(runtimeState, phaseID, fingerprint, path)
					switch len(observed) {
					case 0:
						return nil, fmt.Errorf("%w: phase %q target %q was not observed from a successful declared preparation Action in the current ledger epoch", ErrInvalidInput, phaseID, path)
					case 1:
						if nextValue != observed[0] {
							return nil, fmt.Errorf("%w: phase %q target %q does not exactly match its observed preparation result", ErrInvalidInput, phaseID, path)
						}
					default:
						return nil, fmt.Errorf("%w: phase %q target %q is ambiguous; refine or explicitly disambiguate the preparation result before binding", ErrInvalidInput, phaseID, path)
					}
				}
			}
			if nextValue == "" {
				isComplete = false
			}
		}
		if wasIncomplete && isComplete && changed {
			resolved[phaseID] = struct{}{}
		}
	}
	return resolved, nil
}

func projectedExternalExpectedTargetArgumentValue(targets map[string]interface{}, path string) string {
	if value := strings.TrimSpace(evidenceStringFromAny(targets[path])); value != "" {
		return value
	}
	return operationPlanArgumentPathValue(targets, path)
}

func projectedExternalActionObservedTargetValues(
	runtimeState map[string]interface{},
	phaseID string,
	bindingFingerprint string,
	targetPath string,
) []string {
	phaseID = strings.TrimSpace(phaseID)
	bindingFingerprint = strings.TrimSpace(bindingFingerprint)
	targetPath = strings.TrimSpace(targetPath)
	ledgerEpoch := operationPlanProjectedLedgerEpoch(runtimeState, phaseID)
	if phaseID == "" || bindingFingerprint == "" || targetPath == "" || ledgerEpoch == "" {
		return nil
	}
	candidate := projectedExternalActionCandidateByFingerprint(runtimeState, bindingFingerprint)
	if len(candidate) == 0 {
		return nil
	}
	allowedPreparationKeys := map[string]struct{}{}
	for _, hint := range evidenceMapsFromAny(candidate["preparation_hints"]) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(hint["relation"])), "resolve_target") ||
			!projectedExternalPreparationHintTargetsPath(hint, targetPath) {
			continue
		}
		if key := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(hint["action_key"]))); key != "" {
			allowedPreparationKeys[key] = struct{}{}
		}
	}
	if len(allowedPreparationKeys) == 0 {
		return nil
	}
	var latestValues []string
	for _, invocation := range terminalStateGuardExternalInvocations(runtimeState) {
		if !runtimeInvocationSucceeded(invocation) ||
			!projectedExternalPreparationProviderStatusSucceeded(terminalStateGuardExternalActionStatus(invocation)) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillExternalApps) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])), "execute_action") {
			continue
		}
		arguments := evidenceMapFromAny(invocation["arguments"])
		if strings.TrimSpace(evidenceStringFromAny(arguments["plan_phase_id"])) != phaseID ||
			strings.TrimSpace(evidenceStringFromAny(arguments[operationPlanServerProjectedLedgerEpochKey])) != ledgerEpoch ||
			strings.TrimSpace(evidenceStringFromAny(arguments[planExpectedActionServerBindingFingerprintKey])) != bindingFingerprint ||
			strings.TrimSpace(evidenceStringFromAny(arguments[operationPlanServerProjectedConnectionBindingKey])) !=
				strings.TrimSpace(evidenceStringFromAny(candidate[operationPlanServerProjectedConnectionBindingKey])) {
			continue
		}
		result := evidenceMapFromAny(invocation["result"])
		integrationID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["integration_id"])))
		actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["action_id"])))
		if _, allowed := allowedPreparationKeys[integrationID+":"+actionID]; !allowed {
			continue
		}
		observed := evidenceMapFromAny(result[projectedExternalObservedPreparationTargetsKey])
		// A refined preparation call in the same unresolved epoch supersedes an
		// earlier broad result. Otherwise one ambiguous search would poison the
		// phase forever even after the user or model narrows the query.
		latestValues = projectedExternalPreparationDistinctValues(evidenceStringSliceFromAny(observed[targetPath]))
	}
	return latestValues
}

func projectedExternalPreparationHintTargetsPath(hint map[string]interface{}, targetPath string) bool {
	for _, path := range evidenceStringSliceFromAny(hint["target_arguments"]) {
		if strings.TrimSpace(path) == strings.TrimSpace(targetPath) {
			return true
		}
	}
	return false
}

func projectedExpectedActionReplacementIssue(baseline map[string]interface{}, next map[string]interface{}) string {
	baselineKey, external := projectedExternalActionKeyFromExpected(baseline)
	if !external || baselineKey == "" {
		return ""
	}
	nextKey, nextExternal := projectedExternalActionKeyFromExpected(next)
	if !nextExternal || nextKey != baselineKey {
		return "cannot replace its existing server-canonical projected Action identity"
	}
	if baselineFingerprint := strings.TrimSpace(evidenceStringFromAny(baseline[planExpectedActionServerBindingFingerprintKey])); baselineFingerprint != "" &&
		baselineFingerprint != strings.TrimSpace(evidenceStringFromAny(next[planExpectedActionServerBindingFingerprintKey])) {
		return "cannot remove or replace its server projection binding fingerprint"
	}
	for _, field := range []string{"target", "target_arguments"} {
		for key, baselineValue := range evidenceMapFromAny(baseline[field]) {
			want := strings.TrimSpace(evidenceStringFromAny(baselineValue))
			got := strings.TrimSpace(evidenceStringFromAny(evidenceMapFromAny(next[field])[key]))
			matches := want == got
			if field == "target" && (key == "integration_id" || key == "action_id") {
				matches = strings.EqualFold(want, got)
			}
			if want != "" && !matches {
				return "cannot remove or change an existing server-canonical projected Action target"
			}
		}
	}
	return ""
}

func projectedExternalActionPlanRequiresPhaseLedger(runtimeState map[string]interface{}) bool {
	if operationPlanHasServerBoundProjectedPhase(runtimeState) {
		return true
	}
	if len(projectedExternalActionKeys(runtimeState)) == 0 || !projectedExternalActionLedgerMode(runtimeState) {
		return false
	}
	count := 0
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if !operationPlanPhaseRequiredByServer(phase, runtimeState) {
			continue
		}
		if operationPlanPhaseOpenForToolCall(phase) {
			count++
		}
	}
	return count > 0
}

func operationPlanHasServerBoundProjectedPhase(runtimeState map[string]interface{}) bool {
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		expected := evidenceMapFromAny(phase["expected_action"])
		if _, external := projectedExternalActionKeyFromExpected(expected); external &&
			(strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey])) != "" ||
				strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerProjectionKey])) != "") {
			return true
		}
	}
	return false
}

func projectedExternalActionLedgerMode(runtimeState map[string]interface{}) bool {
	if len(projectedExternalActionRequiredPhaseIDs(runtimeState)) > 0 {
		return true
	}
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
		if _, external := projectedExternalActionKeyFromExpected(evidenceMapFromAny(phase["expected_action"])); external {
			return true
		}
	}
	return false
}

func projectedExternalActionRequiredPhaseIDs(runtimeState map[string]interface{}) map[string]struct{} {
	out := map[string]struct{}{}
	phases := evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"])
	for _, requirement := range projectedExternalActionRequirementsForPhases(phases, runtimeState) {
		if phaseID := strings.TrimSpace(requirement.phaseID); phaseID != "" {
			out[phaseID] = struct{}{}
		}
	}
	return out
}

func operationPlanPhaseServerClassifiedNonTool(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["verification_mode"]))) {
	case "final_answer", "non_tool":
		return true
	}
	outcome := operationPlanOutcomeForPhase(phase, runtimeState)
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(outcome["verification_mode"]))) {
	case "final_answer", "non_tool":
		return true
	default:
		return false
	}
}

func operationPlanPhaseRequiredByServer(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	if required, exists := phase["required"].(bool); exists {
		return required
	}
	if required, exists := operationPlanOutcomeForPhase(phase, runtimeState)["required"].(bool); exists {
		return required
	}
	return true
}

func operationPlanOutcomeForPhase(phase map[string]interface{}, runtimeState map[string]interface{}) map[string]interface{} {
	outcomeID := strings.TrimSpace(evidenceStringFromAny(phase["outcome_id"]))
	if outcomeID == "" {
		return nil
	}
	for _, outcome := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["outcomes"]) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(outcome["id"])), outcomeID) {
			return outcome
		}
	}
	return nil
}

func operationPlanPhaseServerRequiresExternalAction(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	for _, capability := range evidenceStringSliceFromAny(operationPlanOutcomeForPhase(phase, runtimeState)["capabilities"]) {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "external-apps", "external_apps", "external-action", "external_action":
			return true
		}
	}
	return false
}

func operationPlanPhaseHasSafeNonToolClassification(phase map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["completion_mode"]))) {
	case "final_answer", "non_tool":
		return true
	default:
		return false
	}
}

func projectedExternalActionKeys(runtimeState map[string]interface{}) map[string]struct{} {
	out := map[string]struct{}{}
	projections := evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionCandidatesKey])
	if len(projections) == 0 {
		projections = evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionProjectionsKey])
	}
	for _, projection := range projections {
		integrationID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["integration_id"])))
		actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["action_id"])))
		if integrationID != "" && actionID != "" {
			out[integrationID+":"+actionID] = struct{}{}
		}
	}
	return out
}

func projectedExternalActionEffect(runtimeState map[string]interface{}, actionKey string) string {
	actionKey = strings.ToLower(strings.TrimSpace(actionKey))
	if actionKey == "" {
		return ""
	}
	projections := evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionCandidatesKey])
	if len(projections) == 0 {
		projections = evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionProjectionsKey])
	}
	for _, projection := range projections {
		integrationID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["integration_id"])))
		actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["action_id"])))
		if integrationID+":"+actionID == actionKey {
			return strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["effect"])))
		}
	}
	return ""
}

func projectedExternalActionKeyFromCall(skillID string, toolName string, arguments map[string]interface{}) string {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillExternalApps) ||
		!strings.EqualFold(strings.TrimSpace(toolName), "execute_action") {
		return ""
	}
	integrationID := strings.ToLower(strings.TrimSpace(operationPlanSkillCallTargetValue(arguments, "integration_id")))
	actionID := strings.ToLower(strings.TrimSpace(operationPlanSkillCallTargetValue(arguments, "action_id")))
	if integrationID == "" || actionID == "" {
		return ""
	}
	return integrationID + ":" + actionID
}

func projectedExternalActionKeyFromExpected(expected map[string]interface{}) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(expected["skill_id"])), skills.SkillExternalApps) ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(expected["tool_name"])), "execute_action") {
		return "", false
	}
	target := evidenceMapFromAny(expected["target"])
	integrationID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(target["integration_id"])))
	actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(target["action_id"])))
	if integrationID == "" || actionID == "" {
		return "", true
	}
	return integrationID + ":" + actionID, true
}

func projectedExternalActionPlanIncompleteStep(callID string, issue string) skillStepResult {
	err := fmt.Errorf("%w: %s", ErrInvalidInput, strings.TrimSpace(issue))
	trace := plannerFeedbackTrace(skills.SkillExternalApps, "execute_action", err)
	trace.Status = "blocked"
	trace.Arguments = map[string]interface{}{
		"code":      projectedExternalActionPlanIncompleteCode,
		"call_id":   strings.TrimSpace(callID),
		"next_step": "declare_complete_projected_action_plan",
	}
	payload := map[string]interface{}{
		"status":      "blocked",
		"code":        projectedExternalActionPlanIncompleteCode,
		"error":       err.Error(),
		"recoverable": true,
		"next_action": "Call update_plan first with every current required phase preserved. Give each phase an expected_action using the exact exposed business function name and stable target IDs, then retry the projected Action in the same response after the plan update.",
	}
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, payload), false, false)
}

func operationPlanSkillCallTarget(arguments map[string]interface{}) map[string]interface{} {
	target := map[string]interface{}{}
	for _, key := range []string{
		"agent_id", "file_id", "asset_id", "resource_id", "dataset_id",
		"data_source_id", "table_id", "workflow_id", "binding_id", "href", "route",
	} {
		if value := operationPlanSkillCallTargetValue(arguments, key); value != "" {
			target[key] = value
		}
	}
	if len(target) == 0 {
		return nil
	}
	return target
}

func projectedExternalActionCallTarget(
	runtimeState map[string]interface{},
	skillID string,
	toolName string,
	arguments map[string]interface{},
) map[string]interface{} {
	actionKey := projectedExternalActionKeyFromCall(skillID, toolName, arguments)
	if actionKey == "" {
		return nil
	}
	businessArguments := evidenceMapFromAny(arguments["arguments"])
	for _, projection := range evidenceMapsFromAny(runtimeState[runtimeStateNativeExternalActionCandidatesKey]) {
		projectionKey := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["integration_id"]))) + ":" +
			strings.ToLower(strings.TrimSpace(evidenceStringFromAny(projection["action_id"])))
		if projectionKey != actionKey {
			continue
		}
		target := map[string]interface{}{}
		for _, path := range evidenceStringSliceFromAny(projection["target_argument_paths"]) {
			path = strings.TrimSpace(path)
			if value := operationPlanArgumentPathValue(businessArguments, path); value != "" {
				target[path] = value
			}
		}
		if len(target) > 0 {
			return target
		}
		return nil
	}
	return nil
}

func normalizeOperationPlanTargetValue(key string, value string) string {
	value = strings.TrimSpace(value)
	if key == "href" || key == "route" {
		if value != "/" {
			value = strings.TrimRight(value, "/")
		}
	}
	return value
}

func operationPlanPhaseMismatchStep(callID string, skillID string, toolName string, arguments map[string]interface{}, err error) skillStepResult {
	trace := plannerFeedbackTrace(skillID, toolName, err)
	trace.Status = "blocked"
	trace.Arguments = map[string]interface{}{
		"code":         operationPlanPhaseMismatchCode,
		"call_id":      strings.TrimSpace(callID),
		"skill_id":     strings.TrimSpace(skillID),
		"tool_name":    strings.TrimSpace(toolName),
		"next_step":    "retry_with_matching_plan_phase",
		"tool_summary": summarizeSkillToolArguments(skillID, toolName, arguments),
	}
	payload := map[string]interface{}{
		"status":      "blocked",
		"code":        operationPlanPhaseMismatchCode,
		"error":       err.Error(),
		"recoverable": true,
		"next_action": "Use the current operation_plan phase whose expected_action exactly matches this skill, tool, and target. Pass its id as plan_phase_id; update the plan first if no unique phase matches.",
	}
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, payload), false, false)
}
