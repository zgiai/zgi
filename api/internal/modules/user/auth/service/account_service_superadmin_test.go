package service

import (
	"testing"

	"github.com/zgiai/zgi/api/config"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
)

func TestFrontendSuperAdminFlagReturnsValueInSelfHostedMode(t *testing.T) {
	setAccountServiceTestEdition(t, "SELF_HOSTED")

	flag := frontendSuperAdminFlag(&auth_model.Account{IsSuperAdmin: true})
	if flag == nil {
		t.Fatal("frontendSuperAdminFlag() = nil, want non-nil in self-hosted mode")
	}
	if !*flag {
		t.Fatal("frontendSuperAdminFlag() = false, want true")
	}
}

func TestFrontendSuperAdminFlagReturnsFalsePointerForRegularAccountInSelfHostedMode(t *testing.T) {
	setAccountServiceTestEdition(t, "SELF_HOSTED")

	flag := frontendSuperAdminFlag(&auth_model.Account{IsSuperAdmin: false})
	if flag == nil {
		t.Fatal("frontendSuperAdminFlag() = nil, want non-nil in self-hosted mode")
	}
	if *flag {
		t.Fatal("frontendSuperAdminFlag() = true, want false")
	}
}

func TestFrontendSuperAdminFlagOmitsValueInCloudMode(t *testing.T) {
	setAccountServiceTestEdition(t, "CLOUD")

	flag := frontendSuperAdminFlag(&auth_model.Account{IsSuperAdmin: true})
	if flag != nil {
		t.Fatalf("frontendSuperAdminFlag() = %v, want nil in cloud mode", *flag)
	}
}

func setAccountServiceTestEdition(t *testing.T, edition string) {
	t.Helper()

	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Platform: config.PlatformConfig{
			Edition: edition,
		},
	}

	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}
