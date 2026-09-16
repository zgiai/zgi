package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
)

func TestSystemFeaturesAlwaysExposeConnectors(t *testing.T) {
	previousConfig := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previousConfig })
	t.Setenv("CONNECTORS_ENABLED", "false")

	{
		config.GlobalConfig = &config.Config{}
		if !model.NewDefaultSystemFeature().EnableConnectors {
			t.Fatal("default system features must expose Connectors")
		}
		result, err := NewFeatureService().GetSystemFeatures(context.Background())
		if err != nil {
			t.Fatalf("GetSystemFeatures() error = %v", err)
		}
		features, ok := result.(*model.SystemFeature)
		if !ok {
			t.Fatalf("GetSystemFeatures() type = %T", result)
		}
		if !features.EnableConnectors {
			t.Fatal("retired environment switch must not hide Connectors")
		}

	}
}
