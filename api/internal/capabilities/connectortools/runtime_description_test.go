package connectortools

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

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
