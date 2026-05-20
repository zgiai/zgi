package service

import (
	"context"
)

type BillingServiceImpl struct {
}

func NewBillingService() *BillingServiceImpl {
	return &BillingServiceImpl{}
}

func (s *BillingServiceImpl) IsEmailInFreeze(ctx context.Context, email string) (bool, error) {
	return false, nil
}

func (s *BillingServiceImpl) CheckResourceQuota(ctx context.Context, accountID string, resourceType string, requestAmount ...int64) error {
	return nil
}
