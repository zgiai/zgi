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
		{name: "three digit quality", header: "en-US;q=0.900,zh-Hans;q=0.901", want: catalog.LocaleChineseSimplified},
		{name: "one with zero fraction", header: "en-US;q=0.9,zh-Hans;q=1.000", want: catalog.LocaleChineseSimplified},
		{name: "zero with empty fraction", header: "zh-Hans;q=0.,en-US;q=0.1", want: catalog.LocaleEnglishUS},
		{name: "underscore alias", header: "zh_CN;q=0.7,en;q=0.6", want: catalog.LocaleChineseSimplified},
		{name: "zero quality skipped", header: "zh-Hans;q=0,en;q=0.5", want: catalog.LocaleEnglishUS},
		{name: "invalid quality skipped", header: "zh-Hans;q=2,en;q=0.5", want: catalog.LocaleEnglishUS},
		{name: "NaN quality skipped", header: "en-US;q=1,zh-Hans;q=NaN", want: catalog.LocaleEnglishUS},
		{name: "infinite quality skipped", header: "en-US;q=0.8,zh-Hans;q=+Inf", want: catalog.LocaleEnglishUS},
		{name: "bare quality skipped", header: "en-US;q=0.8,zh-Hans;q", want: catalog.LocaleEnglishUS},
		{name: "empty parameter skipped", header: "en-US;q=0.8,zh-Hans;", want: catalog.LocaleEnglishUS},
		{name: "unknown parameter skipped", header: "en-US;q=0.8,zh-Hans;level=1", want: catalog.LocaleEnglishUS},
		{name: "duplicate quality skipped", header: "en-US;q=0.8,zh-Hans;q=0.9;q=0.7", want: catalog.LocaleEnglishUS},
		{name: "exponent quality skipped", header: "en-US;q=0.9,zh-Hans;q=1e0", want: catalog.LocaleEnglishUS},
		{name: "leading decimal quality skipped", header: "en-US;q=0.9,zh-Hans;q=.5", want: catalog.LocaleEnglishUS},
		{name: "signed quality skipped", header: "en-US;q=0.9,zh-Hans;q=+0.5", want: catalog.LocaleEnglishUS},
		{name: "overprecise quality skipped", header: "en-US;q=0.9,zh-Hans;q=0.9999", want: catalog.LocaleEnglishUS},
		{name: "one with nonzero fraction skipped", header: "en-US;q=0.9,zh-Hans;q=1.001", want: catalog.LocaleEnglishUS},
		{name: "specific exclusion overrides broad range", header: "en-US;q=0,en;q=1,zh;q=0.5", want: catalog.LocaleChineseSimplified},
		{name: "specific preference overrides broad exclusion", header: "en;q=0,en-US;q=0.7,zh;q=0.5", want: catalog.LocaleEnglishUS},
		{name: "wildcard supplies unmatched locale", header: "*;q=1,en-US;q=0", want: catalog.LocaleChineseSimplified},
		{name: "wildcard tie uses stable supported order", header: "*;q=1", want: catalog.LocaleEnglishUS},
		{name: "specific ranges override wildcard", header: "*;q=1,zh-Hans;q=0,en;q=0.5", want: catalog.LocaleEnglishUS},
		{name: "higher quality wins between equal aliases", header: "zh-CN;q=0.1,zh-Hans;q=0.9,en;q=0.5", want: catalog.LocaleChineseSimplified},
		{name: "header order breaks equal quality tie", header: "zh;q=0.8,en;q=0.8", want: catalog.LocaleChineseSimplified},
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

func BenchmarkLocaleFromAcceptLanguage(b *testing.B) {
	const header = "en-US;q=0, en;q=0.8, zh-Hans;q=0.7"
	b.ReportAllocs()
	for range b.N {
		_ = transport.LocaleFromAcceptLanguage(header)
	}
}
