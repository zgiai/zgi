package service

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func ProviderSignature(providerKey string, engine contracts.ParseEngine) string {
	key := strings.TrimSpace(providerKey)
	if key == "" && engine == "" {
		return "unknown"
	}
	if key == "" {
		return string(engine)
	}
	if engine == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", key, engine)
}
