package channelprovider

import "testing"

func TestNormalizeIdentityKeyFoldsSupplierSpelling(t *testing.T) {
	cases := map[string]string{
		"agicto":            "agicto",
		"Agicto Supplier":   "agicto",
		"agicto-supplier":   "agicto",
		"agicto_supplier":   "agicto",
		"agicto 供应商":        "agicto",
		"agicto供应商":         "agicto",
		"  agicto channel ": "agicto",
		"zhipu":             "glm",
		"Z.AI":              "glm",
		"":                  "",
		"   ":               "",
	}
	for input, want := range cases {
		if got := NormalizeIdentityKey(input); got != want {
			t.Errorf("NormalizeIdentityKey(%q) = %q, want %q", input, got, want)
		}
	}
}
