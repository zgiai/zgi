package interfaces

import (
	"context"
)

// FeatureService defines system feature related business logic
type FeatureService interface {
	GetSystemFeatures(ctx context.Context) (interface{}, error)
	IsPublicDeployment() bool
	IsFeatureEnabled(featureName string) bool
}
