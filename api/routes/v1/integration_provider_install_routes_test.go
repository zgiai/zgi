package v1

import (
	"net/url"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestProviderInstallResultRedirectContainsOnlyBoundedStatus(t *testing.T) {
	result, err := providerInstallResultRedirectURL(
		"https://cloud.vibeclass.cn/console/integrations?provider=wecom",
		integrations.ProviderInstallFlowSucceeded,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("provider") != "wecom" || parsed.Query().Get("install_status") != "succeeded" {
		t.Fatalf("redirect=%s", result)
	}
	for _, forbidden := range []string{"auth_code", "state", "permanent_code", "suite_ticket"} {
		if parsed.Query().Has(forbidden) {
			t.Fatalf("redirect leaked %s: %s", forbidden, result)
		}
	}
}

func TestProviderInstallResultRedirectRejectsUnsafeURL(t *testing.T) {
	if _, err := providerInstallResultRedirectURL("javascript:alert(1)", integrations.ProviderInstallFlowFailed, "auth_invalid"); err == nil {
		t.Fatal("unsafe provider-install result URL was accepted")
	}
}
