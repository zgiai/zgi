package service

import "testing"

func TestConsoleNavigationRouteHintsIncludeIntegrations(t *testing.T) {
	hint := consoleNavigationRouteHintForHref("/console/integrations")
	if hint.Href != "/console/integrations" || hint.Label != "集成管理" {
		t.Fatalf("integration route hint = %#v, want registered integration management route", hint)
	}
}
