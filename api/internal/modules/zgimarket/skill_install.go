package zgimarket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	installsPrefix   = "installs/skills"
	skillSHAHeader   = "X-ZGI-Skill-Sha256"
	installBodyLimit = 4 << 10
)

var marketSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)

// SkillPackage is a skill package the market served for an installed listing.
type SkillPackage struct {
	Slug     string
	Version  string
	SHA256   string
	Filename string
	Data     []byte
}

// SkillInstall is the outcome of importing a market skill package into the organization's skills.
// Exactly one of the three holds: the skill was installed, the package was refused by the import
// checks, or it would replace a skill that did not come from this listing.
type SkillInstall struct {
	Installed bool   `json:"installed"`
	SkillID   string `json:"skill_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Version   string `json:"version,omitempty"`
	// Enabled reports whether the organization has the installed skill switched on.
	Enabled          bool     `json:"enabled"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	// Conflict names the organization's own skill the package would replace.
	Conflict string `json:"conflict,omitempty"`
}

// InstalledSkill is an organization skill that was installed from a market listing.
type InstalledSkill struct {
	Slug    string `json:"slug"`
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SkillInstaller imports market skill packages into the organization's skills through the
// import every skill package goes through. The market keeps no copy of what is installed here.
type SkillInstaller interface {
	Install(ctx context.Context, organizationID, accountID string, pkg SkillPackage, overwrite bool) (*SkillInstall, error)
	Installed(ctx context.Context, organizationID, accountID string) ([]InstalledSkill, error)
}

// WithSkillInstaller lets organization administrators install market skills.
func (p *Proxy) WithSkillInstaller(installer SkillInstaller) *Proxy {
	p.skills = installer
	return p
}

// install handles the requests below installs/skills, which this instance answers itself. It
// reports whether the request was one of them.
func (p *Proxy) install(c *gin.Context) bool {
	path := strings.Trim(c.Param("path"), "/")
	if path != installsPrefix && !strings.HasPrefix(path, installsPrefix+"/") {
		return false
	}
	if p.skills == nil || p.config.ConsoleURL == "" {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is not configured")
		return true
	}
	slug := strings.TrimPrefix(strings.TrimPrefix(path, installsPrefix), "/")
	switch {
	case slug == "" && c.Request.Method == http.MethodGet:
		p.listInstalledSkills(c)
	case marketSlugPattern.MatchString(slug) && c.Request.Method == http.MethodPost:
		p.installSkill(c, slug)
	default:
		response.Fail(c, response.ErrNotFound)
	}
	return true
}

func (p *Proxy) listInstalledSkills(c *gin.Context) {
	who, ok := p.member(c)
	if !ok {
		return
	}
	installed, err := p.skills.Installed(c.Request.Context(), who.organizationID, who.accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if installed == nil {
		installed = []InstalledSkill{}
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, installed)
}

// installSkill activates the skill listing for the organization, which is the market's install
// and what the market bills, then downloads its package and imports it into the organization.
func (p *Proxy) installSkill(c *gin.Context, slug string) {
	who, ok := p.member(c)
	if !ok {
		return
	}
	elevated := false
	if !who.admin {
		// A free skill costs the organization nothing, so a member installs it too. Replacing a
		// skill the organization already has stays an administrator's decision.
		listing, ok := p.describe(c.Request.Context(), who, "skill", slug)
		if !ok || !pricesAreFree(listing.Prices, "") {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		who.admin, elevated = true, true
	}
	var input struct {
		Overwrite bool `json:"overwrite"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, installBodyLimit))
	if err != nil || (len(strings.TrimSpace(string(body))) > 0 && json.Unmarshal(body, &input) != nil) {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if elevated && input.Overwrite {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	ctx := c.Request.Context()
	unavailable := func() {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is unavailable")
	}

	listing, err := p.call(ctx, who, http.MethodGet, "listings/skill/"+slug, nil, nil, answerBodyLimit)
	if err != nil {
		unavailable()
		return
	}
	if listing.status != http.StatusOK {
		p.relay(c, listing)
		return
	}
	var described struct {
		Data struct {
			ID             uint64 `json:"id"`
			CurrentVersion *struct {
				Label string `json:"label"`
			} `json:"current_version"`
		} `json:"data"`
	}
	if json.Unmarshal(listing.content, &described) != nil || described.Data.ID == 0 {
		unavailable()
		return
	}

	request, _ := json.Marshal(map[string]uint64{"listing_id": described.Data.ID}) //nolint:errcheck // a map of integers always encodes
	headers := http.Header{"Content-Type": []string{"application/json"}}
	subscription, err := p.call(ctx, who, http.MethodPost, "subscriptions", request, headers, answerBodyLimit)
	if err != nil {
		unavailable()
		return
	}
	// The market answers 201 for a new install and 200 for one that is already active; anything
	// else, such as a balance that does not cover the install price, is its answer to the member.
	if subscription.status != http.StatusOK && subscription.status != http.StatusCreated {
		p.relay(c, subscription)
		return
	}
	if elevated {
		// The skill is activated now: the requests asking an administrator for it are answered.
		p.closeActivated(ctx, who, described.Data.ID, 0)
	}

	archive, err := p.call(ctx, who, http.MethodGet, "skills/"+slug+"/package", nil, nil, packageBodyLimit)
	if err != nil {
		unavailable()
		return
	}
	if archive.status != http.StatusOK {
		p.relay(c, archive)
		return
	}
	sum := sha256.Sum256(archive.content)
	digest := hex.EncodeToString(sum[:])
	if stated := strings.TrimSpace(archive.header.Get(skillSHAHeader)); stated != "" && !strings.EqualFold(stated, digest) {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "The skill package arrived damaged; try again")
		return
	}
	pkg := SkillPackage{Slug: slug, SHA256: digest, Filename: slug + ".zip", Data: archive.content}
	if described.Data.CurrentVersion != nil {
		pkg.Version = described.Data.CurrentVersion.Label
	}
	result, err := p.skills.Install(ctx, who.organizationID, who.accountID, pkg, input.Overwrite)
	if err != nil || result == nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "The skill package could not be imported")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, result)
}
