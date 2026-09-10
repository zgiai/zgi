package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

func TestModelConfigCacheWriteTTLOverrideColumnNames(t *testing.T) {
	parsed, err := schema.Parse(&ModelConfig{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)

	tests := map[string]string{
		"CacheWrite5mPriceOverride": "cache_write_5m_price_override",
		"CacheWrite1hPriceOverride": "cache_write_1h_price_override",
	}
	for fieldName, columnName := range tests {
		field := parsed.LookUpField(fieldName)
		require.NotNil(t, field)
		require.Equal(t, columnName, field.DBName)
	}
}
