package middleware

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/shared/interface"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/pkg/response"
)

func AccountInitRequired(accountService interfaces.AccountService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("account_service", accountService)

		accountID := c.GetString("account_id")
		if accountID == "" {
			response.Fail(c, response.ErrUnauthorized)
			c.Abort()
			return
		}

		account, err := accountService.GetAccountByID(context.Background(), accountID)
		if err != nil {
			response.Fail(c, response.ErrAccountNotFound)
			c.Abort()
			return
		}

		if account.Status == "uninitialized" {
			response.Fail(c, response.ErrAccountNotInitialized)
			c.Abort()
			return
		}

		c.Set("current_account", account)

		c.Next()
	}
}
