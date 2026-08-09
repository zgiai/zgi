package transport_test

import (
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func TestLocaleFromAcceptLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   catalog.Locale
	}{
		{name: "simplified Chinese", header: "zh-CN,zh;q=0.9,en;q=0.8", want: catalog.LocaleChineseSimplified},
		{name: "quality order", header: "zh-Hans;q=0.4,en-US;q=0.8", want: catalog.LocaleEnglishUS},
		{name: "underscore alias", header: "zh_CN;q=0.7,en;q=0.6", want: catalog.LocaleChineseSimplified},
		{name: "zero quality skipped", header: "zh-Hans;q=0,en;q=0.5", want: catalog.LocaleEnglishUS},
		{name: "invalid quality skipped", header: "zh-Hans;q=2,en;q=0.5", want: catalog.LocaleEnglishUS},
		{name: "NaN quality skipped", header: "en-US;q=1,zh-Hans;q=NaN", want: catalog.LocaleEnglishUS},
		{name: "infinite quality skipped", header: "en-US;q=0.8,zh-Hans;q=+Inf", want: catalog.LocaleEnglishUS},
		{name: "unsupported", header: "fr-FR,de;q=0.8", want: ""},
		{name: "empty", header: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := transport.LocaleFromAcceptLanguage(test.header); got != test.want {
				t.Fatalf("LocaleFromAcceptLanguage(%q) = %q, want %q", test.header, got, test.want)
			}
		})
	}
}
