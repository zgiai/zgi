package middleware

import (
	"net/http"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/response"

	"github.com/gin-gonic/gin"
)

func AccountInitializationRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("current_account"); exists {
			c.Next()
			return
		}

		userID, exists := c.Get("account_id")
		if !exists {
			response.Fail(c, response.ErrUnauthorized)
			c.Abort()
			return
		}

		var account auth_model.Account
		db := database.GetDB()
		if err := db.WithContext(c.Request.Context()).Where("ID = ?", userID).First(&account).Error; err != nil {
			response.Fail(c, response.ErrGetUserInfoFailed)
			c.Abort()
			return
		}

		//if !user.Initialized{
		//	c.JSON(http.StatusForbidden, gin.H{
		//		"code": 403,
		//		"msg":  "user not initialized",
		//	})
		//	c.Abort()
		//	return
		//}

		c.Set("current_account", &account)
		c.Next()
	}
}

func MarshalWith(fields []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		data, exists := c.Get("response")
		if !exists {
			return
		}

		filteredData := make(map[string]interface{})
		if dataMap, ok := data.(map[string]interface{}); ok {
			for _, field := range fields {
				if value, exists := dataMap[field]; exists {
					filteredData[field] = value
				}
			}
		}

		c.JSON(http.StatusOK, filteredData)
	}
}
