package zgimarket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeInstaller struct {
	packages []SkillPackage
	result   *SkillInstall
}

func (f *fakeInstaller) Install(_ context.Context, _, _ string, pkg SkillPackage, _ bool) (*SkillInstall, error) {
	f.packages = append(f.packages, pkg)
	return f.result, nil
}

func (f *fakeInstaller) Installed(context.Context, string, string) ([]InstalledSkill, error) {
	return []InstalledSkill{{Slug: "csv", SkillID: "csv-summary", Name: "CSV", Version: "1.0.0"}}, nil
}

// installListing is the skill listing installConsole's market answers with.
var installListing = `{"id":7,"current_version":{"label":"1.2.0"}}`

// installConsole is a console whose market holds one skill listing. subscription is the status
// the market answers an install with, and digest the digest it states for the package.
func installConsole(t *testing.T, member, admin bool, subscription int, digest string) (*gin.Engine, *fakeInstaller, *[]string) {
	t.Helper()
	calls := []string{}
	console := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, consoleProxyPath)
		calls = append(calls, r.Method+" "+path+" as "+r.Header.Get(headerOrganizationRole))
		switch path {
		case "listings/skill/csv":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"data":`+installListing+`}`) //nolint:errcheck // test server
		case "subscriptions":
			body, _ := io.ReadAll(r.Body) //nolint:errcheck // test server
			calls = append(calls, "body "+string(body))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(subscription)
			_, _ = io.WriteString(w, `{"code":402,"error_code":"BILLING.INSUFFICIENT_BALANCE"}`) //nolint:errcheck // test server
		case "skills/csv/package":
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set(skillSHAHeader, digest)
			_, _ = io.WriteString(w, "zip-bytes") //nolint:errcheck // test server
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(console.Close)
	installer := &fakeInstaller{result: &SkillInstall{Installed: true, SkillID: "csv-summary", Name: "CSV", Version: "1.2.0"}}
	proxy := NewProxy(
		Config{ConsoleURL: console.URL, InternalKey: "shared-key"},
		func(c *gin.Context) string { return c.GetHeader("X-Test-Organization") },
		func(c *gin.Context) string { return c.GetHeader("X-Test-Account") },
		func(*gin.Context, string, string) (bool, bool, error) { return member, admin, nil },
	).WithSkillInstaller(installer)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/workspaces/current/zgi-market/*path", proxy.Forward)
	return engine, installer, &calls
}

func packageDigest() string {
	sum := sha256.Sum256([]byte("zip-bytes"))
	return hex.EncodeToString(sum[:])
}

func TestInstallSkillActivatesTheListingThenImportsItsPackage(t *testing.T) {
	engine, installer, calls := installConsole(t, true, true, http.StatusCreated, packageDigest())
	answer := send(engine, http.MethodPost, "/workspaces/current/zgi-market/installs/skills/csv", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", answer.Code, answer.Body.String())
	}
	want := []string{"GET listings/skill/csv as admin", "POST subscriptions as admin", `body {"listing_id":7}`, "GET skills/csv/package as admin"}
	if strings.Join(*calls, "|") != strings.Join(want, "|") {
		t.Fatalf("console calls = %v, want %v", *calls, want)
	}
	if len(installer.packages) != 1 {
		t.Fatalf("imported %d packages, want 1", len(installer.packages))
	}
	pkg := installer.packages[0]
	if pkg.Slug != "csv" || pkg.Version != "1.2.0" || pkg.SHA256 != packageDigest() || string(pkg.Data) != "zip-bytes" || pkg.Filename != "csv.zip" {
		t.Fatalf("imported package = %+v", pkg)
	}
	var body struct {
		Data SkillInstall `json:"data"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &body); err != nil || !body.Data.Installed || body.Data.SkillID != "csv-summary" {
		t.Fatalf("answer = %s (%v)", answer.Body.String(), err)
	}
}

func TestInstallSkillIsForAdministratorsOnly(t *testing.T) {
	engine, installer, calls := installConsole(t, true, false, http.StatusCreated, packageDigest())
	answer := send(engine, http.MethodPost, "/workspaces/current/zgi-market/installs/skills/csv", "", nil)
	// The listing names no price, so it is not free: the member is refused once it is read.
	if answer.Code != http.StatusForbidden || strings.Join(*calls, ",") != "GET listings/skill/csv as member" || len(installer.packages) != 0 {
		t.Fatalf("status = %d, calls = %v, imports = %d; want a refusal after reading the listing", answer.Code, *calls, len(installer.packages))
	}
	// A member still sees what is installed.
	listed := send(engine, http.MethodGet, "/workspaces/current/zgi-market/installs/skills", "", nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"skill_id":"csv-summary"`) {
		t.Fatalf("installed list = %d %s", listed.Code, listed.Body.String())
	}
}

func TestInstallSkillStopsWhenTheMarketRefusesTheInstall(t *testing.T) {
	engine, installer, calls := installConsole(t, true, true, http.StatusPaymentRequired, packageDigest())
	answer := send(engine, http.MethodPost, "/workspaces/current/zgi-market/installs/skills/csv", "", nil)
	if answer.Code != http.StatusPaymentRequired || !strings.Contains(answer.Body.String(), "BILLING.INSUFFICIENT_BALANCE") {
		t.Fatalf("status = %d, body = %s; want the market's refusal", answer.Code, answer.Body.String())
	}
	if len(installer.packages) != 0 || strings.Contains(strings.Join(*calls, "|"), "package") {
		t.Fatalf("the package was fetched or imported after a refused install: %v", *calls)
	}
}

func TestInstallSkillRefusesADamagedPackageAndUnknownPaths(t *testing.T) {
	engine, installer, _ := installConsole(t, true, true, http.StatusOK, strings.Repeat("0", 64))
	answer := send(engine, http.MethodPost, "/workspaces/current/zgi-market/installs/skills/csv", "", nil)
	if answer.Code == http.StatusOK || len(installer.packages) != 0 {
		t.Fatalf("status = %d, imports = %d; want a package whose digest differs refused", answer.Code, len(installer.packages))
	}
	for _, target := range []string{"/installs/skills/Bad_Slug", "/installs/skills/a/b", "/installs/plugins/x", "/skills/csv/package"} {
		got := send(engine, http.MethodPost, "/workspaces/current/zgi-market"+target, "", nil)
		if got.Code != http.StatusNotFound {
			t.Fatalf("%s = %d, want not found", target, got.Code)
		}
	}
	if got := send(engine, http.MethodDelete, "/workspaces/current/zgi-market/installs/skills/csv", "", nil); got.Code != http.StatusNotFound {
		t.Fatalf("DELETE = %d, want not found", got.Code)
	}
}

func TestAMemberInstallsAFreeSkillButDoesNotReplaceOne(t *testing.T) {
	installListing = `{"id":7,"current_version":{"label":"1.2.0"},"prices":[{"meter":"install","unit_price_micros":0}]}`
	t.Cleanup(func() { installListing = `{"id":7,"current_version":{"label":"1.2.0"}}` })
	engine, installer, calls := installConsole(t, true, false, http.StatusCreated, packageDigest())

	replace := send(engine, http.MethodPost, "/workspaces/current/zgi-market/installs/skills/csv", `{"overwrite":true}`, nil)
	if replace.Code != http.StatusForbidden || len(installer.packages) != 0 {
		t.Fatalf("overwrite = %d, imports = %d; replacing a skill stays an administrator's", replace.Code, len(installer.packages))
	}
	answer := send(engine, http.MethodPost, "/workspaces/current/zgi-market/installs/skills/csv", "", nil)
	if answer.Code != http.StatusOK || len(installer.packages) != 1 {
		t.Fatalf("status = %d %s, imports = %d", answer.Code, answer.Body.String(), len(installer.packages))
	}
	if !strings.Contains(strings.Join(*calls, ","), "POST subscriptions as admin") {
		t.Fatalf("calls = %v; a free skill is activated with the administrator's right", *calls)
	}
}
