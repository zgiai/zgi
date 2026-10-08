package digitalhuman

import (
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/datatypes"
)

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
	if config == nil {
		return chatfile.Resolve(policy, 50, 10, chatfile.DefaultCount)
	}
	return chatfile.Resolve(policy, config.FileSizeLimit, config.ImageFileSizeLimit, chatfile.DefaultCount)
}
