package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	baselinepkg "github.com/zgiai/ginext/internal/migrationsv2/baseline"
	"gorm.io/gorm"
)

type baselineMode string

const (
	baselineModeFresh            baselineMode = "fresh"
	baselineModeBridgeAIYoung    baselineMode = "bridge-ai-young"
	baselineModeBridgeJingzhi    baselineMode = "bridge-jingzhi-dev"
	baselineModeBridgeLegacyTail baselineMode = "bridge-legacy-tail"
)

// M0001_cutover_baseline freezes the ai-young cutover point as the baseline for the new migration chain.
func M0001_cutover_baseline() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2CutoverBaselineID,
		Migrate: func(tx *gorm.DB) error {
			mode, err := resolveBaselineMode(tx)
			if err != nil {
				return err
			}

			switch mode {
			case baselineModeFresh:
				return baselinepkg.ApplySnapshot(tx)
			case baselineModeBridgeAIYoung, baselineModeBridgeJingzhi, baselineModeBridgeLegacyTail:
				return baselinepkg.ValidateBridgeSignature(tx)
			default:
				return fmt.Errorf("unsupported cutover baseline mode %s", mode)
			}
		},
		Rollback: func(tx *gorm.DB) error {
			return fmt.Errorf("rollback of cutover baseline is not supported")
		},
	}
}

func resolveBaselineMode(tx *gorm.DB) (baselineMode, error) {
	if !tx.Migrator().HasTable("migrations") {
		return baselineModeFresh, nil
	}

	appliedIDs, err := appliedMigrationIDs(tx)
	if err != nil {
		return "", fmt.Errorf("load migration ids for cutover baseline: %w", err)
	}

	if len(appliedIDs) == 0 {
		return baselineModeFresh, nil
	}

	switch {
	case isJingzhiBridgeState(appliedIDs):
		return baselineModeBridgeJingzhi, nil
	case isLegacyTailBridgeState(appliedIDs):
		return baselineModeBridgeLegacyTail, nil
	case isAiYoungBridgeState(appliedIDs):
		return baselineModeBridgeAIYoung, nil
	case hasAnyPostCutoverLegacyIDs(appliedIDs):
		return "", fmt.Errorf("database already contains unsupported legacy post-cutover migrations; migrate it to ai-young tip or jingzhi-dev tip before entering migrationsv2")
	default:
		return baselineModeFresh, nil
	}
}
