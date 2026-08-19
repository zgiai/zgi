package service

import (
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
)

func TestFeatureServiceExposesDigitalHumanFlag(t *testing.T) {
	original := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = original })
	config.GlobalConfig = &config.Config{Feature: config.FeatureConfig{DigitalHumanEnabled: true}}

	service := NewFeatureService()
	features, err := service.GetSystemFeatures(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	systemFeatures, ok := features.(*model.SystemFeature)
	if !ok {
		t.Fatalf("features type = %T", features)
	}
	if !systemFeatures.EnableDigitalHuman || !service.IsFeatureEnabled("digital_human") {
		t.Fatalf("features = %#v", systemFeatures)
	}
}
