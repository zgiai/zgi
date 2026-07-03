package tests

import (
	"os"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
)

func testDBConfig() driver.Config {
	return driver.Config{
		DBHost: envOrDefault("SQLMETA_TEST_DB_HOST", "localhost"),
		DBPort: envOrDefault("SQLMETA_TEST_DB_PORT", "5432"),
		DBUser: envOrDefault("SQLMETA_TEST_DB_USER", "postgres"),
		DBPass: envOrDefault("SQLMETA_TEST_DB_PASS", "Abc1234"),
		DBName: envOrDefault("SQLMETA_TEST_DB_NAME", "postgres"),
	}
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
