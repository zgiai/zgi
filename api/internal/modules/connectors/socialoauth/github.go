package socialoauth

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func (s *Service) githubInstallURL() string {
	return "https://github.com/apps/" + s.opts.Providers["github"].AppSlug + "/installations/new"
}

// Discover installations from the authenticated user's grant, never from an
// installation_id supplied by the browser. Only this configured app qualifies.
func (s *Service) githubInstalled(ctx context.Context, token string) (bool, error) {
	for page := 1; page <= 10; page++ {
		var result struct {
			TotalCount    int `json:"total_count"`
			Installations []struct {
				ID          int64   `json:"id"`
				AppID       int64   `json:"app_id"`
				SuspendedAt *string `json:"suspended_at"`
			} `json:"installations"`
		}
		endpoint := fmt.Sprintf("https://api.github.com/user/installations?per_page=100&page=%d", page)
		if err := s.request(ctx, http.MethodGet, endpoint, nil, token, false, ProviderConfig{}, &result); err != nil {
			return false, err
		}
		for _, installation := range result.Installations {
			if installation.ID > 0 && strconv.FormatInt(installation.AppID, 10) == s.opts.Providers["github"].AppID && installation.SuspendedAt == nil {
				return true, nil
			}
		}
		if len(result.Installations) < 100 || page*100 >= result.TotalCount {
			return false, nil
		}
	}
	return false, fail(core.ErrorCodeProviderUnavailable)
}

func (s *Service) waitForGitHubInstallation(ctx context.Context, r *flowRecord, secret secretFlow) error {
	envelope, err := s.seal(flowSubject(*r), secret)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&flowRecord{}).Where("id = ? AND status = ? AND expires_at > ?", r.ID, "processing", s.now()).Updates(map[string]any{"status": "install_app", "envelope": envelope})
	if result.Error != nil {
		return fail(core.ErrorCodeInternal)
	}
	if result.RowsAffected != 1 {
		return fail(core.ErrorCodeFlowExpired)
	}
	return nil
}

// Resume explicitly after repository selection on GitHub. The initiating actor
// must still own the pending, unexpired flow. CAS prevents concurrent resumes.
func (s *Service) CompleteInstallation(ctx context.Context, org, owner, id uuid.UUID) (Flow, error) {
	var r flowRecord
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ?", id, org, owner, "github").Take(&r).Error; err != nil {
		return Flow{}, fail(core.ErrorCodeNotFound)
	}
	if r.Status != "install_app" || !s.now().Before(r.ExpiresAt) {
		return Flow{}, fail(core.ErrorCodeFlowExpired)
	}
	claim := s.db.WithContext(ctx).Model(&flowRecord{}).Where("id = ? AND status = ? AND expires_at > ?", r.ID, "install_app", s.now()).Update("status", "processing")
	if claim.Error != nil {
		return Flow{}, fail(core.ErrorCodeInternal)
	}
	if claim.RowsAffected != 1 {
		return Flow{}, fail(core.ErrorCodeFlowExpired)
	}
	var secret secretFlow
	err := s.open(flowSubject(r), r.Envelope, &secret)
	if err == nil && (secret.Grant.AccessToken == "" || (!secret.Grant.ExpiresAt.IsZero() && !s.now().Before(secret.Grant.ExpiresAt))) {
		err = fail(core.ErrorCodeFlowExpired)
	}
	installed := false
	if err == nil {
		installed, err = s.githubInstalled(ctx, secret.Grant.AccessToken)
	}
	if err == nil && !installed {
		err = s.waitForGitHubInstallation(ctx, &r, secret)
	}
	if err == nil && installed {
		err = s.saveConnection(ctx, &r, &secret)
	}
	if err != nil {
		if persistErr := s.finishFailure(ctx, &r, core.ErrorCodeOf(err)); persistErr != nil {
			return Flow{}, persistErr
		}
		return Flow{}, err
	}
	return s.Get(ctx, org, owner, id)
}
