package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesDoesNotConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")

	NewHandler(nil).RegisterRoutes(group)
}
