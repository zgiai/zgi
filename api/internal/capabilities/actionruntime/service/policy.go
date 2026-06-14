package service

import (
	"strings"

	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
)

type planPolicy struct {
	RiskLevel            string
	RequiresConfirmation bool
}

func evaluatePlanPolicy(capability CapabilityManifest, requestedRisk string, requestedConfirmation *bool) planPolicy {
	risk := higherRisk(capability.RiskLevel, requestedRisk)
	requiresConfirmation := capability.RequiresConfirmation || riskRequiresConfirmation(risk)
	if requestedConfirmation != nil && *requestedConfirmation {
		requiresConfirmation = true
	}
	return planPolicy{
		RiskLevel:            risk,
		RequiresConfirmation: requiresConfirmation,
	}
}

func normalizeRiskLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case actionmodel.RiskLevelCritical:
		return actionmodel.RiskLevelCritical
	case actionmodel.RiskLevelHigh:
		return actionmodel.RiskLevelHigh
	case actionmodel.RiskLevelMedium:
		return actionmodel.RiskLevelMedium
	default:
		return actionmodel.RiskLevelLow
	}
}

func higherRisk(left string, right string) string {
	leftRank := riskRank(normalizeRiskLevel(left))
	rightRisk := normalizeRiskLevel(right)
	if riskRank(rightRisk) > leftRank {
		return rightRisk
	}
	return normalizeRiskLevel(left)
}

func riskRequiresConfirmation(risk string) bool {
	return riskRank(normalizeRiskLevel(risk)) >= riskRank(actionmodel.RiskLevelMedium)
}

func riskRank(risk string) int {
	switch risk {
	case actionmodel.RiskLevelCritical:
		return 4
	case actionmodel.RiskLevelHigh:
		return 3
	case actionmodel.RiskLevelMedium:
		return 2
	default:
		return 1
	}
}
