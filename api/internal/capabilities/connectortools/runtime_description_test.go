package connectortools

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

func TestConnectorOperationEntityRuntimeDescriptionIncludesChineseNames(t *testing.T) {
	definition, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range definition.Operations {
		entity, err := OperationEntity(definition, operation.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(entity.Description.LLM, operation.Description) ||
			!strings.Contains(entity.Description.LLM, operation.NameI18n["zh-Hans"]) ||
			!strings.Contains(entity.Description.LLM, definition.NameI18n["zh-Hans"]) {
			t.Errorf("%s: model description must retain English and add Chinese names: %q", operation.ID, entity.Description.LLM)
		}
		if strings.Contains(entity.Description.LLM, operation.DescriptionI18n["zh-Hans"]) {
			t.Errorf("%s: full localized descriptions must not influence lexical discovery", operation.ID)
		}
		if !reflect.DeepEqual(entity.Description.Human, labels(operation.Description, operation.DescriptionI18n)) {
			t.Fatalf("%s: model discovery changed human-facing copy", operation.ID)
		}
	}
}

func TestConnectorOperationRuntimeDescriptionLocaleFallbackAndDeduplication(t *testing.T) {
	for _, test := range []struct {
		name      string
		localized catalog.LocalizedText
		want      string
	}{
		{"canonical", catalog.LocalizedText{"zh-Hans": " 查找用户 "}, "查找用户"},
		{"region alias", catalog.LocalizedText{"zh-CN": "查找用户"}, "查找用户"},
		{"language alias", catalog.LocalizedText{"zh": "查找用户"}, "查找用户"},
		{"case insensitive", catalog.LocalizedText{"ZH-hANS": "查找用户"}, "查找用户"},
		{"no Chinese translation", catalog.LocalizedText{"en-US": "Find people", "fr": "Chercher"}, ""},
		{"underscore is not a catalog locale", catalog.LocalizedText{"zh_Hans": "查找用户"}, ""},
		{"no traditional fallback", catalog.LocalizedText{"zh-Hant": "查找用戶"}, ""},
		{"empty", catalog.LocalizedText{"zh-Hans": " \n "}, ""},
		{"fixed precedence", catalog.LocalizedText{"zh": "language", "zh-CN": "region", "zh-Hans": "canonical"}, "canonical"},
		{"blank canonical falls back", catalog.LocalizedText{"zh-Hans": " ", "zh-CN": "region"}, "region"},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := catalog.Definition{}
			operation := catalog.Operation{Name: "English name", Description: "English safety instructions.", NameI18n: test.localized,
				DescriptionI18n: catalog.LocalizedText{"zh-Hans": "不会发送消息的完整中文描述"}}
			want := operation.Description
			if test.want != "" {
				want += "\n\n" + test.want
			}
			for i := 0; i < 20; i++ {
				if got := OperationRuntimeDescription(definition, operation); got != want {
					t.Fatalf("got %q, want deterministic %q", got, want)
				}
			}
		})
	}
	operation := catalog.Operation{Name: "English name", Description: " \n ", NameI18n: catalog.LocalizedText{"zh-Hans": " 同名 "}}
	definition := catalog.Definition{NameI18n: catalog.LocalizedText{"zh": "同名"}}
	if got := OperationRuntimeDescription(definition, operation); got != "English name\n\n同名" {
		t.Fatalf("English name fallback or name deduplication failed: %q", got)
	}
}

func TestConnectorOperationRuntimeDescriptionBoundsOnlyAddedNames(t *testing.T) {
	definition := catalog.Definition{NameI18n: catalog.LocalizedText{"zh-Hans": strings.Repeat("连接器", 200)}}
	operation := catalog.Operation{Description: strings.Repeat("Do not expose secrets. ", 1000) + "Keep all restrictions.", NameI18n: catalog.LocalizedText{"zh-Hans": "按姓名查找用户"}}
	originalDefinition, originalOperation := definition.NameI18n.Clone(), operation.NameI18n.Clone()
	got := OperationRuntimeDescription(definition, operation)
	if !strings.HasPrefix(got, operation.Description+"\n\n按姓名查找用户") {
		t.Fatal("connector name crowded out the action or truncated English safety instructions")
	}
	if len([]rune(got))-len([]rune(operation.Description)) > operationLocalizedNameBudget || !utf8.ValidString(got) {
		t.Fatal("localized suffix exceeded its Unicode bound or split a character")
	}
	if !reflect.DeepEqual(definition.NameI18n, originalDefinition) || !reflect.DeepEqual(operation.NameI18n, originalOperation) {
		t.Fatal("presentation mutated an admitted catalog definition")
	}
	operation.NameI18n["zh-Hans"] = strings.Repeat("查", 500)
	got = OperationRuntimeDescription(definition, operation)
	if got != operation.Description+"\n\n"+strings.Repeat("查", operationLocalizedNameBudget-2) {
		t.Fatal("oversized labels did not reserve all available suffix space for the action first")
	}
}

func TestDigitalHumanTargetMarkerIsDeterministicAndOpaque(t *testing.T) {
	const connectionID = "5b0f8a3e-1c2d-4e5f-8a9b-0123456789ab"
	parsed := uuid.MustParse(connectionID)
	marker := digitalHumanTargetMarker(parsed)
	digest := sha256.Sum256([]byte(connectionID))
	want := hex.EncodeToString(digest[:])[:digitalHumanTargetMarkerHexLength]
	if marker != want {
		t.Fatalf("marker = %q, want deterministic sha256 prefix %q", marker, want)
	}
	if len(marker) != 10 {
		t.Fatalf("marker = %q, want 10 hex characters", marker)
	}
	for _, character := range marker {
		if !strings.ContainsRune("0123456789abcdef", character) {
			t.Fatalf("marker = %q, want lowercase hex only", marker)
		}
	}
	if strings.Contains(marker, connectionID) || strings.Contains(marker, strings.ReplaceAll(connectionID, "-", "")) {
		t.Fatalf("marker exposed the connection UUID: %q", marker)
	}
	// Equivalent parseable spellings derive the same marker because the digest
	// is taken over the canonical UUID, never the caller's raw spelling.
	if other := digitalHumanTargetMarker(uuid.MustParse(strings.ToUpper(connectionID))); other != marker {
		t.Fatalf("equivalent UUID spellings derived different markers: %q vs %q", marker, other)
	}
}

func TestDigitalHumanConnectionRuntimeDescriptionSanitizesUntrustedLabel(t *testing.T) {
	const (
		connectionID = "5b0f8a3e-1c2d-4e5f-8a9b-0123456789ab"
		base         = "Read the connected account."
	)
	marker := digitalHumanTargetMarker(uuid.MustParse(connectionID))
	for _, test := range []struct {
		name           string
		label          string
		wantLabel      string
		wantMarkerOnly bool
	}{
		{name: "plain label", label: "Work GitHub", wantLabel: `"Work GitHub"`},
		{name: "blank label falls back to the marker", label: " \t ", wantMarkerOnly: true},
		{name: "controls and bidi formatting are stripped", label: "Work\x00\x1b[31m\u202eGitHub\u2069", wantLabel: `"Work [31m GitHub"`},
		{name: "quotes and backslashes are escaped", label: `he said "hi" \ bye`, wantLabel: `"he said \"hi\" \\ bye"`},
		{name: "line breaks collapse into a space", label: "Work\nGitHub\tMain", wantLabel: `"Work GitHub Main"`},
		{name: "zero width joiners are stripped", label: "Wo\u200drd", wantLabel: `"Wo rd"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := base + "\n\n"
			if test.wantMarkerOnly {
				want += "Digital Human server target marker: " + marker + "."
			} else {
				want += "Digital Human connection label: " + test.wantLabel +
					" (server target marker " + marker + ")."
			}
			got, err := DigitalHumanConnectionRuntimeDescription(base, connectionID, test.label)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("description = %q, want %q", got, want)
			}
			if strings.Contains(got, connectionID) || strings.Contains(got, strings.ReplaceAll(connectionID, "-", "")) {
				t.Fatalf("description exposed the connection UUID: %q", got)
			}
			if strings.Contains(got, "connection id") {
				t.Fatalf("description still presented the target as a connection id: %q", got)
			}
			// Only the identity sentence is user-controlled; the admitted base
			// description keeps its own line breaks.
			for _, character := range strings.TrimPrefix(got, base+"\n\n") {
				if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
					t.Fatalf("description retained a control or bidi character: %q", got)
				}
			}
		})
	}
}

func TestDigitalHumanConnectionRuntimeDescriptionBoundsLongLabel(t *testing.T) {
	const connectionID = "5b0f8a3e-1c2d-4e5f-8a9b-0123456789ab"
	marker := digitalHumanTargetMarker(uuid.MustParse(connectionID))
	longLabel := strings.Repeat("图", 200)
	got, err := DigitalHumanConnectionRuntimeDescription("Read the connected account.", connectionID, longLabel)
	if err != nil {
		t.Fatal(err)
	}
	bounded := strings.Repeat("图", connectionLabelBudget)
	want := "Read the connected account.\n\nDigital Human connection label: \"" + bounded +
		"\" (server target marker " + marker + ")."
	if got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Fatal("bounding split a character")
	}
	if strings.Contains(got, strings.Repeat("图", connectionLabelBudget+1)) {
		t.Fatal("label exceeded its rune budget")
	}
	if strings.Contains(got, connectionID) {
		t.Fatalf("description exposed the connection UUID: %q", got)
	}
}

func TestDigitalHumanConnectionRuntimeDescriptionEdgeCases(t *testing.T) {
	const connectionID = "5b0f8a3e-1c2d-4e5f-8a9b-0123456789ab"
	marker := digitalHumanTargetMarker(uuid.MustParse(connectionID))
	got, err := DigitalHumanConnectionRuntimeDescription(" \n ", connectionID, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Digital Human connection label: \"Work\" (server target marker " + marker + ")."; got != want {
		t.Fatalf("empty base did not produce a standalone identity sentence: %q", got)
	}
	got, err = DigitalHumanConnectionRuntimeDescription("Read.", connectionID, "\u202e \u2069")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Read.\n\nDigital Human server target marker: " + marker + "."; got != want {
		t.Fatalf("blank sanitized label did not fall back to the marker: %q", got)
	}
}

func TestDigitalHumanConnectionRuntimeDescriptionRejectsInvalidIdentity(t *testing.T) {
	for _, connectionID := range []string{
		"",
		" \t ",
		"not-a-uuid",
		"draft-1",
		uuid.Nil.String(),
		"{5b0f8a3e-1c2d-4e5f-8a9b-0123456789ab",
	} {
		t.Run(connectionID, func(t *testing.T) {
			got, err := DigitalHumanConnectionRuntimeDescription("Read the connected account.", connectionID, "Work")
			if !errors.Is(err, errInvalidDigitalHumanConnectionIdentity) {
				t.Fatalf("error = %v, want fail-closed identity error", err)
			}
			if got != "" {
				t.Fatalf("invalid identity produced model-visible text: %q", got)
			}
			if connectionID != "" && strings.Contains(err.Error(), connectionID) {
				t.Fatalf("error echoed the rejected identity: %v", err)
			}
		})
	}
}
