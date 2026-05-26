package v1

import (
	"github.com/gin-gonic/gin"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	paymentHandler "github.com/zgiai/zgi/api/internal/modules/payment/handler"
	"github.com/zgiai/zgi/api/internal/modules/payment/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

// PaymentRouteDeps contains dependencies required by payment routes.
type PaymentRouteDeps struct {
	DB              *gorm.DB
	AccountService  interfaces.AccountService
	ConsoleProvider platformconsole.ConsoleProvider
}

// RegisterPaymentRoutes registers all payment-related routes
func RegisterPaymentRoutes(router *gin.RouterGroup, deps PaymentRouteDeps) {
	if deps.DB == nil {
		panic("payment routes require db")
	}
	if deps.AccountService == nil {
		panic("payment routes require account service")
	}
	if deps.ConsoleProvider == nil {
		panic("payment routes require console provider")
	}

	services := service.NewPaymentServices(deps.DB)

	aiCreditHandler := paymentHandler.NewAICreditHandler(services.AICredit, deps.AccountService, deps.ConsoleProvider)
	orderHandler := paymentHandler.NewOrderHandler(deps.AccountService, deps.ConsoleProvider)
	walletHandler := paymentHandler.NewWalletHandler(deps.AccountService, deps.ConsoleProvider)
	transactionHandler := paymentHandler.NewTransactionHandler(services.Transaction, deps.AccountService, deps.ConsoleProvider)
	bankTransferHandler := paymentHandler.NewBankTransferHandler(deps.AccountService, deps.ConsoleProvider)

	aiCreditHandler.RegisterRoutes(router)
	orderHandler.RegisterRoutes(router)
	walletHandler.RegisterRoutes(router)
	transactionHandler.RegisterRoutes(router)
	bankTransferHandler.RegisterRoutes(router)
}
