package service

import (
	"context"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
)

type SystemConfigService interface {
	ConfigDefaultPluginAndConfig(ctx context.Context, tenantID string, account *auth_model.Account) error
}

type systemConfigServiceImpl struct{}

func NewSystemConfigService() SystemConfigService {
	return &systemConfigServiceImpl{}
}

func (s *systemConfigServiceImpl) ConfigDefaultPluginAndConfig(ctx context.Context, tenantID string, account *auth_model.Account) error {
	// TODO: implement later
	return nil
}
