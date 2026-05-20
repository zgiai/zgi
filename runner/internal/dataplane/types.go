package dataplane

import (
	"database/sql"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Connections groups data-plane clients for convenience.
type Connections struct {
	ORM   *gorm.DB
	SQL   *sql.DB
	Cache redis.UniversalClient
}
