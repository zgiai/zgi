package base

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type NodeData struct {
	Title   string `json:"title"`
	Desc    string `json:"desc"`
	Version string `json:"version"`
	//NodeType      shared.NodeType      `json:"node_type"`
	ErrorStrategy shared.ErrorStrategy  `json:"error_strategy"`
	DefaultValue  []shared.DefaultValue `json:"default_value"`
	RetryConfig   shared.RetryConfig    `json:"retry_config"`
}
