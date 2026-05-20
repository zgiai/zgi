package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/pkg/response"
)

func errorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, response.ErrorResponse{
		Code:    strconv.Itoa(status),
		Message: message,
	})
}

func successWithStatus(c *gin.Context, status int, data interface{}) {
	c.JSON(status, response.Response{
		Code:    "0",
		Message: "success",
		Data:    data,
	})
}

func getAccountID(c *gin.Context) (uuid.UUID, error) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		return uuid.Nil, errors.New("account_id not found in context")
	}
	return uuid.Parse(accountID)
}

func getGroupID(c *gin.Context) (uuid.UUID, error) {
	for _, key := range []string{"tenant_id", "group_id"} {
		if value := c.GetString(key); value != "" {
			return uuid.Parse(value)
		}
	}
	return uuid.Nil, errors.New("group_id not found in context")
}

func getIntQuery(c *gin.Context, key string, defaultValue int) int {
	if raw := c.Query(key); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return defaultValue
}
