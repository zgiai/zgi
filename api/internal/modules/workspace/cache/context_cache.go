package cache

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/internal/cache/keys"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

const (
	modulePrefix          = "workspace.context"
	accountGenerationPart = "account_generation"
	orgGenerationPart     = "organization_generation"
	currentOrgPart        = "current_organization"
	organizationsPart     = "organizations"
	workspacesPart        = "workspaces"

	entryTTL       = 45 * time.Second
	generationTTL  = 24 * time.Hour
	redisOpTimeout = 50 * time.Millisecond
)

type AccountScopedToken struct {
	accountID  string
	generation string
}

type OrganizationWorkspaceToken struct {
	organizationID         string
	accountID              string
	accountGeneration      string
	organizationGeneration string
}

func NewAccountScopedToken(ctx context.Context, accountID string) AccountScopedToken {
	return AccountScopedToken{
		accountID:  accountID,
		generation: accountGeneration(ctx, accountID),
	}
}

func NewOrganizationWorkspaceToken(ctx context.Context, organizationID, accountID string) OrganizationWorkspaceToken {
	generationValues := generations(ctx, accountGenerationKey(accountID), organizationGenerationKey(organizationID))
	return OrganizationWorkspaceToken{
		organizationID:         organizationID,
		accountID:              accountID,
		accountGeneration:      generationValues[0],
		organizationGeneration: generationValues[1],
	}
}

func GetCurrentOrganization(ctx context.Context, token AccountScopedToken) (*shared_dto.CurrentOrganizationResponse, bool) {
	var value shared_dto.CurrentOrganizationResponse
	if !getJSON(ctx, currentOrganizationKey(token), &value) {
		return nil, false
	}
	return &value, true
}

func SetCurrentOrganization(ctx context.Context, token AccountScopedToken, value *shared_dto.CurrentOrganizationResponse) {
	if token.generation != accountGeneration(ctx, token.accountID) {
		return
	}
	setJSON(ctx, currentOrganizationKey(token), value)
}

func GetOrganizationList(ctx context.Context, token AccountScopedToken, page, limit int, status string) (*shared_dto.OrganizationPaginationResponse, bool) {
	var value shared_dto.OrganizationPaginationResponse
	if !getJSON(ctx, organizationListKey(token, page, limit, status), &value) {
		return nil, false
	}
	return &value, true
}

func SetOrganizationList(ctx context.Context, token AccountScopedToken, page, limit int, status string, value *shared_dto.OrganizationPaginationResponse) {
	if token.generation != accountGeneration(ctx, token.accountID) {
		return
	}
	setJSON(ctx, organizationListKey(token, page, limit, status), value)
}

func GetOrganizationWorkspaces(ctx context.Context, token OrganizationWorkspaceToken, page, limit int, status, keyword string) (*shared_dto.OrganizationWorkspacePaginationResponse, bool) {
	var value shared_dto.OrganizationWorkspacePaginationResponse
	if !getJSON(ctx, organizationWorkspacesKey(token, page, limit, status, keyword), &value) {
		return nil, false
	}
	return &value, true
}

func SetOrganizationWorkspaces(ctx context.Context, token OrganizationWorkspaceToken, page, limit int, status, keyword string, value *shared_dto.OrganizationWorkspacePaginationResponse) {
	if token.accountGeneration != accountGeneration(ctx, token.accountID) {
		return
	}
	if token.organizationGeneration != organizationGeneration(ctx, token.organizationID) {
		return
	}
	setJSON(ctx, organizationWorkspacesKey(token, page, limit, status, keyword), value)
}

func InvalidateAccount(ctx context.Context, accountID string) {
	incrementGeneration(ctx, accountGenerationKey(accountID))
}

func InvalidateOrganization(ctx context.Context, organizationID string) {
	incrementGeneration(ctx, organizationGenerationKey(organizationID))
}

func InvalidateOrganizationWithWorkspaceMembers(ctx context.Context, db *gorm.DB, organizationID string, accountIDs ...string) {
	if redisutil.GetClient() == nil {
		return
	}
	InvalidateOrganization(ctx, organizationID)
	invalidateAccounts(ctx, accountIDs...)
	if db == nil || organizationID == "" {
		return
	}

	var joinedAccountIDs []string
	if err := db.WithContext(ctx).
		Table("members").
		Select("account_id").
		Where("organization_id = ?", organizationID).
		Pluck("account_id", &joinedAccountIDs).Error; err != nil {
		return
	}

	var workspaceAccountIDs []string
	if err := db.WithContext(ctx).
		Table("workspace_members").
		Select("DISTINCT workspace_members.account_id").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id").
		Where("workspaces.organization_id = ?", organizationID).
		Pluck("workspace_members.account_id", &workspaceAccountIDs).Error; err != nil {
		return
	}

	invalidateAccounts(ctx, append(joinedAccountIDs, workspaceAccountIDs...)...)
}

func InvalidateWorkspace(ctx context.Context, db *gorm.DB, workspaceID string, accountIDs ...string) {
	if redisutil.GetClient() == nil {
		return
	}
	invalidateAccounts(ctx, accountIDs...)
	if db == nil || workspaceID == "" {
		return
	}

	var organizationID string
	if err := db.WithContext(ctx).
		Table("workspaces").
		Select("organization_id").
		Where("id = ? AND organization_id IS NOT NULL", workspaceID).
		Scan(&organizationID).Error; err != nil || organizationID == "" {
		return
	}
	InvalidateOrganizationWithWorkspaceMembers(ctx, db, organizationID, accountIDs...)
}

func invalidateAccounts(ctx context.Context, accountIDs ...string) {
	seen := make(map[string]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID == "" {
			continue
		}
		if _, ok := seen[accountID]; ok {
			continue
		}
		seen[accountID] = struct{}{}
		InvalidateAccount(ctx, accountID)
	}
}

func currentOrganizationKey(token AccountScopedToken) string {
	return keys.DefaultBuilder().Build(modulePrefix, currentOrgPart, token.accountID, token.generation)
}

func organizationListKey(token AccountScopedToken, page, limit int, status string) string {
	return keys.DefaultBuilder().Build(modulePrefix, organizationsPart, token.accountID, status, strconv.Itoa(page), strconv.Itoa(limit), token.generation)
}

func organizationWorkspacesKey(token OrganizationWorkspaceToken, page, limit int, status, keyword string) string {
	return keys.DefaultBuilder().Build(modulePrefix, workspacesPart, token.organizationID, token.accountID, status, keyword, strconv.Itoa(page), strconv.Itoa(limit), token.accountGeneration, token.organizationGeneration)
}

func accountGeneration(ctx context.Context, accountID string) string {
	return generation(ctx, accountGenerationKey(accountID))
}

func organizationGeneration(ctx context.Context, organizationID string) string {
	return generation(ctx, organizationGenerationKey(organizationID))
}

func generations(ctx context.Context, generationKeys ...string) []string {
	values := make([]string, len(generationKeys))
	for i := range values {
		values[i] = "0"
	}

	client := redisutil.GetClient()
	if client == nil || len(generationKeys) == 0 {
		return values
	}
	redisCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()

	results, err := client.MGet(redisCtx, generationKeys...).Result()
	if err != nil {
		return values
	}
	for i, result := range results {
		if value, ok := result.(string); ok && value != "" {
			values[i] = value
		}
	}
	return values
}

func accountGenerationKey(accountID string) string {
	return keys.DefaultBuilder().Build(modulePrefix, accountGenerationPart, accountID)
}

func organizationGenerationKey(organizationID string) string {
	return keys.DefaultBuilder().Build(modulePrefix, orgGenerationPart, organizationID)
}

func generation(ctx context.Context, key string) string {
	client := redisutil.GetClient()
	if client == nil {
		return "0"
	}
	redisCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()

	value, err := client.Get(redisCtx, key).Result()
	if errors.Is(err, goredis.Nil) || value == "" {
		return "0"
	}
	if err != nil {
		return "0"
	}
	return value
}

func getJSON(ctx context.Context, key string, value interface{}) bool {
	client := redisutil.GetClient()
	if client == nil || key == "" {
		return false
	}
	redisCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()

	payload, err := client.Get(redisCtx, key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(payload, value) == nil
}

func setJSON(ctx context.Context, key string, value interface{}) {
	client := redisutil.GetClient()
	if client == nil || key == "" || value == nil {
		return
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	redisCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()
	_ = client.SetEx(redisCtx, key, payload, entryTTL).Err()
}

func incrementGeneration(ctx context.Context, key string) {
	client := redisutil.GetClient()
	if client == nil || key == "" {
		return
	}
	redisCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()
	_, _ = client.Pipelined(redisCtx, func(pipe goredis.Pipeliner) error {
		pipe.Incr(redisCtx, key)
		pipe.Expire(redisCtx, key, generationTTL)
		return nil
	})
}
