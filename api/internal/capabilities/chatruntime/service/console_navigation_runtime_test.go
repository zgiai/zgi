package service

import "testing"

func TestConsoleNavigationRouteHintsExcludeUnsupportedIntegrations(t *testing.T) {
	for _, hint := range consoleNavigationRouteHints {
		if hint.Href == "/console/integrations" {
			t.Fatalf("unsupported main route advertised by runtime hint: %#v", hint)
		}
	}
}
