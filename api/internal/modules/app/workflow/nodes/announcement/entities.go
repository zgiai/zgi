package announcement

import (
	announcementruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/announcement"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
)

type NodeData struct {
	base.NodeData
	Announcement announcementruntime.NodeConfig    `json:"announcement"`
	Timeout      announcementruntime.TimeoutConfig `json:"timeout"`
}

type Node struct {
	base.NodeStruct
	NodeData NodeData
}
