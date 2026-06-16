package routing

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestPlannerPrefersConfiguredProvidersBeforeFallback(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "vlm", Enabled: true, Priority: 1100, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineVLM},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineReducto},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileHighQuality,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "reducto" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	if len(plan.FallbackCandidates) == 0 || plan.FallbackCandidates[len(plan.FallbackCandidates)-1].ProviderKey != "vlm" {
		t.Fatalf("fallbacks=%+v", plan.FallbackCandidates)
	}
}

func TestPlannerUsesFallbackChainWhenNoConfiguredProviders(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "vlm", Enabled: true, Priority: 1100, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineVLM},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileAuto,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "local" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
}

func TestPlannerHonorsLocalFirstProfile(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileLocalFirst,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "local" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	for _, item := range plan.FallbackCandidates {
		if item.ProviderKey == "mineru" {
			t.Fatalf("unexpected remote fallback in local_first plan: %+v", plan.FallbackCandidates)
		}
	}
}

func TestPlannerSkipsUnhealthyAdapters(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "remote_adapter", Engine: contracts.ParseEngineReducto},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "healthy_adapter", Engine: contracts.ParseEngineMineru},
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "healthy_adapter", Engine: contracts.ParseEngineLocal},
		},
	}
	health := &contracts.ParseHealth{
		Adapters: []contracts.AdapterHealth{
			{Name: "remote_adapter", Available: false},
			{Name: "healthy_adapter", Available: true},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileHighQuality,
	}, catalog, health)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "mineru" {
		t.Fatalf("expected healthy provider primary, got %+v", plan.Primary)
	}
	for _, candidate := range plan.FallbackCandidates {
		if candidate.ProviderKey == "reducto" {
			t.Fatalf("unhealthy provider should not be a fallback: %+v", plan.FallbackCandidates)
		}
	}
}

func TestPlannerRoutesPDFByFileExtension(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "vlm", Enabled: true, Priority: 1100, FallbackOnly: true, Adapter: "system_vlm", Engine: contracts.ParseEngineVLM},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineReducto},
			{Name: "hyperparse_api", Enabled: true, Priority: 300, Adapter: "hyperparse_api"},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		FileName: "statement.PDF",
		Profile:  contracts.ParseProfileDatasetIndex,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "reducto" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	got := providerKeys(plan.FallbackCandidates)
	want := []string{"mineru", "vlm", "local"}
	if !sameStringSlice(got, want) {
		t.Fatalf("fallbacks=%v, want %v", got, want)
	}
	if plan.Metadata["selection"] != "file_extension_auto_route" || plan.Metadata["file_ext"] != ".pdf" {
		t.Fatalf("metadata=%+v", plan.Metadata)
	}
}

func TestPlannerRoutesWordByFileExtension(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineReducto},
			{Name: "hyperparse_api", Enabled: true, Priority: 300, Adapter: "hyperparse_api"},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		FileName: "contract.docx",
		Profile:  contracts.ParseProfileDatasetIndex,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "reducto" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	got := providerKeys(plan.FallbackCandidates)
	want := []string{"mineru", "local"}
	if !sameStringSlice(got, want) {
		t.Fatalf("fallbacks=%v, want %v", got, want)
	}
}

func TestFileExtensionProviderOrder(t *testing.T) {
	cases := []struct {
		fileName string
		wantExt  string
		want     []string
	}{
		{"report.pdf", ".pdf", []string{"reducto", "mineru", "vlm", "local"}},
		{"lesson.docx", ".docx", []string{"reducto", "mineru", "local"}},
		{"deck.ppt", ".ppt", []string{"reducto", "mineru"}},
		{"sheet.csv", ".csv", []string{"local"}},
		{"scan.png", ".png", []string{"vlm", "mineru"}},
		{"notes.md", ".md", []string{"local"}},
		{"archive.bin", ".bin", []string{"local"}},
	}

	for _, tc := range cases {
		got, gotExt := FileExtensionProviderOrder(tc.fileName)
		if gotExt != tc.wantExt {
			t.Fatalf("%s ext=%q want %q", tc.fileName, gotExt, tc.wantExt)
		}
		if !sameStringSlice(got, tc.want) {
			t.Fatalf("%s providers=%v want %v", tc.fileName, got, tc.want)
		}
	}
}

func TestPlannerRoutesUnknownExtensionToLocalOnly(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineReducto},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		FileName: "blob.unknown",
		Profile:  contracts.ParseProfileDatasetIndex,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "local" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	if len(plan.FallbackCandidates) != 0 {
		t.Fatalf("fallbacks=%+v, want none", plan.FallbackCandidates)
	}
}

func TestPlannerFileExtensionRouteSkipsUnavailableProvider(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "healthy_adapter", Engine: contracts.ParseEngineLocal},
			{Name: "mineru", Enabled: false, Priority: 200, Adapter: "healthy_adapter", Engine: contracts.ParseEngineMineru},
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "unhealthy_adapter", Engine: contracts.ParseEngineReducto},
			{Name: "hyperparse_api", Enabled: true, Priority: 300, Adapter: "healthy_adapter"},
		},
	}
	health := &contracts.ParseHealth{
		Adapters: []contracts.AdapterHealth{
			{Name: "healthy_adapter", Available: true},
			{Name: "unhealthy_adapter", Available: false},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		FileName: "report.pdf",
		Profile:  contracts.ParseProfileDatasetIndex,
	}, catalog, health)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "local" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	got := providerKeys(plan.FallbackCandidates)
	want := []string{}
	if !sameStringSlice(got, want) {
		t.Fatalf("fallbacks=%v, want %v", got, want)
	}
}

func TestPlannerLocalFirstIgnoresFileExtensionRoute(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		FileName: "report.pdf",
		Profile:  contracts.ParseProfileLocalFirst,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "local" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
}

func providerKeys(candidates []RouteCandidate) []string {
	keys := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		keys = append(keys, candidate.ProviderKey)
	}
	return keys
}

func sameStringSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
