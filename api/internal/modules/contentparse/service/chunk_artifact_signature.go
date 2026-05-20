package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/zgiai/ginext/internal/contracts"
)

func ChunkUnitsContentHash(units []contracts.ChunkUnit) string {
	h := sha256.New()
	for _, unit := range units {
		h.Write([]byte(unit.ChunkID))
		h.Write([]byte{0})
		h.Write([]byte(unit.Kind))
		h.Write([]byte{0})
		h.Write([]byte(strconv.Itoa(unit.Order)))
		h.Write([]byte{0})
		h.Write([]byte(strings.TrimSpace(unit.Content)))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ChunkArtifactSignature(sourceContentHash string, plan contracts.ChunkPlan, plannerName string, chunkerVersion string, contentHash string) string {
	return SHA256Hex(strings.Join([]string{
		strings.TrimSpace(sourceContentHash),
		string(plan.UseCase),
		strings.TrimSpace(plannerName),
		strings.TrimSpace(plan.ParentMode),
		strings.TrimSpace(plan.Segmentation),
		strings.TrimSpace(chunkerVersion),
		strings.TrimSpace(contentHash),
	}, "|"))
}

func SHA256Hex(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
