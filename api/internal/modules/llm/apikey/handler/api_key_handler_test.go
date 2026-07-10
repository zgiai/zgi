package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/dto"
)

func TestDecodeUpdateAPIKeyRequestMarksExplicitNulls(t *testing.T) {
	var req dto.UpdateAPIKeyRequest

	err := decodeUpdateAPIKeyRequest(strings.NewReader(`{"quota_limit":null,"expires_at":null}`), &req)

	require.NoError(t, err)
	require.True(t, req.ClearQuotaLimit)
	require.True(t, req.ClearExpiresAt)
}

func TestDecodeUpdateAPIKeyRequestLeavesMissingFieldsUnchanged(t *testing.T) {
	var req dto.UpdateAPIKeyRequest

	err := decodeUpdateAPIKeyRequest(strings.NewReader(`{"name":"demo"}`), &req)

	require.NoError(t, err)
	require.False(t, req.ClearQuotaLimit)
	require.False(t, req.ClearExpiresAt)
}

func TestDecodeUpdateAPIKeyRequestRejectsOversizedBody(t *testing.T) {
	var req dto.UpdateAPIKeyRequest

	err := decodeUpdateAPIKeyRequest(strings.NewReader(strings.Repeat("x", 64*1024+1)), &req)

	require.ErrorContains(t, err, "request body exceeds 65536 bytes")
}
