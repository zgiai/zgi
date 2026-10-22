package digitalhuman

import (
	"encoding/json"

	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/datatypes"
)

func fileUploadEnabled(version *Version) bool {
	return version != nil && (version.FileUploadPolicy == nil || len(version.FileUploadPolicy.AllowedFileTypes) > 0)
}

func fileUploadPolicyJSON(policy *chatfile.Policy) datatypes.JSON {
	if policy == nil {
		return nil
	}
	encoded, _ := json.Marshal(policy)
	return datatypes.JSON(encoded)
}

func effectiveFileUploadPolicy(version *Version, config *interfaces.FileUploadConfigResponse) chatfile.Effective {
	var policy *chatfile.Policy
	if version != nil {
		policy = version.FileUploadPolicy
	}
	return chatfile.ResolveLimits(policy, config.ChatLimits())
}
