package modelmeta

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/pkg/response"
	"gorm.io/gorm"
)

type superAdminRow struct {
	IsSuperAdmin bool `gorm:"column:is_super_admin"`
}

// SuperAdminRequired allows only system super admins to access ModelMeta APIs.
func (h *Handler) SuperAdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := strings.TrimSpace(c.GetString("account_id"))
		if accountID == "" {
			response.Fail(c, response.ErrUnauthorized)
			c.Abort()
			return
		}

		allowed, err := h.isSuperAdmin(c.Request.Context(), accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			c.Abort()
			return
		}

		if !allowed {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}

		c.Next()
	}
}

func (h *Handler) isSuperAdmin(ctx context.Context, accountID string) (bool, error) {
	if h == nil || h.service == nil || h.service.db == nil {
		return false, gorm.ErrInvalidDB
	}

	var row superAdminRow
	tx := h.service.db.WithContext(ctx).
		Table("accounts").
		Select("is_super_admin").
		Where("id = ? AND deleted_at IS NULL", accountID).
		Limit(1).
		Scan(&row)
	if tx.Error != nil {
		return false, tx.Error
	}
	if tx.RowsAffected == 0 {
		return false, nil
	}

	return row.IsSuperAdmin, nil
}
