package approval

import (
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
)

type NodeData struct {
	base.NodeData
	Approval      approvalruntime.NodeConfig    `json:"approval"`
	SubmitMethods approvalruntime.SubmitMethods `json:"submit_methods"`
	Timeout       approvalruntime.TimeoutConfig `json:"timeout"`
}

type Node struct {
	base.NodeStruct
	NodeData  NodeData
	smsSender notificationsms.Service
}
