package dingtalk

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

func TestActionsDeclareExplicitBilingualIntentAliases(t *testing.T) {
	assertBilingualIntentAliases(t, actions())
}

func assertBilingualIntentAliases(t *testing.T, actions []integrations.ActionDefinition) {
	t.Helper()
	for _, action := range actions {
		hasASCII, hasNonASCII := false, false
		if len(action.IntentAliases) < 2 {
			t.Fatalf("Action %s intent aliases = %#v, want explicit bilingual aliases", action.ID, action.IntentAliases)
		}
		for _, alias := range action.IntentAliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || utf8.RuneCountInString(alias) > 64 || strings.EqualFold(alias, strings.TrimSpace(action.Description)) {
				t.Fatalf("Action %s has invalid or description-sized intent alias %q", action.ID, alias)
			}
			for _, current := range alias {
				if current <= unicode.MaxASCII && unicode.IsLetter(current) {
					hasASCII = true
				}
				if current > unicode.MaxASCII && unicode.IsLetter(current) {
					hasNonASCII = true
				}
			}
		}
		if !hasASCII || !hasNonASCII {
			t.Fatalf("Action %s intent aliases are not bilingual: %#v", action.ID, action.IntentAliases)
		}
	}
}
