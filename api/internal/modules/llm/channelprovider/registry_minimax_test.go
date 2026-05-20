package channelprovider

import "testing"

func TestResolve_MiniMaxAliasesUseMiniMaxAdapter(t *testing.T) {
	t.Helper()

	cases := []struct {
		input          string
		wantName       string
		wantAdapterKey string
		wantLookup     string
	}{
		{input: "minimax", wantName: "minimax", wantAdapterKey: "minimax", wantLookup: "minimax"},
		{input: "minmax", wantName: "minimax", wantAdapterKey: "minimax", wantLookup: "minimax"},
	}

	for _, tc := range cases {
		spec, err := Resolve(tc.input)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", tc.input, err)
		}
		if spec.Name != tc.wantName {
			t.Fatalf("Resolve(%q).Name = %q, want %q", tc.input, spec.Name, tc.wantName)
		}
		if spec.AdapterKey != tc.wantAdapterKey {
			t.Fatalf("Resolve(%q).AdapterKey = %q, want %q", tc.input, spec.AdapterKey, tc.wantAdapterKey)
		}
		if spec.LookupProvider != tc.wantLookup {
			t.Fatalf("Resolve(%q).LookupProvider = %q, want %q", tc.input, spec.LookupProvider, tc.wantLookup)
		}
	}
}
