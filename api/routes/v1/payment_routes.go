package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/container"
	paymentHandler "github.com/zgiai/zgi/api/internal/modules/payment/handler"
	"github.com/zgiai/zgi/api/internal/modules/payment/service"
)

// RegisterPaymentRoutes registers all payment-related routes
func RegisterPaymentRoutes(router *gin.RouterGroup, container *container.ServiceContainer) {
	// Get account service from container
	accountService := container.GetAccountServiceAdapter()
	consoleProvider := container.GetConsoleProvider()

	// Initialize payment services
	services := service.NewPaymentServices(container.GetDB())

	// Initialize handlers
	aiCreditHandler := paymentHandler.NewAICreditHandler(services.AICredit, accountService, consoleProvider)
	orderHandler := paymentHandler.NewOrderHandler(accountService, consoleProvider)
	walletHandler := paymentHandler.NewWalletHandler(accountService, consoleProvider)
	transactionHandler := paymentHandler.NewTransactionHandler(services.Transaction, accountService, consoleProvider)
	bankTransferHandler := paymentHandler.NewBankTransferHandler(accountService, consoleProvider)

	// Register routes for each handler
	aiCreditHandler.RegisterRoutes(router)
	orderHandler.RegisterRoutes(router)
	walletHandler.RegisterRoutes(router)
	transactionHandler.RegisterRoutes(router)
	bankTransferHandler.RegisterRoutes(router)
}
