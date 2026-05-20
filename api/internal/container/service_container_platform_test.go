package container

import (
	"testing"

	"github.com/zgiai/ginext/internal/infra/platform"
)

func TestGetPlatformChannels_ReturnsErrorWhenUninitialized(t *testing.T) {
	c := &ServiceContainer{}

	_, err := c.GetPlatformChannels()
	if err == nil {
		t.Fatalf("expected error when platform container is not initialized")
	}
}

func TestGetPlatformChannels_ReturnsPlatformContainerWhenInitialized(t *testing.T) {
	pc := &platform.Container{}
	c := &ServiceContainer{
		platformContainer: pc,
	}

	got, err := c.GetPlatformChannels()
	if err != nil {
		t.Fatalf("GetPlatformChannels returned error: %v", err)
	}
	if got.Console != pc.Console || got.Channel != pc.Channel || got.Billing != pc.Billing || got.Identity != pc.Identity {
		t.Fatalf("GetPlatformChannels returned unexpected container")
	}
}
