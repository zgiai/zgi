package service

import (
	"gorm.io/gorm"

	"github.com/zgiai/ginext/internal/modules/payment/repository"
)

// PaymentServices contains all payment-related services
type PaymentServices struct {
	AICredit    *AICreditService
	Transaction *TransactionService
}

// NewPaymentServices creates services that remain owned by zgi-api.
func NewPaymentServices(db *gorm.DB) *PaymentServices {
	creditAccountRepo := repository.NewGroupAICreditAccountRepository(db)
	privateChannelFundRepo := repository.NewPrivateChannelFundRepository(db)
	walletTxRepo := repository.NewTransactionRepository(db)

	aiCreditSvc := NewAICreditService(
		db,
		creditAccountRepo,
		walletTxRepo, // Use unified TransactionRepository
		NewConsoleOfficialCreditChecker(),
		privateChannelFundRepo,
	)

	transactionSvc := NewTransactionService(db)

	return &PaymentServices{
		AICredit:    aiCreditSvc,
		Transaction: transactionSvc,
	}
}
